package quest

import (
	"testing"

	"api-library/internal/grant"
)

// tut_p: one step that pays the Turtle (and $5).
const petCatalog = `{
  "version": 11,
  "quests": [
    {"id": "tut_p", "type": "Tutorial", "initial": true, "steps": [
      {"id": "join", "condition": {"eventType": "ActionDone", "key": "staff.joined", "target": 1},
       "rewards": [{"type": "Pet", "key": "Pet_Turtle", "amount": 1}, {"type": "Cash", "key": "", "amount": 5}]}
    ]}
  ]
}`

func TestPetStepWritesAnUnclaimedGrantAndStaysOffTheClientList(t *testing.T) {
	c, err := ParseCatalog([]byte(petCatalog))
	if err != nil {
		t.Fatal(err)
	}
	repo := newMemRepo()
	s := NewService(repo, c)

	res := complete(t, s, "tut_p", "join", 0)
	if !res.Accepted {
		t.Fatalf("not accepted: %+v", res)
	}
	// Only the cash is in `granted`: the pet arrives as a grant the client applies.
	if len(res.Granted) != 1 || res.Granted[0].Type != RewardCash {
		t.Errorf("granted = %+v, want only the cash", res.Granted)
	}
	d := repo.grants["uid"]
	if d == nil {
		t.Fatal("no grants document")
	}
	g := d.Grants[GemGrantID("tut_p", "join")+":pet:Pet_Turtle"]
	if g == nil || g.Source != grant.SourceQuest || g.SourceRef != "tut_p/join" {
		t.Fatalf("grant = %+v", g)
	}
	if len(g.Rewards) != 1 || g.Rewards[0].Type != grant.RewardPet || g.Rewards[0].Key != "Pet_Turtle" {
		t.Errorf("rewards = %+v", g.Rewards)
	}
	if g.ClaimedAt != nil {
		t.Error("the pet grant must wait for the client to apply and claim it")
	}

	// A repeat of the same step is a no-op: nothing is added twice.
	complete(t, s, "tut_p", "join", 0)
	if n := len(repo.grants["uid"].Grants); n != 1 {
		t.Errorf("grants after a repeat = %d, want 1", n)
	}
}

func TestCatalogRejectsAnUnknownPet(t *testing.T) {
	bad := `{"version": 1, "quests": [{"id": "q", "type": "Tutorial", "initial": true, "steps": [
      {"id": "s", "condition": {"eventType": "ActionDone", "key": "k", "target": 1},
       "rewards": [{"type": "Pet", "key": "Pet_Dragon", "amount": 1}]}]}]}`
	if _, err := ParseCatalog([]byte(bad)); err == nil {
		t.Fatal("a Pet reward for an unknown pet must be rejected")
	}
}

// The second pet of the store tutorial (user, 2026-10-16): after golden_fever a Turtle joins (a Pet reward, recorded in
// the grants ledger), then the fever is started once more for free (no gems).
func TestShippedTutorialTurtleJoinsThenSecondFever(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	store, ok := c.Get("tut_ch1_store")
	if !ok {
		t.Fatal("tut_ch1_store missing")
	}
	var ids []string
	byID := map[string]Step{}
	for _, s := range store.Steps {
		ids = append(ids, s.ID)
		byID[s.ID] = s
	}
	idx := func(id string) int {
		for i, v := range ids {
			if v == id {
				return i
			}
		}
		return -1
	}
	if !(idx("golden_fever") >= 0 && idx("golden_fever") < idx("turtle_joins") && idx("turtle_joins")+1 == idx("second_fever") &&
		idx("second_fever") < idx("level_to_5")) {
		t.Fatalf("store steps out of order: %v", ids)
	}
	join := byID["turtle_joins"]
	if len(join.Rewards) != 1 || join.Rewards[0].Type != RewardPet || join.Rewards[0].Key != "Pet_Turtle" || !join.NeedsGrants() {
		t.Errorf("turtle_joins rewards = %+v, want one Pet_Turtle", join.Rewards)
	}
	if len(byID["second_fever"].Rewards) != 0 {
		t.Errorf("second_fever pays %+v, want nothing (a free start, no gems)", byID["second_fever"].Rewards)
	}
}
