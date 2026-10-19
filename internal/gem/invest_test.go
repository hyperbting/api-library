package gem

import "testing"

func TestInvestLevelsCostAndHours(t *testing.T) {
	f := newFixture()
	f.give("u1", "g1", 200)
	st := f.state(t, "u1")
	if st.InvestLevel != 0 || st.InvestAdHours != 1.0 || st.InvestNextPrice != 25 || st.InvestNextAdHours != 1.5 || st.InvestMaxLevel != 3 {
		t.Fatalf("start state: %+v", st)
	}
	wantHours := []float64{1.5, 2.0, 2.5}
	wantNext := []int64{50, 75, 0}
	balance := int64(200)
	for i, price := range []int64{25, 50, 75} {
		res := f.spend(t, "u1", ProductInvest, "invest-req-"+string(rune('a'+i))+"0000", false)
		if !res.OK || res.Grant != nil {
			t.Fatalf("level %d: %+v", i+1, res)
		}
		balance -= price
		s := res.State
		if s.InvestLevel != i+1 || s.InvestAdHours != wantHours[i] || s.InvestNextPrice != wantNext[i] || s.Balance != balance {
			t.Errorf("after level %d: %+v (balance want %d)", i+1, s, balance)
		}
	}
	// The top: refused, nothing charged.
	res := f.spend(t, "u1", ProductInvest, "invest-req-z0000", false)
	if res.OK || res.ErrorCode != CodeInvestMax || res.State.Balance != balance {
		t.Errorf("past the cap: %+v", res)
	}
}

func TestInvestNeedsGemsAndIsIdempotent(t *testing.T) {
	f := newFixture()
	f.give("u1", "g1", 24)
	if res := f.spend(t, "u1", ProductInvest, "invest-req-0001", false); res.OK || res.ErrorCode != CodeInsufficientGems {
		t.Fatalf("24 gems bought level 1: %+v", res)
	}
	f.give("u1", "g2", 1)
	a := f.spend(t, "u1", ProductInvest, "invest-req-0002", false)
	b := f.spend(t, "u1", ProductInvest, "invest-req-0002", false) // a retry of the same purchase
	if !a.OK || !b.OK {
		t.Fatalf("a=%+v b=%+v", a, b)
	}
	if st := f.state(t, "u1"); st.InvestLevel != 1 || st.Balance != 0 {
		t.Errorf("a retry charged twice: %+v", st)
	}
}

func TestAdWarpPaysTheInvestmentHours(t *testing.T) {
	f := newFixture()
	f.give("u1", "g1", 100)
	f.spend(t, "u1", ProductInvest, "invest-req-0001", false)
	f.spend(t, "u1", ProductInvest, "invest-req-0002", false) // level 2: 2.0 h
	res := f.ad(t, "u1", PlacementWarp, "ad-req-0001", "", true)
	if !res.OK || res.Grant == nil || res.Grant.Rewards[0].Amount != 2.0 {
		t.Fatalf("ad warp at level 2: %+v", res)
	}
	if got := f.state(t, "u1").WarpHoursUsed; got != 2.0 {
		t.Errorf("hours used = %v, want 2", got)
	}
}

func TestAdWarpPaysWhatIsLeftOfTheDay(t *testing.T) {
	f := newFixture()
	f.give("u1", "g1", 200)
	for _, id := range []string{"invest-req-0001", "invest-req-0002", "invest-req-0003"} {
		f.spend(t, "u1", ProductInvest, id, false) // level 3: 2.5 h
	}
	var paid []float64
	for _, id := range []string{"ad-req-0001", "ad-req-0002", "ad-req-0003", "ad-req-0004", "ad-req-0005"} {
		res := f.ad(t, "u1", PlacementWarp, id, "", true)
		if res.OK {
			paid = append(paid, res.Grant.Rewards[0].Amount)
		}
	}
	// 2.5 + 2.5 + 2.5 = 7.5, the fourth ad pays the 0.5 left, the fifth is refused.
	if len(paid) != 4 || paid[0] != 2.5 || paid[3] != 0.5 {
		t.Fatalf("paid = %v, want [2.5 2.5 2.5 0.5]", paid)
	}
	if got := f.state(t, "u1").WarpHoursUsed; got != 8.0 {
		t.Errorf("hours used = %v, want 8", got)
	}
}

func TestAdWarpRemainderCountsDoubleWhileTheBoostRuns(t *testing.T) {
	f := newFixture()
	f.give("u1", "g1", 200)
	for _, id := range []string{"invest-req-0001", "invest-req-0002", "invest-req-0003"} {
		f.spend(t, "u1", ProductInvest, id, false)
	}
	f.spend(t, "u1", ProductBoost10m, "boost-req-0001", false)
	// Boost on: 2.5 h counts as 5. 5 + (3 left / 2 = 1.5 h counted as 3) = 8.
	a := f.ad(t, "u1", PlacementWarp, "ad-req-0001", "", true)
	b := f.ad(t, "u1", PlacementWarp, "ad-req-0002", "", true)
	if !a.OK || a.Grant.Rewards[0].Amount != 2.5 || !b.OK || b.Grant.Rewards[0].Amount != 1.5 {
		t.Fatalf("a=%+v b=%+v", a, b)
	}
	if c := f.ad(t, "u1", PlacementWarp, "ad-req-0003", "", true); c.OK || c.ErrorCode != CodeWarpLimit {
		t.Errorf("past the allowance: %+v", c)
	}
}
