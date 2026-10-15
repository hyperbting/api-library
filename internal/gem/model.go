// Package gem is the server-owned premium currency: the balance, what it buys (time warps and the x2 income
// boost, GDD 5.2.1) and the limits on it. Gems enter as Gem rewards in the grant ledger (package grant: IAP
// receipts, redeem codes, support gifts, tutorial gifts); the balance is credited from those grants when the
// service next reads the user's state, so the credit and the spend are always computed against the same
// documents inside one transaction.
//
// The game economy itself runs on the client; the server only keeps what must not be forgeable by editing a
// local file: how many gems a player has, how many hours of warp they bought today and how much boost they
// still have.
package gem

import (
	"errors"
	"time"

	"api-library/internal/grant"
)

// Firestore layout (server-only; the rules deny all client access to users/{uid}/private/*):
//
//	users/{uid}/private/gems   Doc, one document per user
const (
	GemsDocID     = "gems"
	SchemaVersion = 1

	// MaxWarpHoursPerDay is the payout hours a player may buy per day (GDD 5.2.1). With the x2 boost running a
	// warp counts double against it.
	MaxWarpHoursPerDay = 8.0
	// ResetHourUTC is when the daily allowance starts again: midnight UTC (decided 2026-10-15). The clients show
	// the countdown from State.WarpResetsInSeconds.
	ResetHourUTC = 0

	// MaxBoostSeconds bounds the stored boost, so a stack of purchases cannot grow without limit.
	MaxBoostSeconds = 2 * 3600
	// MaxConsumeSeconds bounds one report of in-store play time.
	MaxConsumeSeconds = 3600

	// Remembered request IDs per user (spend idempotency). Older ones are dropped.
	maxSpendRecords = 64
)

// Product is what gems buy.
type Product string

const (
	ProductWarp4h   Product = "warp_4h"
	ProductWarp8h   Product = "warp_8h"
	ProductBoost10m Product = "boost_10m"
	ProductBoost30m Product = "boost_30m"
	// ProductFever starts a store's golden fever for gems (the sale itself is client side, like the ad start; the
	// ad start has the cooldown, this one has none). Decided 2026-10-15: 5 gems.
	ProductFever Product = "golden_fever"
)

// Offer is one entry of the shop: its price in gems and what it gives.
type Offer struct {
	Price int64
	// WarpHours: payout hours of the player's current income (a TimeWarp grant), 0 for a boost.
	WarpHours float64
	// BoostSeconds: seconds of x2 income added to the running boost, 0 for a warp.
	BoostSeconds int64
	// Fever: a golden fever start (charged, nothing stored beyond the spend record).
	Fever bool
}

// Offers are decided by the user (handoff 4ad): 4 h = 20 gems, 8 h = 37, boost 10 min = 5, 30 min = 13.
var Offers = map[Product]Offer{
	ProductWarp4h:   {Price: 20, WarpHours: 4},
	ProductWarp8h:   {Price: 37, WarpHours: 8},
	ProductBoost10m: {Price: 5, BoostSeconds: 10 * 60},
	ProductBoost30m: {Price: 13, BoostSeconds: 30 * 60},
	ProductFever:    {Price: 5, Fever: true},
}

// Error codes returned in SpendResponse.ErrorCode (HTTP 200, ok=false).
const (
	CodeUnknownProduct   = "unknown_product"
	CodeInsufficientGems = "insufficient_gems"
	CodeWarpLimit        = "warp_limit"
	CodeWarpNotEnabled   = "warp_not_enabled"
	CodeBoostFull        = "boost_full"
)

var (
	ErrInvalidRequest = errors.New("invalid gem request")
	ErrTooManyGrants  = grant.ErrTooManyGrants
)

// Doc is stored at users/{uid}/private/gems.
type Doc struct {
	SchemaVersion int   `firestore:"schemaVersion"`
	Balance       int64 `firestore:"balance"`
	// Grant IDs whose Gem rewards are already in Balance.
	Credited map[string]time.Time `firestore:"credited"`

	// Payout hours bought on WarpDay ("2006-01-02", the server day that starts at ResetHourUTC).
	WarpDay   string  `firestore:"warpDay"`
	WarpHours float64 `firestore:"warpHours"`

	// Remaining x2 income, in seconds. The client runs the clock (only while the player is in a store) and
	// reports what it used; the server only caps the report at what is left.
	BoostSeconds int64 `firestore:"boostSeconds"`

	// Results of recent spends by request ID, so a retried request is answered, not charged twice.
	Spends map[string]*SpendRecord `firestore:"spends"`

	// Rewarded ads: recent results by request ID (idempotency) and when each placement may be rewarded again
	// ("fever:<store>", see ads.go).
	Ads       map[string]*AdRecord `firestore:"ads"`
	AdReadyAt map[string]time.Time `firestore:"adReadyAt"`

	UpdatedAt time.Time `firestore:"updatedAt"`
}

type SpendRecord struct {
	Product      Product   `firestore:"product"`
	Price        int64     `firestore:"price"`
	At           time.Time `firestore:"at"`
	BalanceAfter int64     `firestore:"balanceAfter"`
	// Set for a warp: the grant that carries the TimeWarp reward.
	GrantID string `firestore:"grantId,omitempty"`
}

func NewDoc() *Doc {
	return &Doc{SchemaVersion: SchemaVersion, Credited: map[string]time.Time{}, Spends: map[string]*SpendRecord{}}
}

// DayKey is the allowance day a moment belongs to.
func DayKey(t time.Time) string {
	return t.UTC().Add(-time.Duration(ResetHourUTC) * time.Hour).Format("2006-01-02")
}

// NextReset is when the allowance of the day containing t starts again.
func NextReset(t time.Time) time.Time {
	u := t.UTC().Add(-time.Duration(ResetHourUTC) * time.Hour)
	day := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	return day.Add(24*time.Hour + time.Duration(ResetHourUTC)*time.Hour)
}

// Settle credits the Gem rewards of grants that are not in Balance yet. It reports how many gems were added.
func (d *Doc) Settle(grants *grant.Doc, now time.Time) int64 {
	if d.Credited == nil {
		d.Credited = map[string]time.Time{}
	}
	var added int64
	for id, g := range grants.Grants {
		if _, done := d.Credited[id]; done {
			continue
		}
		var n int64
		for _, r := range g.Rewards {
			if r.Type == grant.RewardGem {
				n += int64(r.Amount)
			}
		}
		// A grant with no gems is marked too, so it is not looked at again.
		d.Credited[id] = now
		added += n
	}
	d.Balance += added
	return added
}

// rollDay starts a fresh allowance when the server day changed.
func (d *Doc) rollDay(now time.Time) {
	if day := DayKey(now); d.WarpDay != day {
		d.WarpDay = day
		d.WarpHours = 0
	}
}

// remember stores a spend result and drops the oldest ones past the bound.
func (d *Doc) remember(id string, rec *SpendRecord) {
	if d.Spends == nil {
		d.Spends = map[string]*SpendRecord{}
	}
	d.Spends[id] = rec
	for len(d.Spends) > maxSpendRecords {
		oldestID, oldest := "", time.Time{}
		for k, v := range d.Spends {
			if oldestID == "" || v.At.Before(oldest) {
				oldestID, oldest = k, v.At
			}
		}
		delete(d.Spends, oldestID)
	}
}

// API DTOs (mirrored in the Unity client when it gets the shop).

// State is what the client shows: GET /api/{version}/gems
type State struct {
	Balance int64 `json:"balance"`
	// Payout hours bought today and left today.
	WarpHoursUsed float64   `json:"warpHoursUsed"`
	WarpHoursLeft float64   `json:"warpHoursLeft"`
	WarpResetsAt  time.Time `json:"warpResetsAt"`
	// Seconds until the allowance resets, on the server clock: the client counts this down from the moment it
	// received it and does not trust its own clock.
	WarpResetsInSeconds int64 `json:"warpResetsInSeconds"`
	BoostSeconds        int64 `json:"boostSeconds"`
	ServerNowMs         int64 `json:"serverNowMs"`
}

// POST /api/{version}/gems/spend
type SpendRequest struct {
	Product Product `json:"product" validate:"required,max=32"`
	// Chosen by the client for each purchase attempt (a UUID); a retry with the same value is not charged again.
	RequestID string `json:"requestId" validate:"required,min=8,max=64"`
	// A warp only works in a store that owns the warp-enabled decoration (GDD 5.2.1). The economy runs on the
	// client, so the client reports it; the server refuses a warp without it.
	WarpEnabled bool `json:"warpEnabled"`
}

type SpendResponse struct {
	OK        bool   `json:"ok"`
	ErrorCode string `json:"errorCode,omitempty"`
	// The TimeWarp grant of a warp (the client applies it like any grant and reports it claimed).
	Grant *grant.Grant `json:"grant,omitempty"`
	State State        `json:"state"`
}

// POST /api/{version}/gems/boost/consume
type ConsumeRequest struct {
	Seconds int64 `json:"seconds" validate:"required,min=1,max=3600"`
}
