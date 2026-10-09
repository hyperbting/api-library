package quest

import (
	"context"
	"testing"
)

// memRepo mimics the Firestore repository: Update works on a copy and keeps it only when fn returns true.
type memRepo struct {
	docs   map[string]*QuestsDoc
	writes int
}

func newMemRepo() *memRepo { return &memRepo{docs: map[string]*QuestsDoc{}} }

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
