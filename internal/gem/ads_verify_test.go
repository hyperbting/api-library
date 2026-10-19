package gem

import (
	"context"
	"errors"
	"testing"
)

// fakeVerifier holds the (uid, requestID) pairs a callback arrived for.
type fakeVerifier struct {
	seen map[string]bool
	err  error
}

func (v *fakeVerifier) Verified(_ context.Context, uid, requestID string) (bool, error) {
	return v.seen[uid+"|"+requestID], v.err
}

func TestAdRewardWaitsForTheVerificationCallback(t *testing.T) {
	f := newFixture()
	v := &fakeVerifier{seen: map[string]bool{}}
	f.srv.ads = v

	// No callback yet: nothing is paid, the client is told to ask again.
	res := f.ad(t, "u1", PlacementWarp, "ad-req-0001", "", true)
	if res.OK || res.ErrorCode != CodeAdPending || res.RetryAfterSeconds != AdPendingRetrySeconds || res.Grant != nil {
		t.Fatalf("before the callback: %+v", res)
	}
	if d := f.repo.grants["u1"]; d != nil && len(d.Grants) != 0 {
		t.Fatalf("grants = %d before the callback", len(d.Grants))
	}
	// A pending ask must not use up the request ID, the fever cooldown or the day's warp allowance.
	if got := f.state(t, "u1").WarpHoursUsed; got != 0 {
		t.Errorf("hours used = %v after a pending ask", got)
	}

	// The callback for another player or another request does not count.
	v.seen["u2|ad-req-0001"] = true
	v.seen["u1|ad-req-9999"] = true
	if res := f.ad(t, "u1", PlacementWarp, "ad-req-0001", "", true); res.OK {
		t.Fatalf("someone else's callback paid: %+v", res)
	}

	// The matching callback arrives: paid once, a retry is answered without paying again.
	v.seen["u1|ad-req-0001"] = true
	a := f.ad(t, "u1", PlacementWarp, "ad-req-0001", "", true)
	b := f.ad(t, "u1", PlacementWarp, "ad-req-0001", "", true)
	if !a.OK || a.Grant == nil || !b.OK || a.Grant.ID != b.Grant.ID {
		t.Fatalf("after the callback: a=%+v b=%+v", a, b)
	}
	if got := f.state(t, "u1").WarpHoursUsed; got != 1 {
		t.Errorf("hours used = %v, want 1", got)
	}
}

func TestAdRewardVerifierErrorPaysNothing(t *testing.T) {
	f := newFixture()
	f.srv.ads = &fakeVerifier{err: errors.New("firestore down")}
	if _, err := f.srv.AdReward(f.ctx, "u1", AdRewardRequest{Placement: PlacementOfflineDouble, RequestID: "ad-req-0001"}); err == nil {
		t.Fatal("a verifier error was swallowed")
	}
}

func TestAdVerificationAppliesToEveryPlacement(t *testing.T) {
	f := newFixture()
	f.srv.ads = &fakeVerifier{seen: map[string]bool{}}
	for _, p := range []Placement{PlacementFever, PlacementOfflineDouble, PlacementWarp} {
		if res := f.ad(t, "u1", p, "ad-req-0002", "store", true); res.OK || res.ErrorCode != CodeAdPending {
			t.Errorf("%s: %+v", p, res)
		}
	}
}
