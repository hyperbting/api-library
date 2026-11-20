package pet

import (
	"reflect"
	"testing"
	"time"

	"api-library/internal/grant"
)

var t0 = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

func ledger(grants ...grant.Grant) *grant.Doc {
	d := grant.NewDoc()
	for _, g := range grants {
		if _, err := d.Add(g); err != nil {
			panic(err)
		}
	}
	return d
}

func petGrant(id string, src grant.Source, at time.Time, pets ...string) grant.Grant {
	g := grant.Grant{ID: id, Source: src, CreatedAt: at}
	for _, p := range pets {
		g.Rewards = append(g.Rewards, grant.Reward{Type: grant.RewardPet, Key: p, Amount: 1})
	}
	return g
}

func change(t *testing.T, d *grant.Doc, req ChangeRequest) *ChangeResponse {
	t.Helper()
	res, _, err := ApplyChange(d, "admin1", req, t0.Add(time.Hour))
	if err != nil {
		t.Fatalf("ApplyChange: %v", err)
	}
	return res
}

func TestAddCreatesAdminGrantsAndLogsThem(t *testing.T) {
	d := ledger(petGrant("quest:q:turtle", grant.SourceQuest, t0, "Pet_Turtle"))
	res := change(t, d, ChangeRequest{RequestID: "req-add-1", Action: grant.PetAdd, Pets: []string{"Pet_Turtle", "Pet_Crow", "Pet_Pig"}, Note: "test"})
	if !res.OK || !reflect.DeepEqual(res.Entry.Pets, []string{"Pet_Crow", "Pet_Pig"}) {
		t.Fatalf("add: %+v (owned pets are skipped)", res)
	}
	g := d.Grants["admin:req-add-1"]
	if g == nil || g.Source != grant.SourceAdmin || len(g.Rewards) != 2 || g.ClaimedAt != nil {
		t.Fatalf("admin grant: %+v (unclaimed, so the client applies it)", g)
	}
	if want := []string{"Pet_Tortoise", "Pet_Crow", "Pet_Pig", "Pet_Turtle"}; !reflect.DeepEqual(Owned(d), want) {
		t.Errorf("owned %v, want %v", Owned(d), want)
	}
	if len(d.PetLog) != 1 || d.PetLog[0].By != "admin1" || d.PetLog[0].Note != "test" || !reflect.DeepEqual(d.PetLog[0].GrantIDs, []string{"admin:req-add-1"}) {
		t.Errorf("log: %+v", d.PetLog)
	}
}

func TestAddSplitsLongListsIntoGrantsOfTen(t *testing.T) {
	d := grant.NewDoc()
	all := knownPets()
	res := change(t, d, ChangeRequest{RequestID: "req-all-01", Action: grant.PetAdd, Pets: all})
	if !res.OK {
		t.Fatalf("add all: %+v", res)
	}
	for _, g := range d.Grants {
		if len(g.Rewards) > grant.MaxRewardsPerGrant {
			t.Errorf("grant %s has %d rewards", g.ID, len(g.Rewards))
		}
	}
	if len(Owned(d)) != len(all) {
		t.Errorf("owned %d pets, want %d", len(Owned(d)), len(all))
	}
}

func TestRemoveArchivesPetOnlyGrantsAndFreesTheirIDs(t *testing.T) {
	mixed := petGrant("admin:mix", grant.SourceAdmin, t0, "Pet_Crow")
	mixed.Rewards = append(mixed.Rewards, grant.Reward{Type: grant.RewardGem, Amount: 50})
	d := ledger(
		petGrant("quest:sto_ch2_growing:parrot_joins", grant.SourceQuest, t0, "Pet_Parrot"),
		petGrant("gacha:r1:pets", grant.SourceGacha, t0.Add(time.Minute), "Pet_Crow", "Pet_Pig"),
		mixed,
	)
	res := change(t, d, ChangeRequest{RequestID: "req-rm-01", Action: grant.PetRemove, Pets: []string{"Pet_Parrot", "Pet_Crow"}})
	if !res.OK || !reflect.DeepEqual(res.Entry.Pets, []string{"Pet_Parrot", "Pet_Crow"}) {
		t.Fatalf("remove: %+v", res)
	}
	if d.Has("quest:sto_ch2_growing:parrot_joins") {
		t.Error("a pet-only grant must leave the ledger, so the quest can grant the Parrot again")
	}
	if g := d.Grants["gacha:r1:pets"]; g == nil || len(g.Rewards) != 1 || g.Rewards[0].Key != "Pet_Pig" {
		t.Errorf("the draw keeps its other pet: %+v", g)
	}
	if g := d.Grants["admin:mix"]; g == nil || len(g.Rewards) != 1 || g.Rewards[0].Type != grant.RewardGem {
		t.Errorf("a mixed grant keeps its gems: %+v", g)
	}
	if want := []string{"Pet_Tortoise", "Pet_Pig"}; !reflect.DeepEqual(Owned(d), want) {
		t.Errorf("owned %v, want %v", Owned(d), want)
	}
	if n := len(res.Entry.Archived); n != 3 {
		t.Errorf("archived %d grant parts, want 3 (parrot, the crow line of the draw, the crow line of the gift)", n)
	}
}

func TestResetLeavesOnlyStartersAndIsRecorded(t *testing.T) {
	d := ledger(
		petGrant("quest:q:turtle", grant.SourceQuest, t0, "Pet_Turtle"),
		petGrant("gacha:r1:pets", grant.SourceGacha, t0, "Pet_Crow"),
	)
	res := change(t, d, ChangeRequest{RequestID: "req-reset1", Action: grant.PetReset})
	if !res.OK || !reflect.DeepEqual(res.Owned, []string{"Pet_Tortoise"}) {
		t.Fatalf("reset: %+v", res)
	}
	if len(d.Grants) != 0 || len(d.PetLog) != 1 || d.PetLog[0].Action != grant.PetReset || len(d.PetLog[0].Archived) != 2 {
		t.Errorf("ledger %v, log %+v", d.Grants, d.PetLog)
	}
	if got := d.LastPetRemoval(); !got.Equal(t0.Add(time.Hour)) {
		t.Errorf("LastPetRemoval %v", got)
	}
	// Nothing left to reset: refused, not logged again.
	if res := change(t, d, ChangeRequest{RequestID: "req-reset2", Action: grant.PetReset}); res.OK || res.ErrorCode != CodeNothingToDo || len(d.PetLog) != 1 {
		t.Errorf("second reset: %+v, log %d", res, len(d.PetLog))
	}
}

func TestAChangeAppliesOncePerRequestID(t *testing.T) {
	d := ledger(petGrant("gacha:r1:pets", grant.SourceGacha, t0, "Pet_Crow"))
	req := ChangeRequest{RequestID: "req-once-1", Action: grant.PetRemove, Pets: []string{"Pet_Crow"}}
	change(t, d, req)
	// The pet is granted again after the removal; a retry of the same request must not remove it again.
	if _, err := d.Add(petGrant("gacha:r2:pets", grant.SourceGacha, t0.Add(2*time.Hour), "Pet_Crow")); err != nil {
		t.Fatal(err)
	}
	res, write, err := ApplyChange(d, "admin1", req, t0.Add(3*time.Hour))
	if err != nil || write || !res.Repeated || !d.Has("gacha:r2:pets") {
		t.Errorf("retry: res %+v write %v err %v", res, write, err)
	}
}

func TestRefusedChanges(t *testing.T) {
	d := ledger(petGrant("gacha:r1:pets", grant.SourceGacha, t0, "Pet_Crow"))
	for _, tc := range []struct {
		req  ChangeRequest
		code string
	}{
		{ChangeRequest{RequestID: "r-empty-1", Action: grant.PetAdd}, CodeNoPets},
		{ChangeRequest{RequestID: "r-unknown", Action: grant.PetAdd, Pets: []string{"Pet_Dragon"}}, CodeUnknownPet},
		{ChangeRequest{RequestID: "r-starter", Action: grant.PetRemove, Pets: []string{"Pet_Tortoise"}}, CodeStarterPet},
		{ChangeRequest{RequestID: "r-owned-1", Action: grant.PetAdd, Pets: []string{"Pet_Crow"}}, CodeNothingToDo},
		{ChangeRequest{RequestID: "r-notown1", Action: grant.PetRemove, Pets: []string{"Pet_Pig"}}, CodeNothingToDo},
	} {
		res, write, err := ApplyChange(d, "admin1", tc.req, t0)
		if err != nil || write || res.OK || res.ErrorCode != tc.code {
			t.Errorf("%+v: res %+v write %v err %v, want %s", tc.req, res, write, err, tc.code)
		}
	}
	if len(d.PetLog) != 0 {
		t.Errorf("refused changes are not logged: %+v", d.PetLog)
	}
}

func TestPetLogKeepsTheNewestEntries(t *testing.T) {
	d := grant.NewDoc()
	for i := 0; i < grant.MaxPetLog+5; i++ {
		d.AppendPetLog(grant.PetLogEntry{RequestID: string(rune('A' + i%26)), At: t0.Add(time.Duration(i) * time.Minute)})
	}
	if len(d.PetLog) != grant.MaxPetLog || !d.PetLog[0].At.Equal(t0.Add(5*time.Minute)) {
		t.Errorf("log length %d, oldest %v", len(d.PetLog), d.PetLog[0].At)
	}
}

func TestOverviewListsSourcesAndNewestLogFirst(t *testing.T) {
	d := ledger(petGrant("gacha:r1:pets", grant.SourceGacha, t0, "Pet_Crow"))
	change(t, d, ChangeRequest{RequestID: "req-add-xx", Action: grant.PetAdd, Pets: []string{"Pet_Pig"}})
	change(t, d, ChangeRequest{RequestID: "req-rm-xxx", Action: grant.PetRemove, Pets: []string{"Pet_Crow"}})
	o := BuildOverview("u1", d, true)
	if len(o.Owned) != 2 || !o.Owned[0].Starter || o.Owned[1].Pet != "Pet_Pig" || o.Owned[1].Sources[0].Source != grant.SourceAdmin {
		t.Errorf("owned: %+v", o.Owned)
	}
	if len(o.Log) != 2 || o.Log[0].Action != grant.PetRemove {
		t.Errorf("log newest first: %+v", o.Log)
	}
	if len(o.Known) != len(grant.KnownPets) {
		t.Errorf("known %d, want %d", len(o.Known), len(grant.KnownPets))
	}
}
