package iap

import (
	"context"
	"errors"
	"testing"
	"time"

	"api-library/internal/grant"
	"api-library/internal/item"
)

type memRepo struct {
	orders map[string]*Order
	grants map[string]*grant.Doc
}

func newMemRepo() *memRepo {
	return &memRepo{orders: map[string]*Order{}, grants: map[string]*grant.Doc{}}
}

func (r *memRepo) Orders(_ context.Context, _ int) ([]Order, error) {
	out := []Order{}
	for _, o := range r.orders {
		out = append(out, *o)
	}
	return out, nil
}

func (r *memRepo) Deliver(_ context.Context, uid, orderID string, fn func(*grant.Doc) (bool, *Order, error)) error {
	if o := r.orders[orderID]; o != nil && o.UID != uid {
		return ErrOrderUsed
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
	write, order, err := fn(grants)
	if err != nil {
		return err
	}
	if write {
		r.grants[uid] = grants
		if order != nil && r.orders[orderID] == nil {
			r.orders[orderID] = order
		}
	}
	return nil
}

// fakeStore answers by token: "good-N" is a paid purchase with order "ORDER-N".
type fakeStore struct {
	receipts map[string]*Receipt
	err      error
	consumed []string
}

func (f *fakeStore) Verify(_ context.Context, _, token string) (*Receipt, error) {
	if f.err != nil {
		return nil, f.err
	}
	if r := f.receipts[token]; r != nil {
		c := *r
		return &c, nil
	}
	return &Receipt{}, nil
}

func (f *fakeStore) Consume(_ context.Context, _, token string) error {
	f.consumed = append(f.consumed, token)
	return nil
}

const testCatalog = `{"version": 1,
 "items": [{"id": "gems_50", "type": "Gem", "amount": 50}],
 "products": [
   {"sku": "gems_100", "items": [{"itemId": "gems_50", "count": 2}], "firstPurchaseDouble": true},
   {"sku": "gems_550", "items": [{"itemId": "gems_50", "count": 11}]}
 ]}`

type fixture struct {
	srv   *service
	repo  *memRepo
	store *fakeStore
	ctx   context.Context
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	catalog, err := item.ParseCatalog([]byte(testCatalog))
	if err != nil {
		t.Fatal(err)
	}
	repo := newMemRepo()
	store := &fakeStore{receipts: map[string]*Receipt{
		"good-1":    {OrderID: "ORDER-1", Purchased: true},
		"good-2":    {OrderID: "ORDER-2", Purchased: true},
		"good-550":  {OrderID: "ORDER-550", Purchased: true},
		"pending-1": {OrderID: "ORDER-P", Pending: true},
		"cancel-1":  {OrderID: "ORDER-C", Canceled: true},
	}}
	srv := NewService(repo, catalog, store).(*service)
	srv.now = func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }
	return &fixture{srv: srv, repo: repo, store: store, ctx: context.Background()}
}

func (f *fixture) buy(t *testing.T, uid, sku, token string) *PurchaseResponse {
	t.Helper()
	res, err := f.srv.Purchase(f.ctx, uid, PurchaseRequest{SKU: sku, PurchaseToken: token})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func gems(g *grant.Grant) float64 {
	var n float64
	for _, r := range g.Rewards {
		if r.Type == grant.RewardGem {
			n += r.Amount
		}
	}
	return n
}

func TestFirstPurchaseIsDoubledOncePerPack(t *testing.T) {
	f := newFixture(t)

	first := f.buy(t, "u1", "gems_100", "good-1")
	if !first.OK || !first.FirstPurchaseBonus || gems(first.Grant) != 200 {
		t.Fatalf("first purchase: %+v gems=%v", first, gems(first.Grant))
	}
	second := f.buy(t, "u1", "gems_100", "good-2")
	if !second.OK || second.FirstPurchaseBonus || gems(second.Grant) != 100 {
		t.Fatalf("second purchase of the same pack: %+v gems=%v", second, gems(second.Grant))
	}
	// A pack that is not marked is never doubled.
	big := f.buy(t, "u1", "gems_550", "good-550")
	if !big.OK || big.FirstPurchaseBonus || gems(big.Grant) != 550 {
		t.Fatalf("gems_550: %+v gems=%v", big, gems(big.Grant))
	}
}

func TestFirstPurchaseBonusIsPerAccount(t *testing.T) {
	f := newFixture(t)
	f.buy(t, "u1", "gems_100", "good-1")
	other := f.buy(t, "u2", "gems_100", "good-2")
	if !other.OK || !other.FirstPurchaseBonus {
		t.Errorf("u2's first purchase should be doubled: %+v", other)
	}
}

func TestSameTokenDeliversOnce(t *testing.T) {
	f := newFixture(t)
	a := f.buy(t, "u1", "gems_100", "good-1")
	b := f.buy(t, "u1", "gems_100", "good-1")
	if !a.OK || !b.OK || a.Grant.ID != b.Grant.ID {
		t.Fatalf("a=%+v b=%+v", a, b)
	}
	if len(f.repo.grants["u1"].Grants) != 1 {
		t.Errorf("grants = %d, want 1", len(f.repo.grants["u1"].Grants))
	}
	if a.Grant.ID != "iap:ORDER-1" || a.Grant.Source != grant.SourceIAP || a.Grant.SourceRef != "gems_100" {
		t.Errorf("grant = %+v", a.Grant)
	}
	// The retry gives the stored grant back, bonus included.
	if gems(b.Grant) != 200 {
		t.Errorf("retry gems = %v, want 200", gems(b.Grant))
	}
}

func TestOrderOfAnotherAccountIsRefused(t *testing.T) {
	f := newFixture(t)
	f.buy(t, "u1", "gems_100", "good-1")
	res := f.buy(t, "u2", "gems_100", "good-1")
	if res.OK || res.ErrorCode != CodeOrderUsed {
		t.Errorf("replay on another account: %+v", res)
	}
}

func TestRefusals(t *testing.T) {
	f := newFixture(t)
	cases := []struct{ name, sku, token, want string }{
		{"unknown sku", "gems_999", "good-1", CodeUnknownProduct},
		{"unknown token", "gems_100", "forged", CodeInvalidReceipt},
		{"pending", "gems_100", "pending-1", CodePending},
		{"canceled", "gems_100", "cancel-1", CodeCanceled},
	}
	for _, c := range cases {
		res := f.buy(t, "u1", c.sku, c.token)
		if res.OK || res.ErrorCode != c.want {
			t.Errorf("%s: ok=%v code=%q, want %q", c.name, res.OK, res.ErrorCode, c.want)
		}
	}
	if d := f.repo.grants["u1"]; d != nil && len(d.Grants) != 0 {
		t.Errorf("a refused purchase left %d grants", len(d.Grants))
	}
}

func TestStoreUnavailableIsAnError(t *testing.T) {
	f := newFixture(t)
	f.store.err = ErrStoreUnavailable
	_, err := f.srv.Purchase(f.ctx, "u1", PurchaseRequest{SKU: "gems_100", PurchaseToken: "good-1"})
	if !errors.Is(err, ErrStoreUnavailable) {
		t.Errorf("err = %v, want ErrStoreUnavailable", err)
	}
}

func TestConsumedAfterDelivery(t *testing.T) {
	f := newFixture(t)
	f.buy(t, "u1", "gems_100", "good-1")
	if len(f.store.consumed) != 1 || f.store.consumed[0] != "good-1" {
		t.Errorf("consumed = %v", f.store.consumed)
	}
	// A refused purchase is not consumed.
	f.buy(t, "u1", "gems_100", "forged")
	if len(f.store.consumed) != 1 {
		t.Errorf("consumed after a refusal = %v", f.store.consumed)
	}
}
