package gem

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"api-library/internal/grant"
	"api-library/internal/item"
)

// Admin side of the gem ledger (panel page "Gems"): look at one player's balance, spends and grants, and gift
// them gems or items. Everything an admin does goes through the same grant ledger as a purchase or a redeem
// code, so it is visible in the player's history and credited by the same rule.

// Limits of one gift, so a typo cannot hand out a fortune.
const (
	MaxGiftGems = 100_000
	maxGiftNote = 120
	// How many of the newest grants and spends the overview returns.
	overviewGrantLimit = 100
)

// AdminSpend is one remembered spend (the last maxSpendRecords of the player).
type AdminSpend struct {
	RequestID    string    `json:"requestId"`
	Product      Product   `json:"product"`
	Price        int64     `json:"price"`
	At           time.Time `json:"at"`
	BalanceAfter int64     `json:"balanceAfter"`
	GrantID      string    `json:"grantId,omitempty"`
}

// AdminGrant is a grant as the panel lists it: what it gave, from where, and whether the gems in it are counted.
type AdminGrant struct {
	grant.Grant
	Claimed bool `json:"claimed"`
	// Gems is the sum of the Gem rewards of the grant; Credited tells whether they are in the balance yet.
	Gems     int64 `json:"gems"`
	Credited bool  `json:"credited"`
}

// GET /api/{version}/admin/gems/user?uid=...
type AdminOverview struct {
	UID    string       `json:"uid"`
	State  State        `json:"state"`
	Spends []AdminSpend `json:"spends"`
	Grants []AdminGrant `json:"grants"`
	// False when the player has neither a gems nor a grants document: the uid may be mistyped.
	HasRecords bool `json:"hasRecords"`
}

// POST /api/{version}/admin/gems/gift
type GiftRequest struct {
	UID string `json:"uid" validate:"required,max=128"`
	// Chosen by the panel for each submit (a UUID): a double click or a retry does not gift twice.
	RequestID string `json:"requestId" validate:"required,min=8,max=64"`
	// Gems is given as a Gem reward as is; Items go through the item catalog (gems_50, timewarp_1h, ...).
	Gems  int64      `json:"gems" validate:"min=0,max=100000"`
	Items []item.Ref `json:"items" validate:"max=5,dive"`
	Note  string     `json:"note" validate:"max=120"`
}

type GiftResponse struct {
	OK        bool         `json:"ok"`
	ErrorCode string       `json:"errorCode,omitempty"`
	Grant     *grant.Grant `json:"grant,omitempty"`
	State     State        `json:"state"`
	// True when the same request ID was already gifted: nothing new was created.
	Repeated bool `json:"repeated,omitempty"`
}

// Admin error codes (HTTP 200, ok=false).
const (
	CodeEmptyGift   = "empty_gift"
	CodeInvalidGift = "invalid_gift"
)

func (s *service) AdminOverview(ctx context.Context, uid string) (*AdminOverview, error) {
	if uid == "" || len(uid) > 128 {
		return nil, fmt.Errorf("%w: uid", ErrInvalidRequest)
	}
	var out AdminOverview
	err := s.repo.Run(ctx, uid, func(d *Doc, grants *grant.Doc) (bool, bool, error) {
		now := s.now()
		// Work on what a read would show, without writing it: the credit is applied in memory only.
		credited := map[string]bool{}
		for id := range d.Credited {
			credited[id] = true
		}
		d.Settle(grants, now)
		out = AdminOverview{
			UID:        uid,
			State:      s.state(d, now),
			Spends:     []AdminSpend{},
			Grants:     []AdminGrant{},
			HasRecords: len(grants.Grants) > 0 || len(credited) > 0 || len(d.Spends) > 0,
		}

		for id, rec := range d.Spends {
			out.Spends = append(out.Spends, AdminSpend{
				RequestID: id, Product: rec.Product, Price: rec.Price, At: rec.At, BalanceAfter: rec.BalanceAfter, GrantID: rec.GrantID,
			})
		}
		sort.Slice(out.Spends, func(i, j int) bool { return out.Spends[i].At.After(out.Spends[j].At) })

		for _, g := range grants.Grants {
			ag := AdminGrant{Grant: *g, Claimed: g.ClaimedAt != nil, Credited: credited[g.ID]}
			for _, r := range g.Rewards {
				if r.Type == grant.RewardGem {
					ag.Gems += int64(r.Amount)
				}
			}
			out.Grants = append(out.Grants, ag)
		}
		sort.Slice(out.Grants, func(i, j int) bool {
			if !out.Grants[i].CreatedAt.Equal(out.Grants[j].CreatedAt) {
				return out.Grants[i].CreatedAt.After(out.Grants[j].CreatedAt)
			}
			return out.Grants[i].ID < out.Grants[j].ID
		})
		if len(out.Grants) > overviewGrantLimit {
			out.Grants = out.Grants[:overviewGrantLimit]
		}
		return false, false, nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Gift gives a player gems and/or catalog items as an "admin" grant. The gems of the grant are credited the next
// time the gem service reads the player's state, like any other grant.
func (s *service) Gift(ctx context.Context, adminUID string, req GiftRequest) (*GiftResponse, error) {
	if req.UID == "" || req.RequestID == "" {
		return nil, fmt.Errorf("%w: uid and requestId", ErrInvalidRequest)
	}
	if req.Gems <= 0 && len(req.Items) == 0 {
		return &GiftResponse{ErrorCode: CodeEmptyGift}, nil
	}
	if req.Gems < 0 || req.Gems > MaxGiftGems {
		return &GiftResponse{ErrorCode: CodeInvalidGift}, nil
	}

	var rewards []grant.Reward
	if len(req.Items) > 0 {
		if s.catalog == nil {
			return nil, fmt.Errorf("%w: no item catalog", ErrInvalidRequest)
		}
		resolved, err := s.catalog.Resolve(req.Items)
		if err != nil {
			return &GiftResponse{ErrorCode: CodeInvalidGift}, nil
		}
		rewards = append(rewards, resolved...)
	}
	if req.Gems > 0 {
		rewards = append(rewards, grant.Reward{Type: grant.RewardGem, Amount: float64(req.Gems)})
	}
	if err := grant.ValidateRewards(rewards); err != nil {
		return &GiftResponse{ErrorCode: CodeInvalidGift}, nil
	}

	note := strings.TrimSpace(req.Note)
	if len(note) > maxGiftNote {
		note = note[:maxGiftNote]
	}
	id := "admin:" + req.RequestID

	var res *GiftResponse
	err := s.repo.Run(ctx, req.UID, func(d *Doc, grants *grant.Doc) (bool, bool, error) {
		// fn may be retried by the transaction: build the response from scratch each time.
		now := s.now()
		res = &GiftResponse{}

		if g := grants.Grants[id]; g != nil {
			stored := *g
			res.OK, res.Repeated, res.Grant = true, true, &stored
			res.State = s.state(d, now)
			return false, false, nil
		}

		g := grant.Grant{
			ID:        id,
			Source:    grant.SourceAdmin,
			SourceRef: strings.TrimSpace(adminUID + " " + note),
			Rewards:   rewards,
			CreatedAt: now,
		}
		if _, err := grants.Add(g); err != nil {
			return false, false, err
		}
		grants.UpdatedAt = now
		// Credit now, so the state in the answer already shows the gift.
		d.Settle(grants, now)
		d.UpdatedAt = now

		stored := *grants.Grants[id]
		res.OK, res.Grant = true, &stored
		res.State = s.state(d, now)
		return true, true, nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}
