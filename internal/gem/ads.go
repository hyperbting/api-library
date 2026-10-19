package gem

import (
	"context"
	"math"
	"time"

	"api-library/internal/grant"
)

// Rewarded ads (decided 2026-10-15, GDD 5.2.1): the client shows the ad, then posts the reward here, so the server
// keeps the things that must not depend on a local file: the fever cooldown and the shared allowance of warp hours.
// Three placements exist:
//
//	fever           starts the store's golden fever; one per cooldown (per store), the sale itself is client side
//	offline_double  doubles the offline earnings; only recorded (the doubling itself is client side)
//	warp            one payout hour of the player's income (a TimeWarp grant) per ad, counted against the same
//	                8 h per day as the gem warps (double while the x2 boost runs); no limit on ads beyond that
//
// Without verification the server cannot see the ad, so a modified client can claim a reward without one; what it
// cannot do is beat the cooldown or the day's allowance. With an AdVerifier (WithAdVerification, package adssv) a
// request is rewarded only after AdMob's server-side verification callback for its requestId has arrived: until then
// the answer is ad_pending and the client asks again with the same requestId.

type Placement string

const (
	PlacementFever         Placement = "fever"
	PlacementOfflineDouble Placement = "offline_double"
	PlacementWarp          Placement = "warp"
)

const (
	// FeverAdCooldown is the wait between two ad-started fevers of one store.
	FeverAdCooldown = 5 * time.Minute
	// AdWarpHours is the payout hours one warp ad gave in the first build; now InvestHours[level] (invest.go).
	AdWarpHours = 1.0

	CodeUnknownPlacement = "unknown_placement"
	CodeAdCooldown       = "ad_cooldown"
	// CodeAdPending: the verification callback for this requestId has not arrived (yet). Ask again.
	CodeAdPending = "ad_pending"

	// AdPendingRetrySeconds is the wait the client is told to keep between two asks.
	AdPendingRetrySeconds = 3

	maxAdRecords = 64
)

// AdRecord is the stored result of one ad reward, by request ID, so a retried request is answered, not repeated.
type AdRecord struct {
	Placement Placement `firestore:"placement"`
	At        time.Time `firestore:"at"`
	GrantID   string    `firestore:"grantId,omitempty"`
}

// POST /api/{version}/ads/reward
type AdRewardRequest struct {
	Placement Placement `json:"placement" validate:"required,max=32"`
	// Chosen by the client for each ad (a UUID); a retry with the same value is not rewarded again.
	RequestID string `json:"requestId" validate:"required,min=8,max=64"`
	// fever: which store (its place address). The cooldown is per store.
	Store string `json:"store" validate:"max=64"`
	// warp: the warp-enabled decoration is owned in the active store (reported by the client, as for a gem warp).
	WarpEnabled bool `json:"warpEnabled"`
}

type AdRewardResponse struct {
	OK        bool   `json:"ok"`
	ErrorCode string `json:"errorCode,omitempty"`
	// fever, refused by the cooldown: seconds until the next ad start.
	RetryAfterSeconds int64 `json:"retryAfterSeconds,omitempty"`
	// warp: the TimeWarp grant (the client applies it like any grant and reports it claimed).
	Grant *grant.Grant `json:"grant,omitempty"`
	State State        `json:"state"`
}

func (d *Doc) rememberAd(id string, rec *AdRecord) {
	if d.Ads == nil {
		d.Ads = map[string]*AdRecord{}
	}
	d.Ads[id] = rec
	for len(d.Ads) > maxAdRecords {
		oldestID, oldest := "", time.Time{}
		for k, v := range d.Ads {
			if oldestID == "" || v.At.Before(oldest) {
				oldestID, oldest = k, v.At
			}
		}
		delete(d.Ads, oldestID)
	}
}

func adReadyKey(p Placement, store string) string { return string(p) + ":" + store }

// AdReward records a rewarded ad and applies what it gives. A refusal is ok=false with an error code.
func (s *service) AdReward(ctx context.Context, uid string, req AdRewardRequest) (*AdRewardResponse, error) {
	switch req.Placement {
	case PlacementFever, PlacementOfflineDouble, PlacementWarp:
	default:
		st, err := s.State(ctx, uid)
		if err != nil {
			return nil, err
		}
		return &AdRewardResponse{ErrorCode: CodeUnknownPlacement, State: *st}, nil
	}

	// Verification reads another collection, so it is asked before the transaction too. A request that was already
	// rewarded still finds its callback (the record outlives the retry window), so a retry is answered as before.
	if s.ads != nil {
		ok, err := s.ads.Verified(ctx, uid, req.RequestID)
		if err != nil {
			return nil, err
		}
		if !ok {
			st, err := s.State(ctx, uid)
			if err != nil {
				return nil, err
			}
			return &AdRewardResponse{ErrorCode: CodeAdPending, RetryAfterSeconds: AdPendingRetrySeconds, State: *st}, nil
		}
	}

	// The gate reads another document, so it is asked before the transaction (see Spend).
	warpAllowed := true
	if req.Placement == PlacementWarp {
		var err error
		if warpAllowed, err = s.warpAllowed(ctx, uid, req.WarpEnabled); err != nil {
			return nil, err
		}
	}

	var res *AdRewardResponse
	err := s.repo.Run(ctx, uid, func(d *Doc, grants *grant.Doc) (bool, bool, error) {
		// fn may be retried by the transaction: build the response from scratch each time.
		now := s.now()
		res = &AdRewardResponse{}
		d.Settle(grants, now)
		d.rollDay(now)
		d.UpdatedAt = now

		refuse := func(code string) (bool, bool, error) {
			res.ErrorCode = code
			res.State = s.state(d, now)
			return true, false, nil // the credit and the day roll are real changes
		}

		// A retried request: answer with what happened, reward nothing.
		if rec := d.Ads[req.RequestID]; rec != nil {
			if rec.Placement != req.Placement {
				return refuse(CodeUnknownPlacement)
			}
			res.OK = true
			if rec.GrantID != "" {
				if g := grants.Grants[rec.GrantID]; g != nil {
					stored := *g
					res.Grant = &stored
				}
			}
			res.State = s.state(d, now)
			return true, false, nil
		}

		rec := &AdRecord{Placement: req.Placement, At: now}
		switch req.Placement {
		case PlacementFever:
			key := adReadyKey(req.Placement, req.Store)
			if ready, ok := d.AdReadyAt[key]; ok && now.Before(ready) {
				res.RetryAfterSeconds = int64(ready.Sub(now).Seconds()) + 1
				return refuse(CodeAdCooldown)
			}
			if d.AdReadyAt == nil {
				d.AdReadyAt = map[string]time.Time{}
			}
			d.AdReadyAt[key] = now.Add(FeverAdCooldown)

		case PlacementWarp:
			if !warpAllowed {
				return refuse(CodeWarpNotEnabled)
			}
			// Same rule as a gem warp: counted double while the x2 boost runs, 8 payout hours per day in all. The hours
			// are the player's Investment level (invest.go); an ad that would go past the day's allowance pays what
			// is left of it, since the ad is already watched (the refusal is only for an allowance that is used up).
			hours := d.InvestAdHours()
			factor := 1.0
			if d.BoostSeconds > 0 {
				factor = 2
			}
			if room := (MaxWarpHoursPerDay - d.WarpHours) / factor; room < hours {
				hours = math.Floor(room*100+1e-9) / 100 // whole hundredths, so the sum never drifts past the cap
			}
			if hours < 0.01 {
				return refuse(CodeWarpLimit)
			}
			counted := hours * factor
			g := grant.Grant{
				ID:        "ad:" + req.RequestID,
				Source:    grant.SourceAds,
				SourceRef: string(req.Placement),
				Rewards: []grant.Reward{{
					Type:   grant.RewardTimeWarp,
					Amount: hours,
					ItemID: "warp_ad",
					Count:  1,
				}},
				CreatedAt: now,
			}
			if _, err := grants.Add(g); err != nil {
				return false, false, err
			}
			d.WarpHours += counted
			rec.GrantID = g.ID
			stored := *grants.Grants[g.ID]
			res.Grant = &stored

		case PlacementOfflineDouble:
			// Only recorded: the doubling happens on the client.
		}

		d.rememberAd(req.RequestID, rec)
		res.OK = true
		res.State = s.state(d, now)
		return true, res.Grant != nil, nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// AdVerifier says whether AdMob confirmed that this player finished the ad behind requestId.
type AdVerifier interface {
	Verified(ctx context.Context, uid, requestID string) (bool, error)
}

// WithAdVerification makes every ad reward wait for its verification callback (see the top of this file).
func WithAdVerification(v AdVerifier) Option {
	return func(s *service) { s.ads = v }
}
