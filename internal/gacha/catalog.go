package gacha

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"api-library/internal/grant"
)

//go:embed data/pool.json
var catalogJSON []byte

// Row is one gacha row (GDD 5.2.2).
type Row string

const (
	RowCharacter Row = "character"
	RowAccessory Row = "accessory"
)

// Rows lists the rows in display order.
var Rows = []Row{RowCharacter, RowAccessory}

// RateScale: tier rates are whole numbers out of this total (basis points, so 6.5% is 650).
const RateScale = 10000

// MinutesPerGem is the reference exchange rate used to check that a duplicate never pays more than the draw
// costs: a 4 h warp is 20 gems (GDD 5.2.1), i.e. 12 income-minutes per gem.
const MinutesPerGem = 12.0

// Catalog is the compiled-in pool: pets, accessories, rates, prices and pity. A change is a version bump and a
// new server image, not a client release.
type Catalog struct {
	Version     int                `json:"version"`
	Rows        map[Row]*RowConfig `json:"rows"`
	Pets        []PetDef           `json:"pets"`
	Accessories []AccessoryDef     `json:"accessories"`

	petsByTier map[int][]*PetDef
	accsByTier map[int][]*AccessoryDef
	pets       map[string]*PetDef
	accs       map[string]*AccessoryDef
}

type RowConfig struct {
	FreeIntervalSeconds int64 `json:"freeIntervalSeconds"`
	FreeMax             int   `json:"freeMax"`
	GemPrice1           int64 `json:"gemPrice1"`
	GemPrice10          int64 `json:"gemPrice10"`
	// TicketCost1: pull tickets that pay one x1 draw; 0 = the row takes no tickets.
	TicketCost1 int64  `json:"ticketCost1"`
	Tiers       []Tier `json:"tiers"`
	Pity        Pity   `json:"pity"`
}

// Tier is a star level (character) or a rarity (accessory), 1 = lowest.
type Tier struct {
	Tier int    `json:"tier"`
	Name string `json:"name"`
	Rate int    `json:"rate"` // out of RateScale
	// DupMinutes: minutes of the player's current income paid when the draw is a duplicate.
	DupMinutes float64 `json:"dupMinutes"`
}

type Pity struct {
	// TenMinTier: a x10 draw holds at least one result of this tier or better.
	TenMinTier int        `json:"tenMinTier"`
	Hard       []HardPity `json:"hard"`
}

// HardPity: a result of MinTier or better at least every Every draws (the Every-th draw is forced).
type HardPity struct {
	MinTier int `json:"minTier"`
	Every   int `json:"every"`
}

type PetDef struct {
	ID       string `json:"id"`
	Address  string `json:"address"` // Addressable address in the Unity client
	Species  string `json:"species"`
	Stars    int    `json:"stars"`
	Category string `json:"category"`
	Role     string `json:"role"`
}

type AccessoryDef struct {
	ID     string `json:"id"`
	Slot   string `json:"slot"`
	Rarity string `json:"rarity"`
	// Fits is ["all"] or a species list.
	Fits []string `json:"fits"`

	tier int
}

var validSlots = map[string]bool{"Hat": true, "Face": true, "Neck": true, "Torso": true, "Waist": true, "Back": true, "Butt": true, "Tail": true, "Held": true}

// LoadCatalog parses and validates the embedded pool; the server refuses to boot on an invalid one.
func LoadCatalog() (*Catalog, error) { return ParseCatalog(catalogJSON) }

func ParseCatalog(data []byte) (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("gacha pool: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("gacha pool: %w", err)
	}
	return &c, nil
}

func (r *RowConfig) tier(n int) *Tier {
	if n < 1 || n > len(r.Tiers) {
		return nil
	}
	return &r.Tiers[n-1]
}

func (r *RowConfig) tierByName(name string) int {
	for _, t := range r.Tiers {
		if t.Name == name {
			return t.Tier
		}
	}
	return 0
}

func (c *Catalog) validate() error {
	if c.Version < 1 {
		return fmt.Errorf("version must be >= 1")
	}
	for _, row := range Rows {
		cfg := c.Rows[row]
		if cfg == nil {
			return fmt.Errorf("row %s is missing", row)
		}
		if cfg.FreeIntervalSeconds < 60 || cfg.FreeMax < 1 {
			return fmt.Errorf("row %s: free draw interval / cap", row)
		}
		if cfg.GemPrice1 < 1 || cfg.GemPrice10 < cfg.GemPrice1 || cfg.GemPrice10 > 10*cfg.GemPrice1 || cfg.TicketCost1 < 0 {
			return fmt.Errorf("row %s: prices", row)
		}
		sum := 0
		for i, t := range cfg.Tiers {
			if t.Tier != i+1 || t.Name == "" || t.Rate < 1 {
				return fmt.Errorf("row %s: tier %d must be numbered from 1 in order, named, with a rate", row, i+1)
			}
			// A duplicate pays coins that never beat the gem price of one draw.
			if t.DupMinutes <= 0 || t.DupMinutes >= float64(cfg.GemPrice1)*MinutesPerGem {
				return fmt.Errorf("row %s: tier %s duplicate cash must be in (0, price)", row, t.Name)
			}
			if i > 0 && t.DupMinutes < cfg.Tiers[i-1].DupMinutes {
				return fmt.Errorf("row %s: duplicate cash must not fall with the tier", row)
			}
			sum += t.Rate
		}
		if len(cfg.Tiers) < 2 || sum != RateScale {
			return fmt.Errorf("row %s: rates add up to %d, want %d", row, sum, RateScale)
		}
		if cfg.Pity.TenMinTier < 1 || cfg.Pity.TenMinTier > len(cfg.Tiers) {
			return fmt.Errorf("row %s: pity tenMinTier", row)
		}
		for _, h := range cfg.Pity.Hard {
			if h.MinTier < 2 || h.MinTier > len(cfg.Tiers) || h.Every < 2 {
				return fmt.Errorf("row %s: hard pity %+v", row, h)
			}
		}
	}

	chars, accs := c.Rows[RowCharacter], c.Rows[RowAccessory]
	c.pets = map[string]*PetDef{}
	c.petsByTier = map[int][]*PetDef{}
	for i := range c.Pets {
		p := &c.Pets[i]
		if p.ID == "" || c.pets[p.ID] != nil {
			return fmt.Errorf("pet %q: empty or duplicate id", p.ID)
		}
		if !grant.KnownPets[p.Address] {
			return fmt.Errorf("pet %s: address %q is not a known pet (grant.KnownPets)", p.ID, p.Address)
		}
		if chars.tier(p.Stars) == nil || p.Species == "" || p.Category == "" || p.Role == "" {
			return fmt.Errorf("pet %s: stars / species / category / role", p.ID)
		}
		for _, s := range grant.StarterPets {
			if s == p.Address {
				return fmt.Errorf("pet %s is a starter pet: it would always be a duplicate", p.ID)
			}
		}
		c.pets[p.ID] = p
		c.petsByTier[p.Stars] = append(c.petsByTier[p.Stars], p)
	}
	c.accs = map[string]*AccessoryDef{}
	c.accsByTier = map[int][]*AccessoryDef{}
	for i := range c.Accessories {
		a := &c.Accessories[i]
		if a.ID == "" || c.accs[a.ID] != nil {
			return fmt.Errorf("accessory %q: empty or duplicate id", a.ID)
		}
		if !validSlots[a.Slot] {
			return fmt.Errorf("accessory %s: unknown slot %q", a.ID, a.Slot)
		}
		if a.tier = accs.tierByName(a.Rarity); a.tier == 0 {
			return fmt.Errorf("accessory %s: unknown rarity %q", a.ID, a.Rarity)
		}
		if len(a.Fits) == 0 {
			return fmt.Errorf("accessory %s: fits is empty (use [\"all\"])", a.ID)
		}
		for _, f := range a.Fits {
			if f == "all" && len(a.Fits) > 1 {
				return fmt.Errorf("accessory %s: \"all\" cannot be mixed with species", a.ID)
			}
		}
		c.accs[a.ID] = a
		c.accsByTier[a.tier] = append(c.accsByTier[a.tier], a)
	}
	for _, t := range chars.Tiers {
		if len(c.petsByTier[t.Tier]) == 0 {
			return fmt.Errorf("no pet of tier %d (%s)", t.Tier, t.Name)
		}
	}
	for _, t := range accs.Tiers {
		if len(c.accsByTier[t.Tier]) == 0 {
			return fmt.Errorf("no accessory of tier %d (%s)", t.Tier, t.Name)
		}
	}
	return nil
}
