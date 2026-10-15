package quest

import (
	"context"
	"testing"

	"api-library/internal/grant"
)

// tut_a: hello (no reward) -> done (10 gems + $50); its follow-up tut_b pays 15 gems on its only step.
const gemCatalog = `{
  "version": 8,
  "quests": [
    {"id": "tut_a", "type": "Tutorial", "initial": true, "followUp": "tut_b", "steps": [
      {"id": "hello", "checkpoint": true, "condition": {"eventType": "ConversationEnded", "key": "A", "target": 1}},
      {"id": "done", "condition": {"eventType": "ActionDone", "key": "a.done", "target": 1},
       "rewards": [{"type": "Gem", "key": "", "amount": 10}, {"type": "Cash", "key": "", "amount": 50}]}
    ]},
    {"id": "tut_b", "type": "Tutorial", "steps": [
      {"id": "only", "condition": {"eventType": "ActionDone", "key": "b.done", "target": 1},
       "rewards": [{"type": "Gem", "key": "", "amount": 15}]}
    ]}
  ]
}`

func newGemService(t *testing.T) (Service, *memRepo) {
	t.Helper()
	c, err := ParseCatalog([]byte(gemCatalog))
	if err != nil {
		t.Fatal(err)
	}
	repo := newMemRepo()
	return NewService(repo, c), repo
}

func gemsOf(res *CompleteStepResponse) float64 {
	var n float64
	for _, r := range res.Granted {
		if r.Type == RewardGem {
			n += r.Amount
		}
	}
	return n
}

func TestGemStepWritesAGrantAndListsTheGems(t *testing.T) {
	s, repo := newGemService(t)
	complete(t, s, "tut_a", "hello", 0)
	res := complete(t, s, "tut_a", "done", 1)
	if !res.Accepted {
		t.Fatalf("not accepted: %+v", res)
	}
	if gemsOf(res) != 10 {
		t.Errorf("granted gems = %v, want 10 (%+v)", gemsOf(res), res.Granted)
	}
	// The cash stays on the client's list as before.
	if len(res.Granted) != 2 {
		t.Errorf("granted = %+v, want gem and cash", res.Granted)
	}

	d := repo.grants["uid"]
	if d == nil {
		t.Fatal("no grants document")
	}
	g := d.Grants[GemGrantID("tut_a", "done")]
	if g == nil || g.Source != grant.SourceQuest || g.SourceRef != "tut_a/done" {
		t.Fatalf("grant = %+v", g)
	}
	if len(g.Rewards) != 1 || g.Rewards[0].Type != grant.RewardGem || g.Rewards[0].Amount != 10 {
		t.Errorf("grant rewards = %+v (cash must stay out of the grant)", g.Rewards)
	}
	if g.ClaimedAt == nil {
		t.Error("the gem grant should be stored as claimed: the client has nothing to apply")
	}
}

func TestGemStepPaysOnce(t *testing.T) {
	s, repo := newGemService(t)
	complete(t, s, "tut_a", "hello", 0)
	complete(t, s, "tut_a", "done", 1)

	// A retry of the same step: accepted, nothing granted, still one grant.
	again := complete(t, s, "tut_a", "done", 1)
	if !again.Accepted || len(again.Granted) != 0 {
		t.Errorf("retry: %+v", again)
	}
	if n := len(repo.grants["uid"].Grants); n != 1 {
		t.Errorf("grants = %d, want 1", n)
	}

	// An admin reset lets the quest be played again, but its gems were paid: no second grant, none listed.
	if _, err := s.AdminResetQuest(context.Background(), "uid", "tut_a"); err != nil {
		t.Fatal(err)
	}
	complete(t, s, "tut_a", "hello", 0)
	replay := complete(t, s, "tut_a", "done", 1)
	if !replay.Accepted {
		t.Fatalf("replay: %+v", replay)
	}
	if gemsOf(replay) != 0 {
		t.Errorf("replay listed %v gems, want 0", gemsOf(replay))
	}
	if n := len(repo.grants["uid"].Grants); n != 1 {
		t.Errorf("grants after replay = %d, want 1", n)
	}
}

func TestGemFollowUpPaysItsOwnStep(t *testing.T) {
	s, repo := newGemService(t)
	complete(t, s, "tut_a", "hello", 0)
	complete(t, s, "tut_a", "done", 1)
	res := complete(t, s, "tut_b", "only", 0)
	if !res.Accepted || gemsOf(res) != 15 {
		t.Fatalf("tut_b: %+v", res)
	}
	if len(repo.grants["uid"].Grants) != 2 {
		t.Errorf("grants = %d, want 2", len(repo.grants["uid"].Grants))
	}
}

func TestRejectedGemStepWritesNothing(t *testing.T) {
	s, repo := newGemService(t)
	// "done" before "hello": the server refuses it, no progress and no grant.
	res := complete(t, s, "tut_a", "done", 1)
	if res.Accepted {
		t.Fatalf("accepted out of order: %+v", res)
	}
	if d := repo.grants["uid"]; d != nil && len(d.Grants) != 0 {
		t.Errorf("a refused step left grants: %+v", d.Grants)
	}
}

func TestGemStepStoresNothingWhenTheTransactionFails(t *testing.T) {
	s, repo := newGemService(t)
	complete(t, s, "tut_a", "hello", 0)
	repo.failGrantWrite = true
	if _, err := s.CompleteStep(context.Background(), "uid", CompleteStepRequest{QuestID: "tut_a", StepID: "done", StepIndex: 1}); err == nil {
		t.Fatal("expected the failed commit to surface")
	}
	if p := repo.docs["uid"].Quests["tut_a"]; p.StepIndex != 1 || p.Completed {
		t.Errorf("progress moved without the grant: %+v", p)
	}
	// The client's retry succeeds and pays.
	repo.failGrantWrite = false
	res := complete(t, s, "tut_a", "done", 1)
	if !res.Accepted || gemsOf(res) != 10 {
		t.Errorf("retry: %+v", res)
	}
}

func TestCatalogRejectsABadGemReward(t *testing.T) {
	for name, amount := range map[string]string{"zero": "0", "fraction": "2.5", "negative": "-1"} {
		src := `{"version": 1, "quests": [{"id": "q", "type": "Tutorial", "initial": true, "steps": [
		  {"id": "s", "condition": {"eventType": "ActionDone", "key": "k", "target": 1},
		   "rewards": [{"type": "Gem", "key": "", "amount": ` + amount + `}]}]}]}`
		if _, err := ParseCatalog([]byte(src)); err == nil {
			t.Errorf("%s: a Gem reward of %s was accepted", name, amount)
		}
	}
}
