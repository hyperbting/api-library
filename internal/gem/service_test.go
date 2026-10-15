package gem

import (
	"context"
	"testing"
	"time"

	"api-library/internal/grant"
)

// memRepo mimics the Firestore repository: Run works on copies and keeps them only when fn asks.
type memRepo struct {
	gems   map[string]*Doc
	grants map[string]*grant.Doc
}

func newMemRepo() *memRepo {
	return &memRepo{gems: map[string]*Doc{}, grants: map[string]*grant.Doc{}}
}

func (r *memRepo) Run(_ context.Context, uid string, fn func(*Doc, *grant.Doc) (bool, bool, error)) error {
	gems := NewDoc()
	if d := r.gems[uid]; d != nil {
		v := *d
		v.Credited = map[string]time.Time{}
		for k, t := range d.Credited {
			v.Credited[k] = t
		}
		v.Spends = map[string]*SpendRecord{}
		for k, rec := range d.Spends {
			c := *rec
			v.Spends[k] = &c
		}
		gems = &v
	}
	grants := grant.NewDoc()
	if d := r.grants[uid]; d != nil {
		v := *d
		v.Grants = map[string]*grant.Grant{}
		for k, g := range d.Grants {
			c := *g
			c.Rewards = append([]grant.Reward(nil), g.Rewards...)
			v.Grants[k] = &c
		}
		grants = &v
	}
	writeGems, writeGrants, err := fn(gems, grants)
	if err != nil {
		return err
	}
	if writeGems {
		r.gems[uid] = gems
	}
	if writeGrants {
		r.grants[uid] = grants
	}
	return nil
}

var clock = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

type fixture struct {
	srv  *service
	repo *memRepo
	ctx  context.Context
	now  time.Time
}

func newFixture() *fixture {
	repo := newMemRepo()
	f := &fixture{repo: repo, ctx: context.Background(), now: clock}
	f.srv = NewService(repo, nil).(*service)
	f.srv.now = func() time.Time { return f.now }
	return f
}

// give puts a Gem grant in the user's ledger, as an IAP receipt or a redeem code would.
func (f *fixture) give(uid, id string, gems float64) {
	doc := f.repo.grants[uid]
	if doc == nil {
		doc = grant.NewDoc()
		f.repo.grants[uid] = doc
	}
	doc.Grants[id] = &grant.Grant{
		ID: id, Source: grant.SourceRedeem, SourceRef: id, CreatedAt: f.now,
		Rewards: []grant.Reward{{Type: grant.RewardGem, Amount: gems}},
	}
}

func (f *fixture) spend(t *testing.T, uid string, p Product, reqID string, warpEnabled bool) *SpendResponse {
	t.Helper()
	res, err := f.srv.Spend(f.ctx, uid, SpendRequest{Product: p, RequestID: reqID, WarpEnabled: warpEnabled})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (f *fixture) state(t *testing.T, uid string) *State {
	t.Helper()
	st, err := f.srv.State(f.ctx, uid)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestStateCreditsGemGrantsOnce(t *testing.T) {
	f := newFixture()
	f.give("u1", "redeem:A", 100)
	f.give("u1", "iap:1", 550)

	if got := f.state(t, "u1").Balance; got != 650 {
		t.Fatalf("balance = %d, want 650", got)
	}
	// Reading again credits nothing more.
	if got := f.state(t, "u1").Balance; got != 650 {
		t.Fatalf("second read balance = %d, want 650", got)
	}
	// A new grant is added on top.
	f.give("u1", "redeem:B", 25)
	if got := f.state(t, "u1").Balance; got != 675 {
		t.Fatalf("balance after a new grant = %d, want 675", got)
	}
}

func TestStateIgnoresOtherRewards(t *testing.T) {
	f := newFixture()
	f.repo.grants["u1"] = grant.NewDoc()
	f.repo.grants["u1"].Grants["redeem:C"] = &grant.Grant{
		ID: "redeem:C", CreatedAt: f.now,
		Rewards: []grant.Reward{{Type: grant.RewardCash, Amount: 1000}, {Type: grant.RewardTimeWarp, Amount: 2}},
	}
	if got := f.state(t, "u1").Balance; got != 0 {
		t.Fatalf("balance = %d, want 0", got)
	}
}

func TestSpendWarpTakesGemsAndAllowance(t *testing.T) {
	f := newFixture()
	f.give("u1", "iap:1", 100)

	res := f.spend(t, "u1", ProductWarp4h, "req-0001", true)
	if !res.OK || res.Grant == nil {
		t.Fatalf("spend refused: %+v", res)
	}
	if res.State.Balance != 80 {
		t.Errorf("balance = %d, want 80", res.State.Balance)
	}
	if res.State.WarpHoursUsed != 4 || res.State.WarpHoursLeft != 4 {
		t.Errorf("allowance used/left = %v/%v, want 4/4", res.State.WarpHoursUsed, res.State.WarpHoursLeft)
	}
	r := res.Grant.Rewards
	if len(r) != 1 || r[0].Type != grant.RewardTimeWarp || r[0].Amount != 4 {
		t.Errorf("grant rewards = %+v", r)
	}
	if res.Grant.Source != grant.SourceGems || res.Grant.ID != "gem:req-0001" {
		t.Errorf("grant = %+v", res.Grant)
	}
	// The grant sits in the ledger for the client to apply.
	if !f.repo.grants["u1"].Has("gem:req-0001") {
		t.Error("grant not stored")
	}
}

func TestSpendIsIdempotentByRequestID(t *testing.T) {
	f := newFixture()
	f.give("u1", "iap:1", 100)
	first := f.spend(t, "u1", ProductWarp4h, "req-0001", true)
	again := f.spend(t, "u1", ProductWarp4h, "req-0001", true)

	if !first.OK || !again.OK {
		t.Fatalf("first=%+v again=%+v", first, again)
	}
	if again.State.Balance != 80 || again.State.WarpHoursUsed != 4 {
		t.Errorf("retried request charged again: %+v", again.State)
	}
	if again.Grant == nil || again.Grant.ID != first.Grant.ID {
		t.Errorf("retry grant = %+v", again.Grant)
	}
}

func TestSpendRefusals(t *testing.T) {
	f := newFixture()
	f.give("u1", "iap:1", 30)

	cases := []struct {
		name string
		p    Product
		req  string
		warp bool
		want string
	}{
		{"unknown", "warp_99h", "r-unknown", true, CodeUnknownProduct},
		{"warp not enabled here", ProductWarp4h, "r-nowarp", false, CodeWarpNotEnabled},
		{"not enough gems", ProductWarp8h, "r-poor", true, CodeInsufficientGems},
	}
	for _, c := range cases {
		res := f.spend(t, "u1", c.p, c.req, c.warp)
		if res.OK || res.ErrorCode != c.want {
			t.Errorf("%s: ok=%v code=%q, want %q", c.name, res.OK, res.ErrorCode, c.want)
		}
		if res.State.Balance != 30 {
			t.Errorf("%s: a refused spend changed the balance to %d", c.name, res.State.Balance)
		}
	}
}

func TestDailyWarpLimitAndReset(t *testing.T) {
	f := newFixture()
	f.give("u1", "iap:1", 1000)

	if r := f.spend(t, "u1", ProductWarp8h, "r-1", true); !r.OK {
		t.Fatalf("first 8 h warp refused: %+v", r)
	}
	r := f.spend(t, "u1", ProductWarp4h, "r-2", true)
	if r.OK || r.ErrorCode != CodeWarpLimit {
		t.Fatalf("second warp: ok=%v code=%q, want warp_limit", r.OK, r.ErrorCode)
	}
	if r.State.WarpHoursLeft != 0 {
		t.Errorf("hours left = %v, want 0", r.State.WarpHoursLeft)
	}
	if r.State.Balance != 1000-37 {
		t.Errorf("balance = %d", r.State.Balance)
	}

	// The next server day starts the allowance again.
	f.now = f.now.Add(24 * time.Hour)
	if st := f.state(t, "u1"); st.WarpHoursLeft != 8 || st.WarpHoursUsed != 0 {
		t.Errorf("after reset used/left = %v/%v", st.WarpHoursUsed, st.WarpHoursLeft)
	}
	if r := f.spend(t, "u1", ProductWarp4h, "r-3", true); !r.OK {
		t.Errorf("warp after the reset refused: %+v", r)
	}
}

func TestBoostExtendsAndDoublesTheWarpAllowance(t *testing.T) {
	f := newFixture()
	f.give("u1", "iap:1", 1000)

	b := f.spend(t, "u1", ProductBoost10m, "b-1", false)
	if !b.OK || b.State.BoostSeconds != 600 {
		t.Fatalf("boost: %+v", b)
	}
	b = f.spend(t, "u1", ProductBoost30m, "b-2", false)
	if !b.OK || b.State.BoostSeconds != 600+1800 {
		t.Fatalf("second boost should extend: %+v", b.State)
	}
	if b.State.Balance != 1000-5-13 {
		t.Errorf("balance = %d", b.State.Balance)
	}

	// With the boost running a 4 h warp counts as 8 h: one fits, a second does not.
	w := f.spend(t, "u1", ProductWarp4h, "w-1", true)
	if !w.OK || w.State.WarpHoursUsed != 8 {
		t.Fatalf("warp during boost: ok=%v used=%v", w.OK, w.State.WarpHoursUsed)
	}
	// The grant still carries 4 h: the client's income is already doubled by the boost.
	if w.Grant.Rewards[0].Amount != 4 {
		t.Errorf("grant hours = %v, want 4", w.Grant.Rewards[0].Amount)
	}
	w = f.spend(t, "u1", ProductWarp4h, "w-2", true)
	if w.OK || w.ErrorCode != CodeWarpLimit {
		t.Errorf("second warp: ok=%v code=%q", w.OK, w.ErrorCode)
	}
}

func TestBoostCap(t *testing.T) {
	f := newFixture()
	f.give("u1", "iap:1", 10000)
	var last *SpendResponse
	for i := 0; i < 20; i++ {
		last = f.spend(t, "u1", ProductBoost30m, "cap-"+string(rune('a'+i)), false)
		if !last.OK {
			break
		}
	}
	if last.OK || last.ErrorCode != CodeBoostFull {
		t.Fatalf("expected boost_full, got ok=%v code=%q", last.OK, last.ErrorCode)
	}
	if last.State.BoostSeconds != MaxBoostSeconds {
		t.Errorf("boost = %d, want the cap %d", last.State.BoostSeconds, MaxBoostSeconds)
	}
}

func TestConsumeBoost(t *testing.T) {
	f := newFixture()
	f.give("u1", "iap:1", 100)
	f.spend(t, "u1", ProductBoost10m, "b-1", false) // 600 s

	st, err := f.srv.ConsumeBoost(f.ctx, "u1", 250)
	if err != nil || st.BoostSeconds != 350 {
		t.Fatalf("after 250 s: %+v err=%v", st, err)
	}
	// More than is left empties it, never below zero.
	st, err = f.srv.ConsumeBoost(f.ctx, "u1", 3000)
	if err != nil || st.BoostSeconds != 0 {
		t.Fatalf("after 3000 s: %+v err=%v", st, err)
	}
	// Without a boost the report changes nothing.
	st, err = f.srv.ConsumeBoost(f.ctx, "u1", 60)
	if err != nil || st.BoostSeconds != 0 {
		t.Fatalf("no boost: %+v err=%v", st, err)
	}
}

func TestSpendIsPerUser(t *testing.T) {
	f := newFixture()
	f.give("u1", "iap:1", 100)
	if r := f.spend(t, "u2", ProductWarp4h, "r-1", true); r.OK || r.ErrorCode != CodeInsufficientGems {
		t.Errorf("u2 spent u1's gems: %+v", r)
	}
}

func TestDayKeyAndNextReset(t *testing.T) {
	a := time.Date(2026, 10, 15, 23, 59, 0, 0, time.UTC)
	b := time.Date(2026, 10, 16, 0, 1, 0, 0, time.UTC)
	if DayKey(a) == DayKey(b) {
		t.Error("the day did not change at midnight UTC")
	}
	if got := NextReset(a); !got.Equal(time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("NextReset = %v", got)
	}
}

func TestStateGivesTheResetCountdown(t *testing.T) {
	f := newFixture()
	// 12:00 UTC: twelve hours to the next midnight.
	if got := f.state(t, "u1").WarpResetsInSeconds; got != 12*3600 {
		t.Errorf("countdown at noon = %d, want %d", got, 12*3600)
	}
	f.now = time.Date(2026, 10, 15, 23, 59, 30, 0, time.UTC)
	if got := f.state(t, "u1").WarpResetsInSeconds; got != 30 {
		t.Errorf("countdown at 23:59:30 = %d, want 30", got)
	}
	// Exactly at midnight a new day has started: a full day to go, never zero.
	f.now = time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	if got := f.state(t, "u1").WarpResetsInSeconds; got != 24*3600 {
		t.Errorf("countdown at midnight = %d, want %d", got, 24*3600)
	}
}

func (f *fixture) ad(t *testing.T, uid string, p Placement, reqID, store string, warpEnabled bool) *AdRewardResponse {
	t.Helper()
	res, err := f.srv.AdReward(f.ctx, uid, AdRewardRequest{Placement: p, RequestID: reqID, Store: store, WarpEnabled: warpEnabled})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestAdWarpGivesOneHourAndCountsAgainstTheDay(t *testing.T) {
	f := newFixture()
	res := f.ad(t, "u1", PlacementWarp, "ad-req-0001", "", true)
	if !res.OK || res.Grant == nil {
		t.Fatalf("ad warp: %+v", res)
	}
	if res.Grant.Source != grant.SourceAds || res.Grant.ID != "ad:ad-req-0001" {
		t.Errorf("grant = %+v", res.Grant)
	}
	if len(res.Grant.Rewards) != 1 || res.Grant.Rewards[0].Type != grant.RewardTimeWarp || res.Grant.Rewards[0].Amount != 1 {
		t.Errorf("rewards = %+v, want one TimeWarp hour", res.Grant.Rewards)
	}
	if res.State.WarpHoursUsed != 1 || res.State.WarpHoursLeft != 7 {
		t.Errorf("allowance = used %v left %v, want 1 / 7", res.State.WarpHoursUsed, res.State.WarpHoursLeft)
	}
}

func TestAdWarpSharesTheDayWithGemWarps(t *testing.T) {
	f := newFixture()
	f.give("u1", "iap:1", 100)
	// A 4 h gem warp, then ads: the day has 8 payout hours in all, so only 4 ad hours fit.
	if r := f.spend(t, "u1", ProductWarp4h, "req-0001", true); !r.OK {
		t.Fatalf("gem warp: %+v", r)
	}
	for i := 1; i <= 4; i++ {
		if r := f.ad(t, "u1", PlacementWarp, "ad-req-000"+string(rune('0'+i)), "", true); !r.OK {
			t.Fatalf("ad %d refused: %+v", i, r)
		}
	}
	r := f.ad(t, "u1", PlacementWarp, "ad-req-0005", "", true)
	if r.OK || r.ErrorCode != CodeWarpLimit {
		t.Errorf("the fifth ad hour: ok=%v code=%q, want warp_limit", r.OK, r.ErrorCode)
	}
	if r.Grant != nil {
		t.Errorf("a refused ad left a grant: %+v", r.Grant)
	}

	// Midnight UTC starts the allowance again.
	f.now = time.Date(2026, 10, 16, 0, 1, 0, 0, time.UTC)
	if r := f.ad(t, "u1", PlacementWarp, "ad-req-0006", "", true); !r.OK {
		t.Errorf("after the reset: %+v", r)
	}
}

func TestAdWarpCountsDoubleWhileTheBoostRuns(t *testing.T) {
	f := newFixture()
	f.give("u1", "iap:1", 100)
	if r := f.spend(t, "u1", ProductBoost10m, "req-boost1", true); !r.OK {
		t.Fatalf("boost: %+v", r)
	}
	r := f.ad(t, "u1", PlacementWarp, "ad-req-0001", "", true)
	if !r.OK || r.State.WarpHoursUsed != 2 {
		t.Errorf("ad warp with the boost: ok=%v used=%v, want 2 counted", r.OK, r.State.WarpHoursUsed)
	}
}

func TestAdWarpNeedsTheWarpDecoration(t *testing.T) {
	f := newFixture()
	r := f.ad(t, "u1", PlacementWarp, "ad-req-0001", "", false)
	if r.OK || r.ErrorCode != CodeWarpNotEnabled {
		t.Errorf("without the decoration: ok=%v code=%q", r.OK, r.ErrorCode)
	}
}

func TestAdRewardIsIdempotentByRequestID(t *testing.T) {
	f := newFixture()
	a := f.ad(t, "u1", PlacementWarp, "ad-req-0001", "", true)
	b := f.ad(t, "u1", PlacementWarp, "ad-req-0001", "", true)
	if !a.OK || !b.OK || a.Grant.ID != b.Grant.ID {
		t.Fatalf("a=%+v b=%+v", a, b)
	}
	if got := f.state(t, "u1").WarpHoursUsed; got != 1 {
		t.Errorf("hours used = %v, want 1 (a retry must not count twice)", got)
	}
	if n := len(f.repo.grants["u1"].Grants); n != 1 {
		t.Errorf("grants = %d, want 1", n)
	}
	// The same request ID for another placement is refused, not rewarded.
	c := f.ad(t, "u1", PlacementFever, "ad-req-0001", "store", false)
	if c.OK {
		t.Errorf("a request ID reused for another placement was accepted: %+v", c)
	}
}

func TestFeverAdCooldownIsPerStore(t *testing.T) {
	f := newFixture()
	if r := f.ad(t, "u1", PlacementFever, "ad-req-0001", "Place_CornerConvenience", false); !r.OK {
		t.Fatalf("first fever ad: %+v", r)
	}
	r := f.ad(t, "u1", PlacementFever, "ad-req-0002", "Place_CornerConvenience", false)
	if r.OK || r.ErrorCode != CodeAdCooldown || r.RetryAfterSeconds < 1 || r.RetryAfterSeconds > 301 {
		t.Errorf("within the cooldown: ok=%v code=%q retry=%d", r.OK, r.ErrorCode, r.RetryAfterSeconds)
	}
	// Another store has its own cooldown.
	if r := f.ad(t, "u1", PlacementFever, "ad-req-0003", "Place_Other", false); !r.OK {
		t.Errorf("other store: %+v", r)
	}
	// After the cooldown the same store is open again.
	f.now = f.now.Add(FeverAdCooldown + time.Second)
	if r := f.ad(t, "u1", PlacementFever, "ad-req-0004", "Place_CornerConvenience", false); !r.OK {
		t.Errorf("after the cooldown: %+v", r)
	}
}

func TestOfflineDoubleAdIsRecordedWithoutLimit(t *testing.T) {
	f := newFixture()
	for i := 1; i <= 3; i++ {
		r := f.ad(t, "u1", PlacementOfflineDouble, "ad-req-000"+string(rune('0'+i)), "", false)
		if !r.OK || r.Grant != nil {
			t.Fatalf("offline double %d: %+v", i, r)
		}
	}
	if n := len(f.repo.gems["u1"].Ads); n != 3 {
		t.Errorf("recorded ads = %d, want 3", n)
	}
}

func TestUnknownAdPlacement(t *testing.T) {
	f := newFixture()
	r := f.ad(t, "u1", Placement("bonus"), "ad-req-0001", "", false)
	if r.OK || r.ErrorCode != CodeUnknownPlacement {
		t.Errorf("unknown placement: ok=%v code=%q", r.OK, r.ErrorCode)
	}
}

func TestFeverIsFiveGemsAndNeedsNoWarpDecoration(t *testing.T) {
	f := newFixture()
	f.give("u1", "iap:1", 12)
	r := f.spend(t, "u1", ProductFever, "req-fever1", false)
	if !r.OK || r.State.Balance != 7 || r.Grant != nil {
		t.Fatalf("fever: ok=%v balance=%d grant=%v", r.OK, r.State.Balance, r.Grant)
	}
	// A retry is answered, not charged again.
	again := f.spend(t, "u1", ProductFever, "req-fever1", false)
	if !again.OK || again.State.Balance != 7 {
		t.Errorf("retry: ok=%v balance=%d, want 7", again.OK, again.State.Balance)
	}
	// Not enough gems.
	f.spend(t, "u1", ProductFever, "req-fever2", false)
	r3 := f.spend(t, "u1", ProductFever, "req-fever3", false)
	if r3.OK || r3.ErrorCode != CodeInsufficientGems {
		t.Errorf("third fever with 2 gems: ok=%v code=%q", r3.OK, r3.ErrorCode)
	}
}
