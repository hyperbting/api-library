// Package grant is the per-user ledger of rewards the server hands out (redeem codes now; IAP receipts and
// support gifts later). A grant is created once, is idempotent by ID, and is delivered to the client, which
// applies it and reports it back as claimed together with its next save snapshot (see package save).
package grant

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

// Firestore layout (server-only; security rules deny all client access to users/{uid}/private/*):
//
//	users/{uid}/private/grants   Doc, one document per user
const (
	UsersCollection   = "users"
	PrivateCollection = "private"
	GrantsDocID       = "grants"

	SchemaVersion = 1

	// Upper bound on stored grants per user, so the document stays small. A claimed grant is kept
	// (its ID is what makes a redeem or a receipt idempotent).
	MaxGrants = 500

	MaxRewardsPerGrant = 10

	// Redeem throttle: failed attempts allowed per window, counted on the user's grants document
	// (Cloud Run has no Redis, and instances do not share memory).
	RedeemMaxFailures = 10
	RedeemWindow      = time.Hour
)

type RewardType string

// Values match the quest reward names in the Unity client (QuestRewardType) where they overlap.
const (
	RewardCash   RewardType = "Cash"
	RewardPull   RewardType = "Pull"
	RewardUnlock RewardType = "Unlock"
	// RewardTimeWarp: Amount is hours of the player's current income. The server cannot know the income
	// (the economy runs on the client), so the client computes the cash when it applies the grant.
	RewardTimeWarp RewardType = "TimeWarp"
)

type Source string

const (
	SourceRedeem Source = "redeem"
	SourceIAP    Source = "iap"
	SourceAdmin  Source = "admin"
)

var (
	ErrInvalidReward = errors.New("invalid reward")
	ErrTooManyGrants = errors.New("too many grants")
)

// Reward mirrors DTO.Quest.GrantedReward in the Unity client ({type, key, amount, itemId, count}).
// Amount is the total: the item's unit amount times Count. ItemID and Count are for display only (the
// client applies Type, Key and Amount), and are empty for rewards that did not come from an item.
type Reward struct {
	Type   RewardType `firestore:"type" json:"type"`
	Key    string     `firestore:"key" json:"key"`
	Amount float64    `firestore:"amount" json:"amount"`
	ItemID string     `firestore:"itemId,omitempty" json:"itemId,omitempty"`
	Count  int        `firestore:"count,omitempty" json:"count,omitempty"`
}

func (r Reward) Validate() error {
	if math.IsNaN(r.Amount) || math.IsInf(r.Amount, 0) || r.Amount < 0 {
		return fmt.Errorf("%w: amount %v", ErrInvalidReward, r.Amount)
	}
	if len(r.Key) > 64 {
		return fmt.Errorf("%w: key too long", ErrInvalidReward)
	}
	switch r.Type {
	case RewardCash, RewardPull:
		if r.Amount <= 0 {
			return fmt.Errorf("%w: %s needs an amount", ErrInvalidReward, r.Type)
		}
	case RewardTimeWarp:
		if r.Amount <= 0 || r.Amount > 24*7 {
			return fmt.Errorf("%w: TimeWarp hours must be in (0, 168]", ErrInvalidReward)
		}
	case RewardUnlock:
		if r.Key == "" {
			return fmt.Errorf("%w: Unlock needs a key", ErrInvalidReward)
		}
	default:
		return fmt.Errorf("%w: unknown type %q", ErrInvalidReward, r.Type)
	}
	return nil
}

// ValidateRewards checks a reward list as it would be stored in a grant or a code.
func ValidateRewards(rewards []Reward) error {
	if len(rewards) == 0 || len(rewards) > MaxRewardsPerGrant {
		return fmt.Errorf("%w: need 1..%d rewards", ErrInvalidReward, MaxRewardsPerGrant)
	}
	for _, r := range rewards {
		if err := r.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Grant is one delivery of rewards. ID is deterministic per source ("redeem:<CODE>", later "iap:<txn>"),
// which is what makes creating it twice a no-op.
type Grant struct {
	ID        string    `firestore:"id" json:"id"`
	Source    Source    `firestore:"source" json:"source"`
	SourceRef string    `firestore:"sourceRef" json:"sourceRef"`
	Rewards   []Reward  `firestore:"rewards" json:"rewards"`
	CreatedAt time.Time `firestore:"createdAt" json:"createdAt"`
	// Set when the client reported the grant as applied; nil while it is still waiting.
	ClaimedAt *time.Time `firestore:"claimedAt,omitempty" json:"-"`
}

// Doc is stored at users/{uid}/private/grants.
type Doc struct {
	SchemaVersion int               `firestore:"schemaVersion"`
	Grants        map[string]*Grant `firestore:"grants"`
	UpdatedAt     time.Time         `firestore:"updatedAt"`

	// Redeem throttle: failed attempts since RedeemWindowStart.
	RedeemFailures    int       `firestore:"redeemFailures"`
	RedeemWindowStart time.Time `firestore:"redeemWindowStart"`
}

func NewDoc() *Doc {
	return &Doc{SchemaVersion: SchemaVersion, Grants: map[string]*Grant{}}
}

func (d *Doc) Has(id string) bool {
	_, ok := d.Grants[id]
	return ok
}

// Add stores g unless a grant with the same ID exists (then it returns false and changes nothing).
func (d *Doc) Add(g Grant) (bool, error) {
	if d.Grants == nil {
		d.Grants = map[string]*Grant{}
	}
	if d.Has(g.ID) {
		return false, nil
	}
	if len(d.Grants) >= MaxGrants {
		return false, ErrTooManyGrants
	}
	c := g
	c.Rewards = append([]Reward(nil), g.Rewards...)
	d.Grants[g.ID] = &c
	return true, nil
}

// Claim marks the named grants as applied; unknown or already claimed IDs are ignored. It reports how many changed.
func (d *Doc) Claim(ids []string, now time.Time) int {
	n := 0
	for _, id := range ids {
		if g := d.Grants[id]; g != nil && g.ClaimedAt == nil {
			t := now
			g.ClaimedAt = &t
			n++
		}
	}
	return n
}

// Unclaimed returns the grants still waiting for the client, oldest first.
func (d *Doc) Unclaimed() []Grant {
	out := []Grant{}
	for _, g := range d.Grants {
		if g.ClaimedAt == nil {
			out = append(out, *g)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// RedeemLocked reports whether the user used up the failed attempts of the current window.
func (d *Doc) RedeemLocked(now time.Time) bool {
	if now.Sub(d.RedeemWindowStart) >= RedeemWindow {
		return false
	}
	return d.RedeemFailures >= RedeemMaxFailures
}

// NoteRedeemFailure counts a failed attempt, starting a new window when the last one has passed.
func (d *Doc) NoteRedeemFailure(now time.Time) {
	if now.Sub(d.RedeemWindowStart) >= RedeemWindow {
		d.RedeemWindowStart = now
		d.RedeemFailures = 0
	}
	d.RedeemFailures++
}
