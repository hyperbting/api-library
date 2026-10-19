package gem

import (
	"context"
	"math"
	"time"

	"api-library/internal/grant"
	"api-library/internal/item"
)

type Service interface {
	// State credits any gems granted since the last read and reports the balance and the limits.
	State(ctx context.Context, uid string) (*State, error)
	// Spend buys a product with gems. A refusal is ok=false with an error code, not an error.
	Spend(ctx context.Context, uid string, req SpendRequest) (*SpendResponse, error)
	// ConsumeBoost takes seconds of in-store play off the remaining x2 boost (never below zero).
	ConsumeBoost(ctx context.Context, uid string, seconds int64) (*State, error)
	// AdReward records a rewarded ad and applies its reward (fever cooldown, a warp hour, offline double): ads.go.
	AdReward(ctx context.Context, uid string, req AdRewardRequest) (*AdRewardResponse, error)

	// Admin (panel page "Gems"): one player's balance, spends and grants; a gift of gems and / or items.
	AdminOverview(ctx context.Context, uid string) (*AdminOverview, error)
	Gift(ctx context.Context, adminUID string, req GiftRequest) (*GiftResponse, error)
}

type service struct {
	repo    Repository
	catalog *item.Catalog // for gifts of catalog items; may be nil (gems only)
	gate    WarpGate      // warps need it on top of the client's flag (warpgate.go); nil = the flag only
	ads     AdVerifier    // ad rewards wait for AdMob's callback (ads.go); nil = the client's word
	now     func() time.Time
}

func NewService(repo Repository, catalog *item.Catalog, opts ...Option) Service {
	s := &service{repo: repo, catalog: catalog, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *service) state(d *Doc, now time.Time) State {
	used := d.WarpHours
	if d.WarpDay != DayKey(now) {
		used = 0 // a new day starts the allowance again, even before the next spend writes it
	}
	left := MaxWarpHoursPerDay - used
	if left < 0 {
		left = 0
	}
	resetAt := NextReset(now)
	return State{
		Balance:             d.Balance,
		WarpHoursUsed:       used,
		WarpHoursLeft:       left,
		WarpResetsAt:        resetAt,
		WarpResetsInSeconds: int64(math.Ceil(resetAt.Sub(now).Seconds())),
		BoostSeconds:        d.BoostSeconds,
		ServerNowMs:         now.UnixMilli(),
	}
}

func (s *service) State(ctx context.Context, uid string) (*State, error) {
	var out State
	err := s.repo.Run(ctx, uid, func(d *Doc, grants *grant.Doc) (bool, bool, error) {
		now := s.now()
		added := d.Settle(grants, now)
		if added > 0 {
			d.UpdatedAt = now
		}
		out = s.state(d, now)
		// Marking a grant without gems as looked at is also a write, but it is not worth one on its own.
		return added > 0, false, nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Spend checks in this order: the product, a repeated request, a warp that is not enabled here (the client's flag
// and the WarpGate), gems, the daily allowance (warp) or the boost cap (boost). All inside one transaction with
// the credit, so a balance can never be spent twice by two devices. The gate is read before the transaction (a
// different document), so a retried request is still answered from its record.
func (s *service) Spend(ctx context.Context, uid string, req SpendRequest) (*SpendResponse, error) {
	offer, known := Offers[req.Product]
	if !known {
		st, err := s.State(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &SpendResponse{ErrorCode: CodeUnknownProduct, State: *st}, nil
	}

	warpAllowed := true
	if offer.WarpHours > 0 {
		var err error
		if warpAllowed, err = s.warpAllowed(ctx, uid, req.WarpEnabled); err != nil {
			return nil, err
		}
	}

	var res *SpendResponse
	err := s.repo.Run(ctx, uid, func(d *Doc, grants *grant.Doc) (bool, bool, error) {
		// fn may be retried by the transaction: build the response from scratch each time.
		now := s.now()
		res = &SpendResponse{}
		d.Settle(grants, now)
		d.rollDay(now)
		d.UpdatedAt = now

		refuse := func(code string) (bool, bool, error) {
			res.ErrorCode = code
			res.State = s.state(d, now)
			// The credit and the day roll are real changes: keep them.
			return true, false, nil
		}

		// A retried request: answer with what happened, charge nothing.
		if rec := d.Spends[req.RequestID]; rec != nil {
			res.OK = rec.Product == req.Product
			if !res.OK {
				return refuse(CodeUnknownProduct)
			}
			if rec.GrantID != "" {
				if g := grants.Grants[rec.GrantID]; g != nil {
					stored := *g
					res.Grant = &stored
				}
			}
			res.State = s.state(d, now)
			return true, false, nil
		}

		if offer.WarpHours > 0 && !warpAllowed {
			return refuse(CodeWarpNotEnabled)
		}
		if d.Balance < offer.Price {
			return refuse(CodeInsufficientGems)
		}

		switch {
		case offer.WarpHours > 0:
			// A warp bought while the x2 boost runs counts double against the day's limit.
			counted := offer.WarpHours
			if d.BoostSeconds > 0 {
				counted *= 2
			}
			if d.WarpHours+counted > MaxWarpHoursPerDay+1e-9 {
				return refuse(CodeWarpLimit)
			}
			g := grant.Grant{
				ID:        "gem:" + req.RequestID,
				Source:    grant.SourceGems,
				SourceRef: string(req.Product),
				Rewards: []grant.Reward{{
					Type:   grant.RewardTimeWarp,
					Amount: offer.WarpHours,
					ItemID: string(req.Product),
					Count:  1,
				}},
				CreatedAt: now,
			}
			if _, err := grants.Add(g); err != nil {
				return false, false, err
			}
			d.WarpHours += counted
			d.Balance -= offer.Price
			d.remember(req.RequestID, &SpendRecord{Product: req.Product, Price: offer.Price, At: now, BalanceAfter: d.Balance, GrantID: g.ID})
			stored := *grants.Grants[g.ID]
			res.Grant = &stored

		case offer.Fever:
			d.Balance -= offer.Price
			d.remember(req.RequestID, &SpendRecord{Product: req.Product, Price: offer.Price, At: now, BalanceAfter: d.Balance})

		default:
			if d.BoostSeconds >= MaxBoostSeconds {
				return refuse(CodeBoostFull)
			}
			d.BoostSeconds = int64(math.Min(float64(d.BoostSeconds+offer.BoostSeconds), MaxBoostSeconds))
			d.Balance -= offer.Price
			d.remember(req.RequestID, &SpendRecord{Product: req.Product, Price: offer.Price, At: now, BalanceAfter: d.Balance})
		}

		res.OK = true
		res.State = s.state(d, now)
		return true, res.Grant != nil, nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *service) ConsumeBoost(ctx context.Context, uid string, seconds int64) (*State, error) {
	if seconds < 1 {
		seconds = 0
	}
	if seconds > MaxConsumeSeconds {
		seconds = MaxConsumeSeconds
	}
	var out State
	err := s.repo.Run(ctx, uid, func(d *Doc, grants *grant.Doc) (bool, bool, error) {
		now := s.now()
		added := d.Settle(grants, now)
		used := seconds
		if used > d.BoostSeconds {
			used = d.BoostSeconds
		}
		d.BoostSeconds -= used
		if added > 0 || used > 0 {
			d.UpdatedAt = now
		}
		out = s.state(d, now)
		return added > 0 || used > 0, false, nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}
