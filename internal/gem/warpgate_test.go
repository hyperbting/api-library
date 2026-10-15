package gem

import (
	"context"
	"errors"
	"testing"
)

// stubGate counts how often it is asked and answers from fields.
type stubGate struct {
	unlocked bool
	err      error
	asked    int
}

func (g *stubGate) WarpUnlocked(context.Context, string) (bool, error) {
	g.asked++
	return g.unlocked, g.err
}

func gatedFixture(g *stubGate) *fixture {
	f := newFixture()
	f.srv.gate = g
	return f
}

func TestGateRefusesAGemWarpThatClaimsTheFlag(t *testing.T) {
	g := &stubGate{unlocked: false}
	f := gatedFixture(g)
	f.give("u1", "iap:1", 100)

	r := f.spend(t, "u1", ProductWarp4h, "req-0001", true)
	if r.OK || r.ErrorCode != CodeWarpNotEnabled {
		t.Fatalf("locked warp: ok=%v code=%q, want warp_not_enabled", r.OK, r.ErrorCode)
	}
	if r.Grant != nil {
		t.Errorf("a refused warp left a grant: %+v", r.Grant)
	}
	if r.State.Balance != 100 || r.State.WarpHoursUsed != 0 {
		t.Errorf("a refused warp changed the state: %+v", r.State)
	}
}

func TestGateOpenLetsAGemWarpThrough(t *testing.T) {
	g := &stubGate{unlocked: true}
	f := gatedFixture(g)
	f.give("u1", "iap:1", 100)

	r := f.spend(t, "u1", ProductWarp4h, "req-0001", true)
	if !r.OK || r.Grant == nil || r.State.Balance != 80 {
		t.Fatalf("unlocked warp: %+v", r)
	}
}

func TestGateDoesNotLetAnUnclaimedFlagThrough(t *testing.T) {
	// The client's own flag still has to be true: the gate only adds to it.
	g := &stubGate{unlocked: true}
	f := gatedFixture(g)
	f.give("u1", "iap:1", 100)

	r := f.spend(t, "u1", ProductWarp4h, "req-0001", false)
	if r.OK || r.ErrorCode != CodeWarpNotEnabled {
		t.Fatalf("flag false: ok=%v code=%q", r.OK, r.ErrorCode)
	}
	if g.asked != 0 {
		t.Errorf("the gate was read %d times for a request that did not claim the flag, want 0", g.asked)
	}
}

func TestGateIsNotAskedForBoostsAndFever(t *testing.T) {
	g := &stubGate{unlocked: false}
	f := gatedFixture(g)
	f.give("u1", "iap:1", 100)

	for i, p := range []Product{ProductBoost10m, ProductBoost30m, ProductFever} {
		if r := f.spend(t, "u1", p, "req-000"+string(rune('1'+i)), false); !r.OK {
			t.Errorf("%s with a locked gate: %+v", p, r)
		}
	}
	if g.asked != 0 {
		t.Errorf("the gate was read %d times for products that are not warps, want 0", g.asked)
	}
}

func TestGateRefusesAnAdWarpButNotOtherPlacements(t *testing.T) {
	g := &stubGate{unlocked: false}
	f := gatedFixture(g)

	r := f.ad(t, "u1", PlacementWarp, "ad-req-0001", "", true)
	if r.OK || r.ErrorCode != CodeWarpNotEnabled || r.Grant != nil {
		t.Fatalf("locked ad warp: %+v", r)
	}
	if r := f.ad(t, "u1", PlacementFever, "ad-req-0002", "corner", false); !r.OK {
		t.Errorf("a fever ad needs no warp gate: %+v", r)
	}
	if r := f.ad(t, "u1", PlacementOfflineDouble, "ad-req-0003", "", false); !r.OK {
		t.Errorf("an offline double needs no warp gate: %+v", r)
	}

	g.unlocked = true
	if r := f.ad(t, "u1", PlacementWarp, "ad-req-0004", "", true); !r.OK || r.Grant == nil {
		t.Errorf("unlocked ad warp: %+v", r)
	}
}

func TestGateErrorChargesAndGrantsNothing(t *testing.T) {
	boom := errors.New("quests unavailable")
	g := &stubGate{err: boom}
	f := gatedFixture(g)
	f.give("u1", "iap:1", 100)

	if _, err := f.srv.Spend(f.ctx, "u1", SpendRequest{Product: ProductWarp4h, RequestID: "req-0001", WarpEnabled: true}); !errors.Is(err, boom) {
		t.Fatalf("spend error = %v, want the gate's", err)
	}
	if _, err := f.srv.AdReward(f.ctx, "u1", AdRewardRequest{Placement: PlacementWarp, RequestID: "ad-req-0001", WarpEnabled: true}); !errors.Is(err, boom) {
		t.Fatalf("ad error = %v, want the gate's", err)
	}
	g.err = nil
	g.unlocked = true
	if st := f.state(t, "u1"); st.Balance != 100 || st.WarpHoursUsed != 0 {
		t.Errorf("a failed gate changed the state: %+v", st)
	}
}

func TestGateDoesNotBreakARetriedPurchase(t *testing.T) {
	// The warp was bought while unlocked; the quest is reset later. The same request ID is still answered as the
	// purchase it was (nothing charged twice), not as a refusal.
	g := &stubGate{unlocked: true}
	f := gatedFixture(g)
	f.give("u1", "iap:1", 100)

	first := f.spend(t, "u1", ProductWarp4h, "req-0001", true)
	if !first.OK {
		t.Fatalf("first: %+v", first)
	}
	g.unlocked = false
	again := f.spend(t, "u1", ProductWarp4h, "req-0001", true)
	if !again.OK || again.State.Balance != first.State.Balance {
		t.Errorf("retry after a reset: ok=%v balance=%d, want ok at %d", again.OK, again.State.Balance, first.State.Balance)
	}
}

func TestNilGateKeepsTheClientsFlagAsTheOnlyCheck(t *testing.T) {
	f := newFixture() // no gate
	f.give("u1", "iap:1", 100)
	if r := f.spend(t, "u1", ProductWarp4h, "req-0001", true); !r.OK {
		t.Errorf("flag true without a gate: %+v", r)
	}
	if r := f.spend(t, "u1", ProductWarp4h, "req-0002", false); r.OK || r.ErrorCode != CodeWarpNotEnabled {
		t.Errorf("flag false without a gate: ok=%v code=%q", r.OK, r.ErrorCode)
	}
}
