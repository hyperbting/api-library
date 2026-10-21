package quest

import (
	"errors"
	"math"
	"testing"
)

func TestEmbeddedCatalogLoads(t *testing.T) {
	if _, err := LoadCatalog(); err != nil {
		t.Fatalf("embedded catalog is invalid: %v", err)
	}
}

const validCatalog = `{
  "version": 3,
  "quests": [
    {"id": "tut_prologue", "type": "Tutorial", "initial": true, "followUp": "ch1",
     "steps": [
       {"id": "intro", "checkpoint": true, "condition": {"eventType": "ConversationEnded", "key": "Prologue Intro", "target": 1}, "rewards": []},
       {"id": "jar", "condition": {"eventType": "ActionDone", "key": "ui.incomejar.tap", "target": 1},
        "rewards": [{"type": "Cash", "key": "", "amount": 100}]}
     ]},
    {"id": "ch1", "type": "Chapter",
     "steps": [{"id": "retail_t1", "condition": {"eventType": "StateChanged", "key": "store.retail.tier", "target": 1}, "rewards": []}]}
  ]
}`

func TestParseCatalog(t *testing.T) {
	c, err := ParseCatalog([]byte(validCatalog))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := c.Get("ch1"); !ok {
		t.Fatal("ch1 not indexed")
	}

	none := func(string) bool { return false }
	done := func(id string) bool { return id == "tut_prologue" }
	if !c.IsUnlockedBy("tut_prologue", none) {
		t.Error("initial quest should be unlocked")
	}
	if c.IsUnlockedBy("ch1", none) {
		t.Error("ch1 should be locked before the prologue completes")
	}
	if !c.IsUnlockedBy("ch1", done) {
		t.Error("ch1 should unlock after the prologue completes")
	}
}

func TestParseCatalogRejects(t *testing.T) {
	cases := map[string]string{
		"duplicate quest": `{"quests":[{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]},{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]}]}`,
		"no steps":        `{"quests":[{"id":"a","type":"Goal","steps":[]}]}`,
		"duplicate step":  `{"quests":[{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"ActionDone"}},{"id":"s","condition":{"eventType":"ActionDone"}}]}]}`,
		"bad type":        `{"quests":[{"id":"a","type":"Daily","steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]}]}`,
		"bad event":       `{"quests":[{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"Tapped"}}]}]}`,
		"bad reward":      `{"quests":[{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"ActionDone"},"rewards":[{"type":"Gems"}]}]}]}`,
		"missing follow":  `{"quests":[{"id":"a","type":"Goal","followUp":"b","steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]}]}`,
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCatalog([]byte(data)); !errors.Is(err, ErrInvalidCatalog) {
				t.Fatalf("want ErrInvalidCatalog, got %v", err)
			}
		})
	}
}

// The shipped tutorial chain (decided 2026-10-15): prologue -> store, with the golden bubble as a step of the store
// tutorial (after the third decoration). Free gems: 10 on the prologue's last step, 15 on the store's golden step and
// 20 on its last step.
func TestShippedTutorialChainAndGems(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("tut_ch1_golden"); ok {
		t.Error("tut_ch1_golden is a step of tut_ch1_store now")
	}

	prologue, ok := c.Get("tut_prologue")
	if !ok || prologue.FollowUp != "tut_ch1_store" {
		t.Fatalf("tut_prologue = %+v", prologue)
	}
	if got := prologue.Steps[len(prologue.Steps)-1].GemReward(); got != 10 {
		t.Errorf("prologue last step pays %v gems, want 10", got)
	}

	store, ok := c.Get("tut_ch1_store")
	if !ok {
		t.Fatal("tut_ch1_store missing")
	}
	var ids []string
	gems := map[string]float64{}
	for _, s := range store.Steps {
		ids = append(ids, s.ID)
		if g := s.GemReward(); g > 0 {
			gems[s.ID] = g
		}
	}
	idx := func(id string) int {
		for i, v := range ids {
			if v == id {
				return i
			}
		}
		return -1
	}
	// The store opens empty (monologue, the pet walks in and talks, the rush tap and the customer lesson), the staff lesson follows
	// the first upgrade, and the free first warp comes right after the third decoration (user, 2026-10-21).
	if !(idx("store_quiet") == 0 && idx("store_quiet") < idx("pet_arrives") && idx("pet_arrives") < idx("pet_talks") && idx("pet_talks") < idx("rush_tap") &&
		idx("rush_tap") < idx("first_served") && idx("first_served") < idx("customer_asleep") && idx("customer_asleep") < idx("earn_level") &&
		idx("level_first") < idx("staff_asleep") && idx("staff_asleep") < idx("buy_decor") &&
		idx("buy_decor") >= 0 && idx("buy_decor") < idx("third_deco") && idx("third_deco") < idx("first_warp") &&
		idx("first_warp") < idx("golden") && idx("golden") < idx("golden_fever")) {
		t.Errorf("store steps out of order: %v", ids)
	}
	// The first warp is a free 10 minutes of income (a TimeWarp reward, in hours).
	var warpHours float64
	for _, s := range store.Steps {
		if s.ID != "first_warp" {
			continue
		}
		for _, r := range s.Rewards {
			if r.Type == RewardTimeWarp {
				warpHours += r.Amount
			}
		}
	}
	if want := 10.0 / 60.0; math.Abs(warpHours-want) > 1e-6 {
		t.Errorf("first_warp pays %v hours of warp, want %v (10 minutes)", warpHours, want)
	}
	if gems["golden"] != 15 || gems["golden_fever"] != 5 || gems[ids[len(ids)-1]] != 20 || len(gems) != 3 {
		t.Errorf("store gems = %v, want golden 15, golden_fever 5 and %s 20", gems, ids[len(ids)-1])
	}
}
