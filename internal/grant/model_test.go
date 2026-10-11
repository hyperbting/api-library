package grant

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

func TestRewardValidate(t *testing.T) {
	valid := []Reward{
		{Type: RewardCash, Amount: 1000},
		{Type: RewardPull, Amount: 3},
		{Type: RewardUnlock, Key: "home.habitat"},
		{Type: RewardTimeWarp, Amount: 2},
	}
	for _, r := range valid {
		if err := r.Validate(); err != nil {
			t.Errorf("%+v: %v", r, err)
		}
	}

	invalid := []Reward{
		{Type: RewardCash, Amount: 0},
		{Type: RewardCash, Amount: -5},
		{Type: RewardPull},
		{Type: RewardUnlock},
		{Type: RewardTimeWarp, Amount: 200},
		{Type: "Gems", Amount: 1},
		{Type: RewardCash, Amount: 1, Key: string(make([]byte, 100))},
	}
	for _, r := range invalid {
		if err := r.Validate(); !errors.Is(err, ErrInvalidReward) {
			t.Errorf("%+v: want ErrInvalidReward, got %v", r, err)
		}
	}
}

func TestValidateRewardsBounds(t *testing.T) {
	if err := ValidateRewards(nil); err == nil {
		t.Error("empty list accepted")
	}
	many := make([]Reward, MaxRewardsPerGrant+1)
	for i := range many {
		many[i] = Reward{Type: RewardPull, Amount: 1}
	}
	if err := ValidateRewards(many); err == nil {
		t.Error("too many rewards accepted")
	}
}

func TestAddIsIdempotentAndClaimed(t *testing.T) {
	d := NewDoc()
	g := Grant{ID: "redeem:ABCD", Source: SourceRedeem, Rewards: []Reward{{Type: RewardPull, Amount: 1}}, CreatedAt: t0}

	added, err := d.Add(g)
	if err != nil || !added {
		t.Fatalf("first add: %v %v", added, err)
	}
	if added, _ = d.Add(g); added {
		t.Error("second add with the same ID created a grant")
	}
	if got := d.Unclaimed(); len(got) != 1 || got[0].ID != g.ID {
		t.Fatalf("unclaimed = %+v", got)
	}

	if n := d.Claim([]string{g.ID, "missing"}, t0); n != 1 {
		t.Errorf("claimed %d, want 1", n)
	}
	if n := d.Claim([]string{g.ID}, t0); n != 0 {
		t.Errorf("claiming twice changed %d", n)
	}
	if got := d.Unclaimed(); len(got) != 0 {
		t.Errorf("claimed grant still listed: %+v", got)
	}
	// A claimed grant keeps its ID, so the same redeem cannot create it again.
	if added, _ = d.Add(g); added {
		t.Error("claimed grant was created again")
	}
}

func TestUnclaimedOrder(t *testing.T) {
	d := NewDoc()
	d.Add(Grant{ID: "b", CreatedAt: t0.Add(time.Minute)})
	d.Add(Grant{ID: "c", CreatedAt: t0})
	d.Add(Grant{ID: "a", CreatedAt: t0})
	var ids []string
	for _, g := range d.Unclaimed() {
		ids = append(ids, g.ID)
	}
	if want := "a,c,b"; join(ids) != want {
		t.Errorf("order = %s, want %s", join(ids), want)
	}
}

func TestAddLimit(t *testing.T) {
	d := NewDoc()
	for i := 0; i < MaxGrants; i++ {
		if _, err := d.Add(Grant{ID: string(rune('a'+i%26)) + string(rune(i))}); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}
	if _, err := d.Add(Grant{ID: "one-too-many"}); !errors.Is(err, ErrTooManyGrants) {
		t.Errorf("want ErrTooManyGrants, got %v", err)
	}
}

func TestRedeemThrottle(t *testing.T) {
	d := NewDoc()
	for i := 0; i < RedeemMaxFailures; i++ {
		if d.RedeemLocked(t0) {
			t.Fatalf("locked after %d failures", i)
		}
		d.NoteRedeemFailure(t0)
	}
	if !d.RedeemLocked(t0.Add(time.Minute)) {
		t.Error("not locked after the failure budget")
	}
	// The window passes: free again, and a new failure starts a new window.
	later := t0.Add(RedeemWindow)
	if d.RedeemLocked(later) {
		t.Error("still locked after the window")
	}
	d.NoteRedeemFailure(later)
	if d.RedeemFailures != 1 || !d.RedeemWindowStart.Equal(later) {
		t.Errorf("window not restarted: %d failures since %v", d.RedeemFailures, d.RedeemWindowStart)
	}
}

func join(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += ","
		}
		out += v
	}
	return out
}
