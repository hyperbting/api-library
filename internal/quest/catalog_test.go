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

	none := &QuestsDoc{Quests: map[string]*Progress{}}
	done := &QuestsDoc{Quests: map[string]*Progress{"tut_prologue": {QuestID: "tut_prologue", Completed: true}}}
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
		"duplicate quest":           `{"quests":[{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]},{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]}]}`,
		"no steps":                  `{"quests":[{"id":"a","type":"Goal","steps":[]}]}`,
		"duplicate step":            `{"quests":[{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"ActionDone"}},{"id":"s","condition":{"eventType":"ActionDone"}}]}]}`,
		"bad type":                  `{"quests":[{"id":"a","type":"Daily","steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]}]}`,
		"bad event":                 `{"quests":[{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"Tapped"}}]}]}`,
		"bad reward":                `{"quests":[{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"ActionDone"},"rewards":[{"type":"Gems"}]}]}]}`,
		"start after unknown quest": `{"quests":[{"id":"a","type":"Goal","startAfter":{"quest":"b","step":"s"},"steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]}]}`,
		"start after unknown step":  `{"quests":[{"id":"a","type":"Goal","steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]},{"id":"b","type":"Chapter","startAfter":{"quest":"a","step":"x"},"steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]}]}`,
		"start after itself":        `{"quests":[{"id":"a","type":"Goal","startAfter":{"quest":"a","step":"s"},"steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]}]}`,
		"missing follow":            `{"quests":[{"id":"a","type":"Goal","followUp":"b","steps":[{"id":"s","condition":{"eventType":"ActionDone"}}]}]}`,
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
	// The store opens empty (monologue, the pet walks in and talks, the rush tap and the customer lesson), the staff lesson follows the customer lesson
	// directly, then the golden customer, its talk and the free sale; the Turtle joins, the second fever runs and the Turtle leaves, then the gacha lessons
	// (Shop, the draws, the free character draw: the second pet) come before the first upgrade (user, 2026-10-24); a golden customer follows the third
	// decoration (`third_deco_golden`), and the free first warp comes right after it. The Chapter 1 maintenance lesson
	// (chore_break, chore_popup) was removed 2026-11-12 (user): nothing breaks before the Chapter 2 repair tutorial is done.
	if !(idx("store_quiet") == 0 && idx("store_quiet") < idx("pet_arrives") && idx("pet_arrives") < idx("pet_talks") && idx("pet_talks") < idx("rush_tap") &&
		idx("rush_tap") < idx("first_served") && idx("first_served") < idx("customer_asleep") && idx("customer_asleep") < idx("staff_asleep") && idx("staff_asleep") < idx("golden") && idx("golden") < idx("golden_why") &&
		idx("golden_why") < idx("golden_fever") && idx("golden_fever")+1 == idx("tell_staff_short") && idx("tell_staff_short") < idx("turtle_joins") && idx("turtle_joins")+1 == idx("turtle_talks") && idx("turtle_talks") < idx("second_fever") &&
		idx("second_fever")+1 == idx("turtle_leaves") && idx("turtle_leaves")+1 == idx("gacha_shop") && idx("gacha_shop")+1 == idx("gacha_open") && idx("gacha_open")+1 == idx("gacha_character") &&
		idx("gacha_character") < idx("earn_level") && idx("earn_level") < idx("level_first") && idx("level_first") < idx("earn_decor") && idx("earn_decor") < idx("buy_decor") && idx("buy_decor") < idx("third_deco") &&
		idx("third_deco")+1 == idx("third_deco_golden") && idx("third_deco_golden")+1 == idx("first_warp")) {
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

// The shipped Chapter 1 story quest runs alongside the store tutorial from the Tortoise's greeting on, and pays
// 10 gems at its end (GDD 5.2.1: 10 x chapter number).
func TestShippedChapterOneStory(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := c.Get("sto_ch1_first_job")
	if !ok {
		t.Fatal("sto_ch1_first_job missing")
	}
	if d.Type != QuestTypeChapter || d.Initial || d.StartAfter == nil ||
		d.StartAfter.Quest != "tut_ch1_store" || d.StartAfter.Step != "pet_talks" {
		t.Fatalf("unexpected start: %+v", d)
	}
	if got := d.Steps[len(d.Steps)-1].GemReward(); got != 10 {
		t.Errorf("last step gems = %v, want 10", got)
	}
	if ids := c.StartedAfter("tut_ch1_store", "pet_talks"); len(ids) != 1 || ids[0] != "sto_ch1_first_job" {
		t.Errorf("StartedAfter = %v", ids)
	}
}

// Chapter 2 part A (Reopening Day) follows Chapter 1 and hands the Raccoon over as a Pet reward; the Technician tutorial
// starts after the outage step. No gems in part A (the chapter pays at the end of part B).
func TestShippedChapterTwoPartA(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	ch1, ok := c.Get("sto_ch1_first_job")
	if !ok || ch1.FollowUp != "sto_ch2_reopening" {
		t.Fatalf("Chapter 1 follow-up = %q, want sto_ch2_reopening", ch1.FollowUp)
	}
	d, ok := c.Get("sto_ch2_reopening")
	if !ok {
		t.Fatal("sto_ch2_reopening missing")
	}
	if d.Type != QuestTypeChapter || d.Initial || d.StartAfter != nil {
		t.Fatalf("unexpected start: %+v", d)
	}
	var ids []string
	var petReward string
	for _, s := range d.Steps {
		ids = append(ids, s.ID)
		if s.GemReward() != 0 {
			t.Errorf("step %s pays gems; part A pays none", s.ID)
		}
		for _, r := range s.Rewards {
			if r.Type == RewardPet {
				petReward = s.ID + ":" + r.Key
			}
		}
	}
	want := []string{"reopening_day", "hand_over", "open_staff", "assign_technician", "power_outage", "outage_talk", "postcard_one"}
	if len(ids) != len(want) {
		t.Fatalf("steps = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("steps = %v, want %v", ids, want)
		}
	}
	if petReward != "hand_over:Pet_Raccoon" {
		t.Errorf("pet reward = %q, want hand_over:Pet_Raccoon", petReward)
	}
	if got := c.StartedAfter("sto_ch2_reopening", "power_outage"); len(got) != 1 || got[0] != "tut_ch2_repair" {
		t.Errorf("StartedAfter(power_outage) = %v", got)
	}
}

// Chapter 2 part B so far: the Parrot arc only (the Parrot walks in, talks, the player puts it on staff).
func TestShippedChapterTwoPartB(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	a, ok := c.Get("sto_ch2_reopening")
	if !ok || a.FollowUp != "sto_ch2_growing" {
		t.Fatalf("part A follow-up = %q, want sto_ch2_growing", a.FollowUp)
	}
	d, ok := c.Get("sto_ch2_growing")
	if !ok {
		t.Fatal("sto_ch2_growing missing")
	}
	if d.Type != QuestTypeChapter || d.Initial || d.StartAfter != nil {
		t.Fatalf("unexpected start: %+v", d)
	}
	var ids []string
	var petReward string
	for _, s := range d.Steps {
		ids = append(ids, s.ID)
		// The chapter's only gem reward: 20 on its last step (parts A and B).
		if s.ID == "slow_and_steady" {
			if s.GemReward() != 20 {
				t.Errorf("step %s pays %v gems, want 20", s.ID, s.GemReward())
			}
		} else if s.GemReward() != 0 {
			t.Errorf("step %s pays gems; the chapter pays at its end only", s.ID)
		}
		for _, r := range s.Rewards {
			if r.Type == RewardPet {
				petReward = s.ID + ":" + r.Key
			}
		}
	}
	want := []string{"ad_blitz", "parrot_joins", "parrot_talks", "assign_advertiser", "memo_one", "black_friday", "slow_and_steady"}
	if len(ids) != len(want) {
		t.Fatalf("steps = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("steps = %v, want %v", ids, want)
		}
	}
	if petReward != "parrot_joins:Pet_Parrot" {
		t.Errorf("pet reward = %q, want parrot_joins:Pet_Parrot", petReward)
	}
	if got := c.StartedAfter("sto_ch2_growing", "assign_advertiser"); len(got) != 1 || got[0] != "tut_ch2_advertiser" {
		t.Errorf("started after assign_advertiser = %v, want tut_ch2_advertiser", got)
	}
}

// Chapter 3 (Main Street), M4a spine: ten steps in their final order, the store unlocks, the Ragdoll, 30 gems at the end, and the two
// tutorials that start after its steps (story pets are not granted yet: their names are open).
func TestShippedChapterThree(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	g, ok := c.Get("sto_ch2_growing")
	if !ok || g.FollowUp != "sto_ch3_main_street" {
		t.Fatalf("Chapter 2 part B follow-up = %q, want sto_ch3_main_street", g.FollowUp)
	}
	d, ok := c.Get("sto_ch3_main_street")
	if !ok {
		t.Fatal("sto_ch3_main_street missing")
	}
	if d.Type != QuestTypeChapter || d.Initial || d.StartAfter != nil {
		t.Fatalf("unexpected start: %+v", d)
	}
	want := []string{"page_two", "arcade_dream", "open_arcade", "motel_needs_host", "open_motel", "cat_joins", "vip_invasion", "hare_late_night", "three_at_t2", "kimono_inspection", "buyout_offer"}
	var ids []string
	unlocks := map[string]string{}
	pets := map[string]string{}
	for _, s := range d.Steps {
		ids = append(ids, s.ID)
		for _, r := range s.Rewards {
			switch r.Type {
			case RewardUnlock:
				unlocks[s.ID] = r.Key
			case RewardPet:
				pets[s.ID] = r.Key
			}
		}
		if s.ID == "buyout_offer" {
			if s.GemReward() != 30 {
				t.Errorf("step %s pays %v gems, want 30", s.ID, s.GemReward())
			}
		} else if s.GemReward() != 0 {
			t.Errorf("step %s pays gems; the chapter pays at its end only", s.ID)
		}
	}
	if len(ids) != len(want) {
		t.Fatalf("steps = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("steps = %v, want %v", ids, want)
		}
	}
	if unlocks["arcade_dream"] != "store.laundromat_arcade" || unlocks["motel_needs_host"] != "store.motel" {
		t.Errorf("store unlocks = %v", unlocks)
	}
	wantPets := map[string]string{
		"arcade_dream":      "Pet_Otter",
		"motel_needs_host":  "Pet_Penguin",
		"open_motel":        "Pet_Mouse",
		"cat_joins":         "Pet_Ragdoll",
		"kimono_inspection": "Pet_Exotic",
	}
	if len(pets) != len(wantPets) {
		t.Errorf("pet rewards = %v, want %v", pets, wantPets)
	}
	for step, pet := range wantPets {
		if pets[step] != pet {
			t.Errorf("pet rewards = %v, want %v", pets, wantPets)
			break
		}
	}
	if got := c.StartedAfter("sto_ch3_main_street", "open_arcade"); len(got) != 1 || got[0] != "tut_ch3_main_street" {
		t.Errorf("started after open_arcade = %v, want tut_ch3_main_street", got)
	}
	if got := c.StartedAfter("tut_ch3_main_street", "arcade_staff"); len(got) != 1 || got[0] != "tut_ch3_tier" {
		t.Errorf("started after arcade_staff = %v, want tut_ch3_tier", got)
	}
	if got := c.StartedAfter("sto_ch3_main_street", "cat_joins"); len(got) != 1 || got[0] != "tut_ch3_concierge" {
		t.Errorf("started after cat_joins = %v, want tut_ch3_concierge", got)
	}
	for id, gems := range map[string]float64{"tut_ch3_main_street": 10, "tut_ch3_tier": 15, "tut_ch3_concierge": 20} {
		tq, ok := c.Get(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		var total float64
		for _, s := range tq.Steps {
			total += s.GemReward()
		}
		if total != gems {
			t.Errorf("%s pays %v gems, want %v", id, total, gems)
		}
	}
}
