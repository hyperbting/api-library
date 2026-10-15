package gem

import (
	"testing"

	"api-library/internal/grant"
	"api-library/internal/item"
)

const adminTestItems = `{"version": 1, "items": [
  {"id": "gems_50", "type": "Gem", "amount": 50},
  {"id": "timewarp_1h", "type": "TimeWarp", "amount": 1}
]}`

func newAdminFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture()
	catalog, err := item.ParseCatalog([]byte(adminTestItems))
	if err != nil {
		t.Fatal(err)
	}
	f.srv.catalog = catalog
	return f
}

func (f *fixture) gift(t *testing.T, req GiftRequest) *GiftResponse {
	t.Helper()
	res, err := f.srv.Gift(f.ctx, "admin-1", req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestGiftGemsCreditsTheBalance(t *testing.T) {
	f := newAdminFixture(t)
	res := f.gift(t, GiftRequest{UID: "u1", RequestID: "gift-0001", Gems: 120, Note: "apology for the bug"})
	if !res.OK || res.Grant == nil {
		t.Fatalf("gift: %+v", res)
	}
	if res.Grant.Source != grant.SourceAdmin || res.Grant.ID != "admin:gift-0001" {
		t.Errorf("grant = %+v", res.Grant)
	}
	if res.Grant.SourceRef != "admin-1 apology for the bug" {
		t.Errorf("sourceRef = %q", res.Grant.SourceRef)
	}
	if res.State.Balance != 120 {
		t.Errorf("balance after the gift = %d, want 120", res.State.Balance)
	}
	// The balance is the same on a normal read.
	if got := f.state(t, "u1").Balance; got != 120 {
		t.Errorf("state balance = %d, want 120", got)
	}
}

func TestGiftIsIdempotentByRequestID(t *testing.T) {
	f := newAdminFixture(t)
	first := f.gift(t, GiftRequest{UID: "u1", RequestID: "gift-0001", Gems: 100})
	again := f.gift(t, GiftRequest{UID: "u1", RequestID: "gift-0001", Gems: 100})
	if !first.OK || !again.OK || !again.Repeated {
		t.Fatalf("first=%+v again=%+v", first, again)
	}
	if got := f.state(t, "u1").Balance; got != 100 {
		t.Errorf("balance = %d, want 100 (a repeated gift must not pay twice)", got)
	}
	if n := len(f.repo.grants["u1"].Grants); n != 1 {
		t.Errorf("grants = %d, want 1", n)
	}
}

func TestGiftItemsAndGems(t *testing.T) {
	f := newAdminFixture(t)
	res := f.gift(t, GiftRequest{
		UID: "u1", RequestID: "gift-0002", Gems: 25,
		Items: []item.Ref{{ItemID: "gems_50", Count: 2}, {ItemID: "timewarp_1h", Count: 3}},
	})
	if !res.OK {
		t.Fatalf("gift: %+v", res)
	}
	if res.State.Balance != 125 {
		t.Errorf("balance = %d, want 125 (2 x 50 + 25)", res.State.Balance)
	}
	var warp float64
	for _, r := range res.Grant.Rewards {
		if r.Type == grant.RewardTimeWarp {
			warp += r.Amount
		}
	}
	if warp != 3 {
		t.Errorf("time warp hours = %v, want 3", warp)
	}
}

func TestGiftRefusals(t *testing.T) {
	f := newAdminFixture(t)
	cases := []struct {
		name string
		req  GiftRequest
		want string
	}{
		{"nothing", GiftRequest{UID: "u1", RequestID: "gift-0003"}, CodeEmptyGift},
		{"too many gems", GiftRequest{UID: "u1", RequestID: "gift-0004", Gems: MaxGiftGems + 1}, CodeInvalidGift},
		{"unknown item", GiftRequest{UID: "u1", RequestID: "gift-0005", Items: []item.Ref{{ItemID: "nope", Count: 1}}}, CodeInvalidGift},
	}
	for _, c := range cases {
		res := f.gift(t, c.req)
		if res.OK || res.ErrorCode != c.want {
			t.Errorf("%s: ok=%v code=%q, want %q", c.name, res.OK, res.ErrorCode, c.want)
		}
	}
	if d := f.repo.grants["u1"]; d != nil && len(d.Grants) != 0 {
		t.Errorf("a refused gift left %d grants", len(d.Grants))
	}
}

func TestAdminOverview(t *testing.T) {
	f := newAdminFixture(t)
	f.give("u1", "iap:1", 550)
	f.spend(t, "u1", ProductWarp4h, "req-0001", true)
	f.gift(t, GiftRequest{UID: "u1", RequestID: "gift-0006", Gems: 10})

	ov, err := f.srv.AdminOverview(f.ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !ov.HasRecords {
		t.Error("HasRecords = false")
	}
	if ov.State.Balance != 550-20+10 {
		t.Errorf("balance = %d, want %d", ov.State.Balance, 550-20+10)
	}
	if len(ov.Spends) != 1 || ov.Spends[0].Product != ProductWarp4h || ov.Spends[0].Price != 20 {
		t.Errorf("spends = %+v", ov.Spends)
	}
	if len(ov.Grants) != 3 {
		t.Fatalf("grants = %d, want 3", len(ov.Grants))
	}
	for _, g := range ov.Grants {
		if g.ID == "iap:1" && (g.Gems != 550 || !g.Credited) {
			t.Errorf("iap grant = %+v", g)
		}
	}

	// A player with no records says so (a mistyped uid).
	none, err := f.srv.AdminOverview(f.ctx, "nobody")
	if err != nil {
		t.Fatal(err)
	}
	if none.HasRecords {
		t.Error("an unknown uid reports records")
	}
}

func TestAdminOverviewDoesNotWrite(t *testing.T) {
	f := newAdminFixture(t)
	f.give("u1", "iap:1", 100)
	if _, err := f.srv.AdminOverview(f.ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if d := f.repo.gems["u1"]; d != nil {
		t.Errorf("the overview wrote a gems document: %+v", d)
	}
}
