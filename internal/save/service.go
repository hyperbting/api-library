package save

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"api-library/internal/grant"
)

var ErrInvalidRequest = errors.New("invalid save request")

type Service interface {
	// Load returns the stored snapshot (nil State when none) and the grants not yet claimed.
	Load(ctx context.Context, uid string) (*LoadResponse, error)
	// Sync stores a snapshot based on BaseRev and marks ClaimedGrants as applied, in one transaction.
	// Rejections (stale rev, regressing progress) are ok=false with the server state, not errors.
	Sync(ctx context.Context, uid string, req SyncRequest) (*SyncResponse, error)
	// Reset drops the user's save (admin only; the route requires the admin role).
	Reset(ctx context.Context, uid string) error
}

type service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) Service {
	return &service{repo: repo, now: time.Now}
}

func (s *service) Load(ctx context.Context, uid string) (*LoadResponse, error) {
	doc, grants, err := s.repo.Get(ctx, uid)
	if err != nil {
		return nil, err
	}
	return &LoadResponse{
		ServerNowMs: s.now().UnixMilli(),
		State:       stateOf(doc),
		Grants:      grants.Unclaimed(),
	}, nil
}

func (s *service) Reset(ctx context.Context, uid string) error {
	return s.repo.Delete(ctx, uid)
}

func (s *service) Sync(ctx context.Context, uid string, req SyncRequest) (*SyncResponse, error) {
	if err := validate(req); err != nil {
		return nil, err
	}

	var res *SyncResponse
	err := s.repo.Update(ctx, uid, func(doc *Doc, grants *grant.Doc) (bool, bool, error) {
		// fn may be retried by the transaction, so build the response from scratch each time.
		now := s.now()
		res = &SyncResponse{ServerNowMs: now.UnixMilli()}

		switch {
		case doc.Rev != req.BaseRev:
			res.ErrorCode = CodeRevConflict
		case doc.Rev > 0 && regressed(req.Summary, doc.Summary):
			res.ErrorCode = CodeRegressed
		}
		if res.ErrorCode != "" {
			res.Rev = doc.Rev
			res.Server = stateOf(doc)
			res.Grants = grants.Unclaimed()
			return false, false, nil
		}

		doc.SchemaVersion = SchemaVersion
		doc.Rev++
		doc.SavedAt = now
		doc.ClientTimeMs = req.ClientTimeMs
		doc.Summary = req.Summary
		doc.Data = req.Data

		claimed := grants.Claim(req.ClaimedGrants, now) > 0
		if claimed {
			grants.UpdatedAt = now
		}

		res.OK = true
		res.Rev = doc.Rev
		res.Grants = grants.Unclaimed()
		return true, claimed, nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func stateOf(doc *Doc) *ServerState {
	if doc == nil {
		return nil
	}
	return &ServerState{Rev: doc.Rev, SavedAt: doc.SavedAt, Summary: doc.Summary, Data: doc.Data}
}

// Progress only grows: earnings below the stored lifetime total mean a broken or reset client, not play.
// The tolerance absorbs float noise on very large numbers.
func regressed(next, stored Summary) bool {
	return next.LifetimeEarned < stored.LifetimeEarned*(1-1e-9)
}

func validate(req SyncRequest) error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidRequest, fmt.Sprintf(format, args...))
	}

	if req.BaseRev < 0 {
		return bad("baseRev")
	}
	if len(req.Data) == 0 || len(req.Data) > MaxDataBytes {
		return bad("data size %d", len(req.Data))
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(req.Data), &probe); err != nil {
		return bad("data is not a JSON object")
	}

	finiteNonNegative := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }
	if !finiteNonNegative(req.Summary.Cash) || !finiteNonNegative(req.Summary.LifetimeEarned) {
		return bad("summary amounts")
	}
	if len(req.Summary.Venues) > MaxVenues {
		return bad("too many venues")
	}
	for id, level := range req.Summary.Venues {
		if id == "" || len(id) > 64 || level < 0 || level > MaxVenueLevel {
			return bad("venue %q", id)
		}
	}

	if len(req.ClaimedGrants) > MaxClaimedGrants {
		return bad("too many claimed grants")
	}
	for _, id := range req.ClaimedGrants {
		if id == "" || len(id) > MaxIDLength {
			return bad("claimed grant id")
		}
	}
	return nil
}
