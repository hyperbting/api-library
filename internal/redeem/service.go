package redeem

import (
	"context"
	"fmt"
	"time"

	"api-library/internal/grant"
	"api-library/internal/item"
)

type Service interface {
	// Use redeems a typed code for the user. A refused code is ok=false with an error code, not an error.
	Use(ctx context.Context, uid, rawCode string) (*UseResponse, error)

	// Admin
	CreateCode(ctx context.Context, adminUID string, req CreateCodeRequest) (*Code, error)
	SetDisabled(ctx context.Context, rawCode string, disabled bool) error
	ListCodes(ctx context.Context) ([]Code, error)
}

type service struct {
	repo    Repository
	catalog *item.Catalog
	now     func() time.Time
}

func NewService(repo Repository, catalog *item.Catalog) Service {
	return &service{repo: repo, catalog: catalog, now: time.Now}
}

// Use refuses in this order: rate limited, unknown, already redeemed by this user, disabled, expired, used up.
// Failed attempts count against a per-user hourly limit (kept on the user's grants document), except
// "already redeemed", which is not a guess. The real protection against guessing is the 60-bit code space:
// a new anonymous account resets the counter.
func (s *service) Use(ctx context.Context, uid, rawCode string) (*UseResponse, error) {
	code, ok := Normalize(rawCode)
	if !ok {
		code = "" // still counts as a failed attempt below
	}

	var res *UseResponse
	err := s.repo.Redeem(ctx, uid, code, func(c *Code, grants *grant.Doc) (bool, bool, error) {
		// fn may be retried by the transaction, so build the response from scratch each time.
		now := s.now()
		res = &UseResponse{}

		if grants.RedeemLocked(now) {
			res.ErrorCode = CodeRateLimited
			return false, false, nil
		}

		fail := func(errorCode string) (bool, bool, error) {
			grants.NoteRedeemFailure(now)
			grants.UpdatedAt = now
			res.ErrorCode = errorCode
			return false, true, nil
		}

		if c == nil {
			return fail(CodeInvalid)
		}
		id := grantID(c.Code)
		if grants.Has(id) {
			res.ErrorCode = CodeAlreadyRedeemed
			return false, false, nil
		}
		switch {
		case c.Disabled:
			return fail(CodeDisabled)
		case c.ExpiresAt != nil && !now.Before(*c.ExpiresAt):
			return fail(CodeExpired)
		case c.MaxRedemptions > 0 && c.Redeemed >= c.MaxRedemptions:
			return fail(CodeFinished)
		}

		// A code whose items no longer resolve (an item was removed from the catalog) is a server-side
		// mistake, not the player's: an error (HTTP 500, logged), not a refusal that counts as a guess.
		rewards, err := s.catalog.Resolve(c.Items)
		if err != nil {
			return false, false, fmt.Errorf("code %s: %w", c.Code, err)
		}
		g := grant.Grant{
			ID:        id,
			Source:    grant.SourceRedeem,
			SourceRef: c.Code,
			Rewards:   rewards,
			CreatedAt: now,
		}
		if _, err := grants.Add(g); err != nil {
			return false, false, err
		}
		grants.UpdatedAt = now
		c.Redeemed++

		stored := *grants.Grants[id]
		res.OK = true
		res.Grant = &stored
		return true, true, nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func grantID(code string) string { return "redeem:" + code }

func (s *service) CreateCode(ctx context.Context, adminUID string, req CreateCodeRequest) (*Code, error) {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidRequest, fmt.Sprintf(format, args...))
	}

	// Every item must exist and the resolved rewards must be valid (e.g. a time warp within its cap)
	if _, err := s.catalog.Resolve(req.Items); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	if req.MaxRedemptions < 0 || req.MaxRedemptions > MaxRedemptionsLimit {
		return nil, bad("maxRedemptions")
	}
	now := s.now()
	if req.ExpiresAt != nil && !req.ExpiresAt.After(now) {
		return nil, bad("expiresAt is in the past")
	}
	if len(req.Note) > maxNoteLength {
		return nil, bad("note too long")
	}

	code := ""
	if req.Code != "" {
		var ok bool
		if code, ok = Normalize(req.Code); !ok {
			return nil, bad("code must be %d-%d letters or digits", minCodeLength, maxCodeLength)
		}
	}

	// A generated code that collides (about 2^-60 per pair) is simply retried.
	for attempt := 0; attempt < 5; attempt++ {
		c := code
		if c == "" {
			var err error
			if c, err = generate(); err != nil {
				return nil, err
			}
		}
		stored := &Code{
			Code:           c,
			Items:          append([]item.Ref(nil), req.Items...),
			MaxRedemptions: req.MaxRedemptions,
			ExpiresAt:      req.ExpiresAt,
			Note:           req.Note,
			CreatedBy:      adminUID,
			CreatedAt:      now,
		}
		err := s.repo.Create(ctx, stored)
		if err == nil {
			return stored, nil
		}
		if err != ErrCodeExists || code != "" {
			return nil, err
		}
	}
	return nil, ErrCodeExists
}

func (s *service) SetDisabled(ctx context.Context, rawCode string, disabled bool) error {
	code, ok := Normalize(rawCode)
	if !ok {
		return fmt.Errorf("%w: code", ErrInvalidRequest)
	}
	return s.repo.SetDisabled(ctx, code, disabled)
}

func (s *service) ListCodes(ctx context.Context) ([]Code, error) {
	return s.repo.List(ctx)
}
