// Package item is the catalog of things the server can hand out: a reward kind and the size of one unit.
// Redeem codes (and IAP products, later) reference items as {itemId, count}; when a grant is created the
// reference is resolved to the concrete {type, key, amount} reward the client applies.
//
// The catalog is static data embedded at build time, like the quest table: the client needs code for every
// reward type anyway, so a new kind of item needs an app release, and IAP validation must not depend on
// rows someone can edit at runtime. The per-event data (which codes exist) stays in Firestore.
package item

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"api-library/internal/grant"
)

// data/items.json is edited by hand in this repo (no Unity export): grants carry the resolved reward, so
// the client does not need a copy.
//
//go:embed data/items.json
var catalogJSON []byte

const (
	// Upper bound for the count of one reference (a code line, an IAP product line).
	MaxCount = 1000
)

var (
	ErrInvalidCatalog = errors.New("invalid item catalog")
	ErrUnknownItem    = errors.New("unknown item")
	ErrInvalidRef     = errors.New("invalid item reference")

	idPattern  = regexp.MustCompile(`^[a-z0-9_]{1,48}$`)
	skuPattern = regexp.MustCompile(`^[a-z0-9_.]{1,64}$`)
)

// Definition is one item: what it is and the size of one unit.
//
//	type    reward kind the client knows (Cash, Pull, Unlock, TimeWarp, Gem)
//	amount  size of ONE unit: 1 pull, 1 hour of income, a cash value
//	key     only for Unlock
//
// How many units a code or product gives is the reference's Count, not part of the item.
type Definition struct {
	ID     string           `json:"id"`
	Type   grant.RewardType `json:"type"`
	Key    string           `json:"key,omitempty"`
	Amount float64          `json:"amount"`
	Note   string           `json:"note,omitempty"`
}

// Ref points at an item with a count. Stored on a redeem code (and later on an IAP product).
type Ref struct {
	ItemID string `firestore:"itemId" json:"itemId"`
	Count  int    `firestore:"count" json:"count"`
}

// Product is something sold for real money: the store SKU (the Play Console product id) and what one purchase
// delivers, as item references. Gem packs today. Validated at load like the items, so a SKU that resolves to
// nothing fails the boot, not a player's purchase.
type Product struct {
	SKU   string `json:"sku"`
	Items []Ref  `json:"items"`
	// FirstPurchaseDouble doubles the Gem rewards the first time an account buys this SKU (GDD 5.2.1, once per
	// pack per account).
	FirstPurchaseDouble bool   `json:"firstPurchaseDouble,omitempty"`
	Note                string `json:"note,omitempty"`
}

type Catalog struct {
	Version  int          `json:"version"`
	Items    []Definition `json:"items"`
	Products []Product    `json:"products,omitempty"`

	byID  map[string]*Definition
	bySKU map[string]*Product
}

// LoadCatalog parses and validates the embedded catalog.
func LoadCatalog() (*Catalog, error) {
	return ParseCatalog(catalogJSON)
}

func ParseCatalog(data []byte) (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCatalog, err)
	}
	if c.Version < 1 {
		return nil, fmt.Errorf("%w: version must be >= 1", ErrInvalidCatalog)
	}
	if len(c.Items) == 0 {
		return nil, fmt.Errorf("%w: no items", ErrInvalidCatalog)
	}

	c.byID = make(map[string]*Definition, len(c.Items))
	for i := range c.Items {
		d := &c.Items[i]
		if !idPattern.MatchString(d.ID) {
			return nil, fmt.Errorf("%w: item id %q (use a-z, 0-9, _; up to 48)", ErrInvalidCatalog, d.ID)
		}
		if _, dup := c.byID[d.ID]; dup {
			return nil, fmt.Errorf("%w: duplicate item %q", ErrInvalidCatalog, d.ID)
		}
		if err := (grant.Reward{Type: d.Type, Key: d.Key, Amount: d.Amount}).Validate(); err != nil {
			return nil, fmt.Errorf("%w: item %q: %v", ErrInvalidCatalog, d.ID, err)
		}
		c.byID[d.ID] = d
	}

	c.bySKU = make(map[string]*Product, len(c.Products))
	for i := range c.Products {
		p := &c.Products[i]
		if !skuPattern.MatchString(p.SKU) {
			return nil, fmt.Errorf("%w: product sku %q (use a-z, 0-9, _ and .; up to 64)", ErrInvalidCatalog, p.SKU)
		}
		if _, dup := c.bySKU[p.SKU]; dup {
			return nil, fmt.Errorf("%w: duplicate product %q", ErrInvalidCatalog, p.SKU)
		}
		if _, err := c.Resolve(p.Items); err != nil {
			return nil, fmt.Errorf("%w: product %q: %v", ErrInvalidCatalog, p.SKU, err)
		}
		c.bySKU[p.SKU] = p
	}
	return &c, nil
}

func (c *Catalog) Get(id string) (*Definition, bool) {
	d, ok := c.byID[id]
	return d, ok
}

// Product returns the product sold under a store SKU.
func (c *Catalog) Product(sku string) (*Product, bool) {
	p, ok := c.bySKU[sku]
	return p, ok
}

// Resolve turns references into the rewards a grant stores. Each reward is the item's unit amount times the
// count, and keeps the item id and count for display. The result is validated as a whole, so an amount that
// no longer makes sense (e.g. a time warp above the cap) is refused here, not at the client.
func (c *Catalog) Resolve(refs []Ref) ([]grant.Reward, error) {
	if len(refs) == 0 || len(refs) > grant.MaxRewardsPerGrant {
		return nil, fmt.Errorf("%w: need 1..%d items", ErrInvalidRef, grant.MaxRewardsPerGrant)
	}

	out := make([]grant.Reward, 0, len(refs))
	for _, ref := range refs {
		d, ok := c.byID[ref.ItemID]
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrUnknownItem, ref.ItemID)
		}
		if ref.Count < 1 || ref.Count > MaxCount {
			return nil, fmt.Errorf("%w: count %d for %q", ErrInvalidRef, ref.Count, ref.ItemID)
		}
		out = append(out, grant.Reward{
			Type:   d.Type,
			Key:    d.Key,
			Amount: d.Amount * float64(ref.Count),
			ItemID: d.ID,
			Count:  ref.Count,
		})
	}
	if err := grant.ValidateRewards(out); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRef, err)
	}
	return out, nil
}
