package quest

import (
	"context"
	"testing"

	"api-library/internal/grant"
)

// memRepo mimics the Firestore repository: Update works on a copy and keeps it only when fn returns true.
type memRepo struct {
	docs   map[string]*QuestsDoc
	grants map[string]*grant.Doc
	writes int
	// failGrantWrite makes UpdateWithGrants fail after fn ran, as a transaction that does not commit.
	failGrantWrite bool
}

func newMemRepo() *memRepo {
	return &memRepo{docs: map[string]*QuestsDoc{}, grants: map[string]*grant.Doc{}}
}

func (r *memRepo) UpdateWithGrants(_ context.Context, uid string, fn func(*QuestsDoc, *grant.Doc) (bool, bool, error)) error {
	doc := clone(r.docs[uid])
	if doc == nil {
		doc = NewQuestsDoc(0)
	}
	grants := grant.NewDoc()
	if g := r.grants[uid]; g != nil {
		c := *g
		c.Grants = map[string]*grant.Grant{}
		for k, v := range g.Grants {
			gv := *v
			c.Grants[k] = &gv
		}
		grants = &c
	}
	writeQuests, writeGrants, err := fn(doc, grants)
	if err != nil {
		return err
	}
	if r.failGrantWrite {
		return context.DeadlineExceeded
	}
	if writeQuests {
		r.docs[uid] = doc
		r.writes++
	}
	if writeGrants {
		r.grants[uid] = grants
	}
	return nil
}

func (r *memRepo) Get(_ context.Context, uid string) (*QuestsDoc, error) {
	return clone(r.docs[uid]), nil
}

func (r *memRepo) Update(_ context.Context, uid string, fn func(doc *QuestsDoc) (bool, error)) error {
	doc := clone(r.docs[uid])
	if doc == nil {
		doc = NewQuestsDoc(0)
	}
	write, err := fn(doc)
	if err != nil || !write {
		return err
	}
	r.docs[uid] = doc
	r.writes++
	return nil
}

func (r *memRepo) Delete(_ context.Context, uid string) error {
	delete(r.docs, uid)
	return nil
}

func clone(d *QuestsDoc) *QuestsDoc {
	if d == nil {
		return nil
	}
	c := *d
	c.Quests = make(map[string]*Progress, len(d.Quests))
	for k, p := range d.Quests {
		q := *p
		q.CompletedSteps = append([]string(nil), p.CompletedSteps...)
		c.Quests[k] = &q
	}
	return &c
}

// tut_prologue (initial): intro (checkpoint) -> jar ($100) -> staff (checkpoint) -> assign; follow-up ch1.
const serviceCatalog = `{
  "version": 7,
  "quests": [
    {"id": "tut_prologue", "type": "Tutorial", "initial": true, "followUp": "ch1", "steps": [
      {"id": "intro", "checkpoint": true, "condition": {"eventType": "ConversationEnded", "key": "Prologue Intro", "target": 1}},
      {"id": "jar", "condition": {"eventType": "ActionDone", "key": "ui.incomejar.tap", "target": 1},
       "rewards": [{"type": "Cash", "key": "", "amount": 100}]},
      {"id": "staff", "checkpoint": true, "condition": {"eventType": "ActionDone", "key": "ui.staffroom.open", "target": 1}},
      {"id": "assign", "condition": {"eventType": "ActionDone", "key": "pet.assign", "target": 1}}
    ]},
    {"id": "ch1", "type": "Chapter", "steps": [
      {"id": "retail_t1", "condition": {"eventType": "StateChanged", "key": "store.retail.tier", "target": 1}}
    ]}
  ]
}`

func newTestService(t *testing.T) (Service, *memRepo) {
	t.Helper()
	c, err := ParseCatalog([]byte(serviceCatalog))
	if err != nil {
		t.Fatal(err)
	}
	repo := newMemRepo()
	return NewService(repo, c), repo
}

func complete(t *testing.T, s Service, quest, step string, index int) *CompleteStepResponse {
	t.Helper()
	res, err := s.CompleteStep(context.Background(), "uid", CompleteStepRequest{QuestID: quest, StepID: step, StepIndex: index})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestFreshAccountHasNoProgress(t *testing.T) {
	s, _ := newTestService(t)
	res, err := s.GetProgress(context.Background(), "uid")
	if err != nil {
		t.Fatal(err)
	}
	if res.Quests == nil || len(res.Quests) != 0 {
		t.Fatalf("want empty non-nil list, got %#v", res.Quests)
	}
}

func TestResetDropsProgressAndRegrants(t *testing.T) {
	s, _ := newTestService(t)
	complete(t, s, "tut_prologue", "intro", 0)
	complete(t, s, "tut_prologue", "jar", 1)

	if err := s.Reset(context.Background(), "uid"); err != nil {
		t.Fatal(err)
	}
	if res, _ := s.GetProgress(context.Background(), "uid"); len(res.Quests) != 0 {
		t.Fatalf("progress after reset: %+v", res.Quests)
	}

	// A fresh run starts at step 0 and grants again.
	complete(t, s, "tut_prologue", "intro", 0)
	if r := complete(t, s, "tut_prologue", "jar", 1); !r.Accepted || len(r.Granted) != 1 {
		t.Fatalf("jar after reset: %+v", r)
	}
}

func TestTutorialRunGrantsOnceAndStartsFollowUp(t *testing.T) {
	s, repo := newTestService(t)

	if r := complete(t, s, "tut_prologue", "intro", 0); !r.Accepted || len(r.Granted) != 0 || r.Quest.StepIndex != 1 {
		t.Fatalf("intro: %+v", r)
	}

	r := complete(t, s, "tut_prologue", "jar", 1)
	if !r.Accepted || len(r.Granted) != 1 || r.Granted[0].Amount != 100 {
		t.Fatalf("jar: %+v", r)
	}
	if r.Quest.StepIndex != 2 || r.Quest.CheckpointIndex != 2 {
		t.Fatalf("jar should move to the staff checkpoint: %+v", r.Quest)
	}

	// Retry of the same report: accepted, nothing granted, nothing written.
	writes := repo.writes
	if r := complete(t, s, "tut_prologue", "jar", 1); !r.Accepted || len(r.Granted) != 0 {
		t.Fatalf("jar repeat: %+v", r)
	}
	if repo.writes != writes {
		t.Fatal("a repeat must not write")
	}

	complete(t, s, "tut_prologue", "staff", 2)
	r = complete(t, s, "tut_prologue", "assign", 3)
	if !r.Accepted || !r.Quest.Completed || len(r.Started) != 1 || r.Started[0].QuestID != "ch1" {
		t.Fatalf("assign should complete the tutorial and start ch1: %+v", r)
	}

	progress, _ := s.GetProgress(context.Background(), "uid")
	if len(progress.Quests) != 2 || progress.Quests[0].QuestID != "ch1" || progress.Quests[1].QuestID != "tut_prologue" {
		t.Fatalf("progress: %+v", progress.Quests)
	}
}

func TestReplayFromCheckpointIsAccepted(t *testing.T) {
	s, _ := newTestService(t)
	complete(t, s, "tut_prologue", "intro", 0)
	complete(t, s, "tut_prologue", "jar", 1)
	complete(t, s, "tut_prologue", "staff", 2)

	// Client restarted and resumed at checkpoint 2 while the server is at step 3.
	if r := complete(t, s, "tut_prologue", "staff", 2); !r.Accepted || r.Quest.StepIndex != 3 {
		t.Fatalf("replayed checkpoint step: %+v", r)
	}
	if r := complete(t, s, "tut_prologue", "assign", 3); !r.Accepted || !r.Quest.Completed {
		t.Fatalf("assign after replay: %+v", r)
	}
}

func TestRejections(t *testing.T) {
	s, repo := newTestService(t)

	if r := complete(t, s, "nope", "x", 0); r.Accepted || r.ErrorCode != CodeUnknownQuest {
		t.Fatalf("unknown quest: %+v", r)
	}
	if r := complete(t, s, "ch1", "retail_t1", 0); r.Accepted || r.ErrorCode != CodeQuestLocked {
		t.Fatalf("locked follow-up: %+v", r)
	}
	if r := complete(t, s, "tut_prologue", "jar", 1); r.Accepted || r.ErrorCode != CodeStepMismatch || r.Quest == nil || r.Quest.StepIndex != 0 {
		t.Fatalf("skipping ahead: %+v", r)
	}
	if r := complete(t, s, "tut_prologue", "jar", 0); r.Accepted || r.ErrorCode != CodeStepMismatch {
		t.Fatalf("id/index mismatch: %+v", r)
	}
	if r := complete(t, s, "tut_prologue", "intro", 9); r.Accepted || r.ErrorCode != CodeStepMismatch {
		t.Fatalf("index out of range: %+v", r)
	}
	if repo.writes != 0 {
		t.Fatalf("rejections must not write, got %d writes", repo.writes)
	}
}

// A quest with startAfter is locked until that step of the other quest is done, is created by the server when the
// step completes (and sent in Started), and an account that passed the step before the quest existed may start it.
func TestStartAfterStep(t *testing.T) {
	c, err := ParseCatalog([]byte(`{"version":1,"quests":[
	  {"id":"tut","type":"Tutorial","initial":true,"steps":[
	    {"id":"a","condition":{"eventType":"ActionDone","key":"x"}},
	    {"id":"b","condition":{"eventType":"ActionDone","key":"y"}},
	    {"id":"c","condition":{"eventType":"ActionDone","key":"z"}}]},
	  {"id":"sto","type":"Chapter","startAfter":{"quest":"tut","step":"b"},"steps":[
	    {"id":"talk","condition":{"eventType":"ConversationEnded","key":"T"}},
	    {"id":"goal","condition":{"eventType":"StateChanged","key":"g","target":1}}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(newMemRepo(), c)

	if res := complete(t, s, "sto", "talk", 0); res.Accepted || res.ErrorCode != CodeQuestLocked {
		t.Fatalf("before tut/b the story quest must be locked, got %+v", res)
	}
	if res := complete(t, s, "tut", "a", 0); !res.Accepted || len(res.Started) != 0 {
		t.Fatalf("tut/a: %+v", res)
	}
	res := complete(t, s, "tut", "b", 1)
	if !res.Accepted || len(res.Started) != 1 || res.Started[0].QuestID != "sto" {
		t.Fatalf("tut/b should start sto, got %+v", res)
	}
	if res := complete(t, s, "sto", "talk", 0); !res.Accepted {
		t.Fatalf("sto/talk after tut/b: %+v", res)
	}
	// Both run side by side: the tutorial goes on, and the story quest is not started a second time.
	if res := complete(t, s, "tut", "c", 2); !res.Accepted || len(res.Started) != 0 {
		t.Fatalf("tut/c: %+v", res)
	}

	// An account that had passed tut/b before sto was in the catalog: no progress for sto, but it is unlocked.
	old := &QuestsDoc{Quests: map[string]*Progress{"tut": {QuestID: "tut", StepIndex: 2, CompletedSteps: []string{"a", "b"}}}}
	if !c.IsUnlockedBy("sto", old) {
		t.Error("sto should be unlocked for an account past tut/b")
	}
	early := &QuestsDoc{Quests: map[string]*Progress{"tut": {QuestID: "tut", StepIndex: 1, CompletedSteps: []string{"a"}}}}
	if c.IsUnlockedBy("sto", early) {
		t.Error("sto should stay locked before tut/b")
	}
}
