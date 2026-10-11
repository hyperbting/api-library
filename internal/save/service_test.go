package save

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"api-library/internal/grant"
)

// memRepo mimics the Firestore repository: Update works on copies and keeps them only when fn asks.
type memRepo struct {
	saves  map[string]*Doc
	grants map[string]*grant.Doc
	writes int
}

func newMemRepo() *memRepo {
	return &memRepo{saves: map[string]*Doc{}, grants: map[string]*grant.Doc{}}
}

func cloneGrants(d *grant.Doc) *grant.Doc {
	if d == nil {
		return grant.NewDoc()
	}
	c := *d
	c.Grants = map[string]*grant.Grant{}
	for k, g := range d.Grants {
		v := *g
		c.Grants[k] = &v
	}
	return &c
}

func (r *memRepo) Get(_ context.Context, uid string) (*Doc, *grant.Doc, error) {
	var doc *Doc
	if d := r.saves[uid]; d != nil {
		c := *d
		doc = &c
	}
	return doc, cloneGrants(r.grants[uid]), nil
}

func (r *memRepo) Update(_ context.Context, uid string, fn func(*Doc, *grant.Doc) (bool, bool, error)) error {
	doc := &Doc{SchemaVersion: SchemaVersion}
	if d := r.saves[uid]; d != nil {
		c := *d
		doc = &c
	}
	grants := cloneGrants(r.grants[uid])
	writeSave, writeGrants, err := fn(doc, grants)
	if err != nil {
		return err
	}
	if writeSave {
		r.saves[uid] = doc
		r.writes++
	}
	if writeGrants {
		r.grants[uid] = grants
	}
	return nil
}

func (r *memRepo) Delete(_ context.Context, uid string) error {
	delete(r.saves, uid)
	return nil
}

var now = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

func newTestService() (Service, *memRepo) {
	repo := newMemRepo()
	s := NewService(repo).(*service)
	s.now = func() time.Time { return now }
	return s, repo
}

func req(baseRev int64, lifetime float64) SyncRequest {
	return SyncRequest{
		BaseRev:      baseRev,
		Data:         `{"cash":1}`,
		Summary:      Summary{Cash: 1, LifetimeEarned: lifetime, Venues: map[string]int{"corner_convenience": 3}},
		ClientTimeMs: now.UnixMilli(),
	}
}

func TestLoadFreshAccount(t *testing.T) {
	s, _ := newTestService()
	res, err := s.Load(context.Background(), "u")
	if err != nil {
		t.Fatal(err)
	}
	if res.State != nil || len(res.Grants) != 0 {
		t.Errorf("fresh account: %+v", res)
	}
	if res.ServerNowMs != now.UnixMilli() {
		t.Errorf("serverNowMs = %d", res.ServerNowMs)
	}
}

func TestSyncAcceptsAndIncrementsRev(t *testing.T) {
	s, repo := newTestService()
	ctx := context.Background()

	res, err := s.Sync(ctx, "u", req(0, 100))
	if err != nil || !res.OK || res.Rev != 1 {
		t.Fatalf("first sync: %+v %v", res, err)
	}
	res, err = s.Sync(ctx, "u", req(1, 150))
	if err != nil || !res.OK || res.Rev != 2 {
		t.Fatalf("second sync: %+v %v", res, err)
	}
	if got := repo.saves["u"]; got.Rev != 2 || got.Summary.LifetimeEarned != 150 || got.Data != `{"cash":1}` {
		t.Errorf("stored = %+v", got)
	}

	load, _ := s.Load(ctx, "u")
	if load.State == nil || load.State.Rev != 2 || load.State.Summary.Venues["corner_convenience"] != 3 {
		t.Errorf("load = %+v", load.State)
	}
}

func TestSyncStaleRevIsRejectedWithServerState(t *testing.T) {
	s, repo := newTestService()
	ctx := context.Background()
	s.Sync(ctx, "u", req(0, 100))
	s.Sync(ctx, "u", req(1, 200)) // another device moved the rev on

	res, err := s.Sync(ctx, "u", req(1, 999)) // this device still thinks rev is 1
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.ErrorCode != CodeRevConflict || res.Rev != 2 || res.Server == nil || res.Server.Summary.LifetimeEarned != 200 {
		t.Errorf("conflict response = %+v", res)
	}
	if repo.saves["u"].Summary.LifetimeEarned != 200 {
		t.Error("a rejected write changed the stored save")
	}
}

func TestSyncFreshDeviceCannotOverwriteExistingSave(t *testing.T) {
	s, _ := newTestService()
	ctx := context.Background()
	s.Sync(ctx, "u", req(0, 5000))

	res, _ := s.Sync(ctx, "u", req(0, 10)) // a reinstalled device that never loaded
	if res.OK || res.ErrorCode != CodeRevConflict {
		t.Errorf("fresh device overwrote the save: %+v", res)
	}
}

func TestSyncRejectsRegressingProgress(t *testing.T) {
	s, repo := newTestService()
	ctx := context.Background()
	s.Sync(ctx, "u", req(0, 5000))

	res, _ := s.Sync(ctx, "u", req(1, 40)) // right rev, but an empty default save
	if res.OK || res.ErrorCode != CodeRegressed || res.Server == nil || res.Server.Rev != 1 {
		t.Errorf("regression response = %+v", res)
	}
	if repo.saves["u"].Summary.LifetimeEarned != 5000 {
		t.Error("regressing write was stored")
	}

	// Same value is fine (nothing earned between two syncs).
	if res, _ = s.Sync(ctx, "u", req(1, 5000)); !res.OK {
		t.Errorf("equal lifetime refused: %+v", res)
	}
}

func TestSyncClaimsGrantsAtomically(t *testing.T) {
	s, repo := newTestService()
	ctx := context.Background()

	g := grant.NewDoc()
	g.Add(grant.Grant{ID: "redeem:AAAA", Source: grant.SourceRedeem, Rewards: []grant.Reward{{Type: grant.RewardPull, Amount: 2}}, CreatedAt: now})
	g.Add(grant.Grant{ID: "redeem:BBBB", Source: grant.SourceRedeem, Rewards: []grant.Reward{{Type: grant.RewardCash, Amount: 5}}, CreatedAt: now.Add(time.Second)})
	repo.grants["u"] = g

	load, _ := s.Load(ctx, "u")
	if len(load.Grants) != 2 {
		t.Fatalf("load grants = %d", len(load.Grants))
	}

	r := req(0, 10)
	r.ClaimedGrants = []string{"redeem:AAAA"}
	res, err := s.Sync(ctx, "u", r)
	if err != nil || !res.OK {
		t.Fatalf("sync: %+v %v", res, err)
	}
	if len(res.Grants) != 1 || res.Grants[0].ID != "redeem:BBBB" {
		t.Errorf("grants after claim = %+v", res.Grants)
	}
	if repo.grants["u"].Grants["redeem:AAAA"].ClaimedAt == nil {
		t.Error("claim was not stored")
	}
}

func TestRejectedSyncDoesNotClaim(t *testing.T) {
	s, repo := newTestService()
	ctx := context.Background()
	s.Sync(ctx, "u", req(0, 100))
	g := grant.NewDoc()
	g.Add(grant.Grant{ID: "redeem:AAAA", CreatedAt: now})
	repo.grants["u"] = g

	r := req(0, 100) // stale
	r.ClaimedGrants = []string{"redeem:AAAA"}
	res, _ := s.Sync(ctx, "u", r)
	if res.OK {
		t.Fatal("stale write accepted")
	}
	if repo.grants["u"].Grants["redeem:AAAA"].ClaimedAt != nil {
		t.Error("a rejected sync claimed a grant (the snapshot holding its rewards was not stored)")
	}
	if len(res.Grants) != 1 {
		t.Errorf("rejected response should still list waiting grants, got %d", len(res.Grants))
	}
}

func TestSyncValidation(t *testing.T) {
	s, _ := newTestService()
	ctx := context.Background()

	mutate := func(f func(r *SyncRequest)) SyncRequest {
		r := req(0, 1)
		f(&r)
		return r
	}
	cases := map[string]SyncRequest{
		"negative rev":      mutate(func(r *SyncRequest) { r.BaseRev = -1 }),
		"empty data":        mutate(func(r *SyncRequest) { r.Data = "" }),
		"data not object":   mutate(func(r *SyncRequest) { r.Data = `[1,2]` }),
		"data not json":     mutate(func(r *SyncRequest) { r.Data = `{oops` }),
		"data too large":    mutate(func(r *SyncRequest) { r.Data = `{"x":"` + strings.Repeat("a", MaxDataBytes) + `"}` }),
		"negative cash":     mutate(func(r *SyncRequest) { r.Summary.Cash = -1 }),
		"negative lifetime": mutate(func(r *SyncRequest) { r.Summary.LifetimeEarned = -1 }),
		"venue level":       mutate(func(r *SyncRequest) { r.Summary.Venues["x"] = MaxVenueLevel + 1 }),
		"empty venue id":    mutate(func(r *SyncRequest) { r.Summary.Venues[""] = 1 }),
		"too many claims": mutate(func(r *SyncRequest) {
			r.ClaimedGrants = make([]string, MaxClaimedGrants+1)
			for i := range r.ClaimedGrants {
				r.ClaimedGrants[i] = "g"
			}
		}),
	}
	for name, r := range cases {
		if _, err := s.Sync(ctx, "u", r); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: want ErrInvalidRequest, got %v", name, err)
		}
	}
}

func TestResetDropsSaveKeepsGrants(t *testing.T) {
	s, repo := newTestService()
	ctx := context.Background()
	s.Sync(ctx, "u", req(0, 100))
	g := grant.NewDoc()
	g.Add(grant.Grant{ID: "redeem:AAAA", CreatedAt: now})
	repo.grants["u"] = g

	if err := s.Reset(ctx, "u"); err != nil {
		t.Fatal(err)
	}
	if repo.saves["u"] != nil {
		t.Error("save not deleted")
	}
	if len(repo.grants["u"].Grants) != 1 {
		t.Error("reset must not touch grants (they keep redeems idempotent)")
	}
	if res, _ := s.Sync(ctx, "u", req(0, 1)); !res.OK || res.Rev != 1 {
		t.Errorf("sync after reset: %+v", res)
	}
}
