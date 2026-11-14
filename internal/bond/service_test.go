package bond

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"api-library/internal/grant"
)

// fakeRepo keeps the bond document in memory. Update copies the document in and out, like a transaction.
type fakeRepo struct {
	doc    *Doc
	grants *grant.Doc
	writes int
}

func (r *fakeRepo) Get(context.Context, string) (*Doc, error) { return r.doc, nil }

func (r *fakeRepo) Grants(context.Context, string) (*grant.Doc, error) {
	if r.grants == nil {
		return grant.NewDoc(), nil
	}
	return r.grants, nil
}

func (r *fakeRepo) Update(_ context.Context, _ string, fresh func() Doc, fn func(*Doc) (bool, error)) error {
	var doc Doc
	if r.doc == nil {
		doc = fresh()
	} else {
		doc = *r.doc
	}
	write, err := fn(&doc)
	if err != nil || !write {
		return err
	}
	r.doc = &doc
	r.writes++
	return nil
}

var t0 = time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time            { return c.now }
func (c *clock) Advance(d time.Duration)   { c.now = c.now.Add(d) }
func newSvc(r *fakeRepo, c *clock) Service { return NewService(r, WithClock(c.Now)) }

func ledgerWith(pets map[string]time.Time) *grant.Doc {
	d := grant.NewDoc()
	for id, at := range pets {
		if _, err := d.Add(grant.Grant{ID: "quest:" + id, Source: grant.SourceQuest, CreatedAt: at,
			Rewards: []grant.Reward{{Type: grant.RewardPet, Key: id}}}); err != nil {
			panic(err)
		}
	}
	return d
}

var batchN int

func play(t *testing.T, s Service, pet string, seconds int, seen ...string) StateResponse {
	t.Helper()
	batchN++
	res, err := s.Report(context.Background(), "u", ReportRequest{
		BatchID: fmt.Sprintf("batch-%04d", batchN),
		Entries: []Entry{{Pet: pet, StoreID: "Store_1", Seconds: seconds, Seen: seen}},
	})
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	return res
}

func TestFirstHeartAt300Seconds(t *testing.T) {
	c := &clock{now: t0}
	s := newSvc(&fakeRepo{}, c)

	c.Advance(5 * time.Minute)
	res := play(t, s, "Pet_Tortoise", 299)
	if got := res.Pets["Pet_Tortoise"].BondDays; got != 0 {
		t.Fatalf("299 s: %d hearts, want 0", got)
	}
	c.Advance(time.Second)
	res = play(t, s, "Pet_Tortoise", 1)
	v := res.Pets["Pet_Tortoise"]
	if v.BondDays != 1 || v.Level != 1 || v.SecondsSinceLastBond != 0 {
		t.Fatalf("300 s: %+v, want 1 heart at level 1", v)
	}
	if v.NextHeartInSeconds != int(Cooldown/time.Second) {
		t.Fatalf("nextHeartInSeconds %d, want %d", v.NextHeartInSeconds, int(Cooldown/time.Second))
	}
}

func TestCooldownDropsSecondsAndOneHeartPerReport(t *testing.T) {
	c := &clock{now: t0}
	r := &fakeRepo{}
	s := newSvc(r, c)
	c.Advance(10 * time.Minute)
	if got := play(t, s, "Pet_Tortoise", 900).Pets["Pet_Tortoise"].BondDays; got != 1 {
		t.Fatalf("900 s in one report: %d hearts, want 1", got)
	}
	c.Advance(10 * time.Minute)
	v := play(t, s, "Pet_Tortoise", 600).Pets["Pet_Tortoise"]
	if v.BondDays != 1 || v.SecondsSinceLastBond != 0 {
		t.Fatalf("in cooldown: %+v, want seconds dropped", v)
	}
	if r.doc.Pets["Pet_Tortoise"].LastReportAt == nil || !r.doc.Pets["Pet_Tortoise"].LastReportAt.Equal(c.now) {
		t.Fatalf("lastReportAt should move on a report during the cooldown")
	}
	c.Advance(Cooldown)
	if got := play(t, s, "Pet_Tortoise", 300).Pets["Pet_Tortoise"]; got.BondDays != 2 || got.NextHeartInSeconds == 0 {
		t.Fatalf("after the cooldown: %+v, want a second heart", got)
	}
}

func TestClampAfterLastReport(t *testing.T) {
	c := &clock{now: t0}
	r := &fakeRepo{}
	s := newSvc(r, c)
	c.Advance(time.Minute)
	play(t, s, "Pet_Tortoise", 60)
	c.Advance(100 * time.Second)
	got := play(t, s, "Pet_Tortoise", 200).Pets["Pet_Tortoise"]
	// 60 + min(200, 100+10) = 170
	if got.SecondsSinceLastBond != 170 {
		t.Fatalf("seconds %d, want 170", got.SecondsSinceLastBond)
	}
}

func TestFirstReportWindowAndGrantTime(t *testing.T) {
	c := &clock{now: t0}
	// A pet granted 2 minutes ago cannot report 5 minutes.
	r := &fakeRepo{grants: ledgerWith(map[string]time.Time{"Pet_Raccoon": t0.Add(-2 * time.Minute)})}
	s := newSvc(r, c)
	got := play(t, s, "Pet_Raccoon", 300).Pets["Pet_Raccoon"]
	if got.BondDays != 0 || got.SecondsSinceLastBond != 130 {
		t.Fatalf("granted 2 min ago: %+v, want 130 s and no heart", got)
	}
	// A starter pet's first report covers at most the first-report window.
	got = play(t, s, "Pet_Tortoise", 2000).Pets["Pet_Tortoise"]
	if got.BondDays != 1 {
		t.Fatalf("starter first report: %+v, want a heart (the window allows 600 s)", got)
	}
}

func TestUnclaimedGrantCountsAndUnknownPetIsRejected(t *testing.T) {
	c := &clock{now: t0}
	r := &fakeRepo{grants: ledgerWith(map[string]time.Time{"Pet_Turtle": t0.Add(-time.Hour)})} // unclaimed
	s := newSvc(r, c)
	res, err := s.Report(context.Background(), "u", ReportRequest{BatchID: "batch-unknown-1", Entries: []Entry{
		{Pet: "Pet_Turtle", StoreID: "Store_1", Seconds: 100},
		{Pet: "Pet_Wani", StoreID: "Store_1", Seconds: 100},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Pets["Pet_Turtle"].SecondsSinceLastBond != 100 {
		t.Fatalf("unclaimed grant: %+v, want its seconds counted", res.Pets["Pet_Turtle"])
	}
	if len(res.Rejected) != 1 || res.Rejected[0].Pet != "Pet_Wani" || res.Rejected[0].Reason != ReasonUnknownPet {
		t.Fatalf("rejected %+v, want Pet_Wani unknown", res.Rejected)
	}
	if _, ok := r.doc.Pets["Pet_Wani"]; ok {
		t.Fatalf("an unknown pet got a record (and a lastReportAt)")
	}
}

func TestSeenPatternIdempotenceAndEmptyStore(t *testing.T) {
	c := &clock{now: t0}
	r := &fakeRepo{}
	s := newSvc(r, c)
	res, err := s.Report(context.Background(), "u", ReportRequest{BatchID: "batch-seen-01", Entries: []Entry{
		{Pet: "Pet_Tortoise", Seen: []string{"Bond1", "lvl1", "Bond9", "Bond1"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Pets["Pet_Tortoise"].Seen; len(got) != 2 || got[0] != "Bond1" || got[1] != "lvl1" {
		t.Fatalf("seen %v, want [Bond1 lvl1]", got)
	}
	if len(res.Rejected) != 1 || res.Rejected[0].Item != "Bond9" || res.Rejected[0].Reason != ReasonUnknownSeen {
		t.Fatalf("rejected %+v, want Bond9", res.Rejected)
	}
	if r.doc.Pets["Pet_Tortoise"].LastReportAt != nil {
		t.Fatalf("a seen-only entry moved lastReportAt")
	}
	play(t, s, "Pet_Tortoise", 0, "Bond1")
	if got := r.doc.Pets["Pet_Tortoise"].Seen; len(got) != 2 {
		t.Fatalf("seen again: %v", got)
	}
}

func TestRepeatedBatchIsAppliedOnce(t *testing.T) {
	c := &clock{now: t0}
	r := &fakeRepo{}
	s := newSvc(r, c)
	c.Advance(10 * time.Minute)
	req := ReportRequest{BatchID: "batch-repeat-1", Entries: []Entry{{Pet: "Pet_Tortoise", StoreID: "Store_1", Seconds: 300}}}
	if _, err := s.Report(context.Background(), "u", req); err != nil {
		t.Fatal(err)
	}
	c.Advance(Cooldown + time.Hour)
	res, err := s.Report(context.Background(), "u", req)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Duplicate || res.Pets["Pet_Tortoise"].BondDays != 1 || r.writes != 1 {
		t.Fatalf("resend: duplicate=%v hearts=%d writes=%d, want true 1 1", res.Duplicate, res.Pets["Pet_Tortoise"].BondDays, r.writes)
	}
}

func TestBatchRingKeepsTheLast20(t *testing.T) {
	doc := newDoc(t0)
	for i := 0; i < MaxBatchIDs+5; i++ {
		applyReport(doc, map[string]bool{"Pet_Tortoise": true}, nil, ReportRequest{BatchID: string(rune('A' + i)), Entries: []Entry{{Pet: "Pet_Tortoise"}}}, t0)
	}
	if len(doc.Batches) != MaxBatchIDs || doc.Batches[0] != string(rune('A'+5)) {
		t.Fatalf("batches %v", doc.Batches)
	}
}

func TestFirstMetAt(t *testing.T) {
	c := &clock{now: t0}
	early, late := t0.Add(-48*time.Hour), t0.Add(-time.Hour)
	g := ledgerWith(map[string]time.Time{"Pet_Raccoon": late})
	if _, err := g.Add(grant.Grant{ID: "gacha:x:pets", Source: grant.SourceGacha, CreatedAt: early,
		Rewards: []grant.Reward{{Type: grant.RewardPet, Key: "Pet_Raccoon"}}}); err != nil {
		t.Fatal(err)
	}
	r := &fakeRepo{grants: g}
	s := newSvc(r, c)

	res, _ := s.State(context.Background(), "u")
	if v := res.Pets["Pet_Raccoon"]; v.FirstMetAt == nil || !v.FirstMetAt.Equal(early) {
		t.Fatalf("raccoon firstMetAt %v, want the earliest grant %v", v.FirstMetAt, early)
	}
	if v := res.Pets["Pet_Tortoise"]; v.FirstMetAt != nil {
		t.Fatalf("starter pet before any bond document: firstMetAt %v, want none", v.FirstMetAt)
	}
	play(t, s, "Pet_Tortoise", 10)
	res, _ = s.State(context.Background(), "u")
	if v := res.Pets["Pet_Tortoise"]; v.FirstMetAt == nil || !v.FirstMetAt.Equal(t0) {
		t.Fatalf("starter firstMetAt %v, want the bond document's creation %v", v.FirstMetAt, t0)
	}
}

func TestReportValidation(t *testing.T) {
	s := newSvc(&fakeRepo{}, &clock{now: t0})
	for name, req := range map[string]ReportRequest{
		"short batch id":   {BatchID: "x", Entries: []Entry{{Pet: "Pet_Tortoise"}}},
		"no entries":       {BatchID: "batch-0001"},
		"bad pet":          {BatchID: "batch-0001", Entries: []Entry{{Pet: "Tortoise"}}},
		"negative seconds": {BatchID: "batch-0001", Entries: []Entry{{Pet: "Pet_Tortoise", Seconds: -1}}},
	} {
		if _, err := s.Report(context.Background(), "u", req); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: err %v, want ErrInvalidRequest", name, err)
		}
	}
}

func TestReportNeverLowersHearts(t *testing.T) {
	c := &clock{now: t0}
	r := &fakeRepo{}
	s := newSvc(r, c)
	days := 5
	if _, err := s.AdminSet(context.Background(), AdminSetRequest{UID: "u", Pet: "Pet_Tortoise", BondDays: &days}); err != nil {
		t.Fatal(err)
	}
	c.Advance(time.Minute)
	if got := play(t, s, "Pet_Tortoise", 30).Pets["Pet_Tortoise"]; got.BondDays != 5 || got.Level != 2 {
		t.Fatalf("after a report: %+v, want 5 hearts at level 2", got)
	}
}

func TestAdminSetAndClear(t *testing.T) {
	c := &clock{now: t0}
	r := &fakeRepo{}
	s := newSvc(r, c)
	c.Advance(10 * time.Minute)
	play(t, s, "Pet_Tortoise", 300)
	res, err := s.AdminSet(context.Background(), AdminSetRequest{UID: "u", Pet: "Pet_Tortoise", ClearLastBondAt: true})
	if err != nil {
		t.Fatal(err)
	}
	if v := res.Pets["Pet_Tortoise"]; v.BondDays != 1 || v.NextHeartInSeconds != 0 || v.LastBondAt != nil {
		t.Fatalf("after clear: %+v, want 1 heart and no cooldown", v)
	}
	for name, req := range map[string]AdminSetRequest{
		"no uid":      {Pet: "Pet_Tortoise", ClearLastBondAt: true},
		"unknown pet": {UID: "u", Pet: "Pet_Nope", ClearLastBondAt: true},
		"nothing":     {UID: "u", Pet: "Pet_Tortoise"},
	} {
		if _, err := s.AdminSet(context.Background(), req); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: err %v, want ErrInvalidRequest", name, err)
		}
	}
}

func TestLevelFor(t *testing.T) {
	for days, want := range map[int]int{0: 0, 1: 1, 2: 1, 3: 2, 7: 3, 13: 3, 14: 4, 30: 5, 100: 5} {
		if got := LevelFor(days); got != want {
			t.Errorf("LevelFor(%d) = %d, want %d", days, got, want)
		}
	}
}
