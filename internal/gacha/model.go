// Package gacha is the server-owned pet and accessory draw (GDD 5.2.2): two rows, each with a timed free draw, a
// gem price and pity counters. The client never rolls; it sends a draw request and plays the reveal of the answer.
//
// What the server keeps: the stored free draws and their timer per row, the pity counters, the accessory
// inventory, the pull-ticket balance and the recent draws by request ID (idempotency). A new pet is a Pet reward
// in the grants ledger (the same ownership the pet package reads); a duplicate pays a TimeWarp grant, minutes of
// the player's current income, which the client turns into coins (the economy runs on the client). Gems come from
// the gem document and are spent in the same transaction as the roll. Ads never touch any of this: no wait is
// skipped by an ad.
package gacha

import (
	"errors"
	"time"

	"api-library/internal/grant"
)

// Firestore layout (server-only; the rules deny all client access to users/{uid}/private/*):
//
//	users/{uid}/private/gacha   Doc, one document per user
const (
	DocID         = "gacha"
	SchemaVersion = 1

	// Remembered draws per user (idempotency). Older ones are dropped.
	maxDrawRecords = 32
)

// Mode is how a draw is paid.
type Mode string

const (
	ModeFree   Mode = "free"
	ModeGem    Mode = "gem"
	ModeTicket Mode = "ticket"
)

// Error codes returned in DrawResponse.ErrorCode (HTTP 200, ok=false).
const (
	CodeNoFreeDraw         = "no_free_draw"
	CodeInsufficientGems   = "insufficient_gems"
	CodeInsufficientTicket = "insufficient_tickets"
	CodeRequestMismatch    = "request_mismatch"
)

var ErrInvalidRequest = errors.New("invalid gacha request")

// Doc is stored at users/{uid}/private/gacha.
type Doc struct {
	SchemaVersion int `firestore:"schemaVersion"`
	// Per row ("character", "accessory"); a row is created on the first look (the rows unlock together).
	Rows map[string]*RowState `firestore:"rows"`
	// Owned accessories by item ID and when they were drawn. An item is owned once: a duplicate becomes cash.
	Accessories map[string]time.Time `firestore:"accessories"`

	// Pull tickets: Pull rewards of grants already counted in Tickets (the ledger credits, this document spends).
	Tickets  int64                `firestore:"tickets"`
	Credited map[string]time.Time `firestore:"credited"`

	Draws     map[string]*DrawRecord `firestore:"draws"`
	UpdatedAt time.Time              `firestore:"updatedAt"`
}

type RowState struct {
	// Stored free draws ready now (0..FreeMax) and when the next one arrives. NextFreeAt is zero while Stored is at
	// the cap: the timer stops there and restarts when a draw is used.
	Stored     int       `firestore:"stored"`
	NextFreeAt time.Time `firestore:"nextFreeAt"`
	// Draws since the last result of each hard-pity tier, by MinTier ("4", "5"). Free, gem and ticket draws share it.
	Pity  map[string]int `firestore:"pity"`
	Total int64          `firestore:"total"`
}

// DrawRecord is a finished draw kept for idempotency: a repeat of the request ID answers from it.
type DrawRecord struct {
	Row      Row       `firestore:"row"`
	Mode     Mode      `firestore:"mode"`
	Count    int       `firestore:"count"`
	At       time.Time `firestore:"at"`
	Results  []Result  `firestore:"results"`
	GrantIDs []string  `firestore:"grantIds"`
	Gems     int64     `firestore:"gems"`
	Tickets  int64     `firestore:"tickets"`
}

// Result is one drawn pet or accessory.
type Result struct {
	Kind     string `firestore:"kind" json:"kind"` // "pet" | "accessory"
	ID       string `firestore:"id" json:"id"`
	Tier     int    `firestore:"tier" json:"tier"` // stars, or 1 = Common .. 4 = Legendary
	TierName string `firestore:"tierName" json:"tierName"`
	// New is false for a duplicate: it paid CashMinutes of the player's current income instead.
	New         bool    `firestore:"new" json:"new"`
	CashMinutes float64 `firestore:"cashMinutes,omitempty" json:"cashMinutes,omitempty"`
	// Pity: the roll was raised by a pity rule.
	Pity bool `firestore:"pity,omitempty" json:"pity,omitempty"`
}

func NewDoc() *Doc {
	d := &Doc{SchemaVersion: SchemaVersion}
	d.Normalize()
	return d
}

// Normalize fills nil maps after a decode.
func (d *Doc) Normalize() {
	if d.Rows == nil {
		d.Rows = map[string]*RowState{}
	}
	if d.Accessories == nil {
		d.Accessories = map[string]time.Time{}
	}
	if d.Credited == nil {
		d.Credited = map[string]time.Time{}
	}
	if d.Draws == nil {
		d.Draws = map[string]*DrawRecord{}
	}
}

// SettleTickets counts the Pull rewards of grants not yet counted. It reports how many tickets were added.
func (d *Doc) SettleTickets(grants *grant.Doc, now time.Time) int64 {
	d.Normalize()
	var added int64
	for id, g := range grants.Grants {
		if _, done := d.Credited[id]; done {
			continue
		}
		for _, r := range g.Rewards {
			if r.Type == grant.RewardPull {
				added += int64(r.Amount)
			}
		}
		d.Credited[id] = now
	}
	d.Tickets += added
	return added
}

func (d *Doc) remember(id string, rec *DrawRecord) {
	d.Draws[id] = rec
	for len(d.Draws) > maxDrawRecords {
		oldestID, oldest := "", time.Time{}
		for k, v := range d.Draws {
			if oldestID == "" || v.At.Before(oldest) {
				oldestID, oldest = k, v.At
			}
		}
		delete(d.Draws, oldestID)
	}
}

func (c *RowConfig) interval() time.Duration {
	return time.Duration(c.FreeIntervalSeconds) * time.Second
}

// openRow returns the row's state, creating it with one draw ready (the timer starts now) on the first look.
func (d *Doc) openRow(cfg *RowConfig, row Row, now time.Time) (rs *RowState, created bool) {
	rs = d.Rows[string(row)]
	if rs == nil {
		rs = &RowState{Stored: 1, NextFreeAt: now.Add(cfg.interval()), Pity: map[string]int{}}
		if rs.Stored >= cfg.FreeMax {
			rs.NextFreeAt = time.Time{}
		}
		d.Rows[string(row)] = rs
		created = true
	}
	if rs.Pity == nil {
		rs.Pity = map[string]int{}
	}
	return rs, created
}

// accrue adds the free draws that arrived by now. The timer keeps its rhythm across closed-app time (each draw
// arrives one interval after the last) and stops at the cap. It reports whether anything changed.
func (rs *RowState) accrue(cfg *RowConfig, now time.Time) bool {
	changed := false
	if rs.Stored >= cfg.FreeMax {
		if !rs.NextFreeAt.IsZero() {
			rs.NextFreeAt = time.Time{}
			changed = true
		}
		return changed
	}
	if rs.NextFreeAt.IsZero() {
		rs.NextFreeAt = now.Add(cfg.interval())
		changed = true
	}
	for rs.Stored < cfg.FreeMax && !now.Before(rs.NextFreeAt) {
		rs.Stored++
		changed = true
		if rs.Stored >= cfg.FreeMax {
			rs.NextFreeAt = time.Time{}
		} else {
			rs.NextFreeAt = rs.NextFreeAt.Add(cfg.interval())
		}
	}
	return changed
}

// useFree takes one stored draw. Used at the cap, the timer starts again from now.
func (rs *RowState) useFree(cfg *RowConfig, now time.Time) {
	if rs.Stored >= cfg.FreeMax {
		rs.NextFreeAt = now.Add(cfg.interval())
	}
	rs.Stored--
}

// API DTOs (mirrored in the Unity client when it gets the shop).

// State is GET /api/{version}/gacha.
type State struct {
	PoolVersion int       `json:"poolVersion"`
	ServerNowMs int64     `json:"serverNowMs"`
	Gems        int64     `json:"gems"`
	Tickets     int64     `json:"tickets"`
	Rows        []RowView `json:"rows"`
	// Accessory inventory: what the client lets a pet wear (at most as many at once as the pet's stars, each in a
	// different slot).
	Accessories []OwnedAccessory `json:"accessories"`
	// Owned pets with the number of accessories each may wear (= stars).
	Pets []OwnedPet `json:"pets"`
}

type RowView struct {
	Row        Row `json:"row"`
	FreeStored int `json:"freeStored"`
	FreeMax    int `json:"freeMax"`
	// NextFreeAt is null while the stored draws are at the cap; NextFreeInSeconds counts down on the server clock.
	NextFreeAt        *time.Time `json:"nextFreeAt"`
	NextFreeInSeconds int64      `json:"nextFreeInSeconds"`
	FreeIntervalSecs  int64      `json:"freeIntervalSeconds"`
	GemPrice1         int64      `json:"gemPrice1"`
	GemPrice10        int64      `json:"gemPrice10"`
	TicketCost1       int64      `json:"ticketCost1"`
	Pity              []PityView `json:"pity"`
	TenMinTier        int        `json:"tenMinTier"`
	// Odds for the Info button (store rules).
	Tiers []TierView `json:"tiers"`
}

type PityView struct {
	MinTier int `json:"minTier"`
	Every   int `json:"every"`
	// Draws since the last result of MinTier or better, and draws left until one is guaranteed.
	Since int `json:"since"`
	Left  int `json:"left"`
}

type TierView struct {
	Tier       int     `json:"tier"`
	Name       string  `json:"name"`
	RatePct    float64 `json:"ratePct"`
	DupMinutes float64 `json:"dupMinutes"`
}

type OwnedAccessory struct {
	ID     string   `json:"id"`
	Slot   string   `json:"slot"`
	Rarity string   `json:"rarity"`
	Fits   []string `json:"fits"`
}

type OwnedPet struct {
	ID             string `json:"id"` // Addressable address
	Stars          int    `json:"stars"`
	MaxAccessories int    `json:"maxAccessories"`
}

// POST /api/{version}/gacha/draw
type DrawRequest struct {
	Row  Row  `json:"row" validate:"required,oneof=character accessory"`
	Mode Mode `json:"mode" validate:"required,oneof=free gem ticket"`
	// 1 or 10. A free draw and a ticket draw are always 1.
	Count int `json:"count" validate:"required,oneof=1 10"`
	// Chosen by the client for each attempt (a UUID); a retry with the same value returns the same result.
	RequestID string `json:"requestId" validate:"required,min=8,max=64"`
}

type DrawResponse struct {
	OK        bool   `json:"ok"`
	ErrorCode string `json:"errorCode,omitempty"`
	// Repeated: answered from the record of an earlier call with this requestId (nothing was charged again).
	Repeated bool     `json:"repeated,omitempty"`
	Results  []Result `json:"results,omitempty"`
	// CashHours: the duplicates' payout as hours of current income (a TimeWarp grant); 0 when none.
	CashHours float64 `json:"cashHours,omitempty"`
	// Grants created by the draw (new pets, duplicate cash). The client applies them like any grant and reports
	// them claimed.
	Grants []grant.Grant `json:"grants,omitempty"`
	State  State         `json:"state"`
}
