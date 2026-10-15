package item

import (
	"errors"
	"testing"

	"api-library/internal/grant"
)

func TestEmbeddedCatalogIsValid(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"pull_ticket", "timewarp_1h", "timewarp_24h"} {
		if _, ok := c.Get(id); !ok {
			t.Errorf("missing item %q", id)
		}
	}
}

const testCatalog = `{"version": 3, "items": [
  {"id": "pull_ticket", "type": "Pull", "amount": 1},
  {"id": "timewarp_1h", "type": "TimeWarp", "amount": 1},
  {"id": "unlock_habitat", "type": "Unlock", "key": "home.habitat", "amount": 0}
]}`

func mustParse(t *testing.T, s string) *Catalog {
	t.Helper()
	c, err := ParseCatalog([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestParseCatalogRejects(t *testing.T) {
	bad := map[string]string{
		"not json":      `{`,
		"no version":    `{"items":[{"id":"a","type":"Pull","amount":1}]}`,
		"no items":      `{"version":1,"items":[]}`,
		"bad id":        `{"version":1,"items":[{"id":"Has Space","type":"Pull","amount":1}]}`,
		"duplicate id":  `{"version":1,"items":[{"id":"a","type":"Pull","amount":1},{"id":"a","type":"Pull","amount":2}]}`,
		"unknown type":  `{"version":1,"items":[{"id":"a","type":"Gems","amount":1}]}`,
		"zero amount":   `{"version":1,"items":[{"id":"a","type":"Cash","amount":0}]}`,
		"unlock no key": `{"version":1,"items":[{"id":"a","type":"Unlock"}]}`,
	}
	for name, s := range bad {
		if _, err := ParseCatalog([]byte(s)); !errors.Is(err, ErrInvalidCatalog) {
			t.Errorf("%s: want ErrInvalidCatalog, got %v", name, err)
		}
	}
}

func TestResolveMultipliesUnitAmountByCount(t *testing.T) {
	c := mustParse(t, testCatalog)

	got, err := c.Resolve([]Ref{
		{ItemID: "pull_ticket", Count: 3},
		{ItemID: "timewarp_1h", Count: 2},
		{ItemID: "unlock_habitat", Count: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []grant.Reward{
		{Type: grant.RewardPull, Amount: 3, ItemID: "pull_ticket", Count: 3},
		{Type: grant.RewardTimeWarp, Amount: 2, ItemID: "timewarp_1h", Count: 2},
		{Type: grant.RewardUnlock, Key: "home.habitat", Amount: 0, ItemID: "unlock_habitat", Count: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rewards, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("reward %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestResolveRefusals(t *testing.T) {
	c := mustParse(t, testCatalog)

	if _, err := c.Resolve(nil); !errors.Is(err, ErrInvalidRef) {
		t.Errorf("empty: %v", err)
	}
	if _, err := c.Resolve([]Ref{{ItemID: "nope", Count: 1}}); !errors.Is(err, ErrUnknownItem) {
		t.Errorf("unknown: %v", err)
	}
	for _, n := range []int{0, -1, MaxCount + 1} {
		if _, err := c.Resolve([]Ref{{ItemID: "pull_ticket", Count: n}}); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("count %d: %v", n, err)
		}
	}
	// 200 hours is above the time warp cap (a week).
	if _, err := c.Resolve([]Ref{{ItemID: "timewarp_1h", Count: 200}}); !errors.Is(err, ErrInvalidRef) {
		t.Errorf("time warp cap: %v", err)
	}
}

func TestResolveDoesNotAliasCatalog(t *testing.T) {
	c := mustParse(t, testCatalog)
	got, _ := c.Resolve([]Ref{{ItemID: "pull_ticket", Count: 1}})
	got[0].Amount = 999
	d, _ := c.Get("pull_ticket")
	if d.Amount != 1 {
		t.Error("changing a resolved reward changed the catalog")
	}
}

func TestProducts(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"gems_100": 100, "gems_550": 550, "gems_1150": 1150, "gems_2400": 2400}
	for sku, gems := range want {
		p, ok := c.Product(sku)
		if !ok {
			t.Errorf("missing product %q", sku)
			continue
		}
		rewards, err := c.Resolve(p.Items)
		if err != nil {
			t.Errorf("%s: %v", sku, err)
			continue
		}
		var total float64
		for _, r := range rewards {
			if r.Type != grant.RewardGem {
				t.Errorf("%s: reward %+v is not a Gem", sku, r)
			}
			total += r.Amount
		}
		if total != gems {
			t.Errorf("%s gives %v gems, want %v", sku, total, gems)
		}
		if !p.FirstPurchaseDouble {
			t.Errorf("%s: first purchase bonus should be on", sku)
		}
	}
}

func TestParseCatalogRejectsBadProducts(t *testing.T) {
	bad := map[string]string{
		"unknown item": `{"version":1,"items":[{"id":"a","type":"Pull","amount":1}],"products":[{"sku":"p","items":[{"itemId":"zzz","count":1}]}]}`,
		"duplicate":    `{"version":1,"items":[{"id":"a","type":"Pull","amount":1}],"products":[{"sku":"p","items":[{"itemId":"a","count":1}]},{"sku":"p","items":[{"itemId":"a","count":1}]}]}`,
		"bad sku":      `{"version":1,"items":[{"id":"a","type":"Pull","amount":1}],"products":[{"sku":"Bad SKU","items":[{"itemId":"a","count":1}]}]}`,
	}
	for name, doc := range bad {
		if _, err := ParseCatalog([]byte(doc)); !errors.Is(err, ErrInvalidCatalog) {
			t.Errorf("%s: want ErrInvalidCatalog, got %v", name, err)
		}
	}
}
