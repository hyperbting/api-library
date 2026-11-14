package bond

import (
	"context"
	"fmt"
	"strings"
	"time"

	"api-library/internal/grant"
)

type Service interface {
	// State is GET /bond: every owned pet's hearts, seen ids and first-met time.
	State(ctx context.Context, uid string) (StateResponse, error)
	// Report applies one batch (POST /bond/report) and returns the new state with the rejected items.
	Report(ctx context.Context, uid string, req ReportRequest) (StateResponse, error)
	// AdminSet repairs one pet's record (PUT /admin/bond).
	AdminSet(ctx context.Context, req AdminSetRequest) (StateResponse, error)
}

type service struct {
	repo Repository
	now  func() time.Time
}

type Option func(*service)

// WithClock replaces the server clock (tests).
func WithClock(now func() time.Time) Option { return func(s *service) { s.now = now } }

func NewService(repo Repository, opts ...Option) Service {
	s := &service{repo: repo, now: time.Now}
	for _, o := range opts {
		o(s)
	}
	return s
}

func (s *service) State(ctx context.Context, uid string) (StateResponse, error) {
	doc, err := s.repo.Get(ctx, uid)
	if err != nil {
		return StateResponse{}, err
	}
	grants, err := s.repo.Grants(ctx, uid)
	if err != nil {
		return StateResponse{}, err
	}
	return buildState(doc, grants, s.now().UTC()), nil
}

func (s *service) Report(ctx context.Context, uid string, req ReportRequest) (StateResponse, error) {
	if err := validateReport(req); err != nil {
		return StateResponse{}, err
	}
	grants, err := s.repo.Grants(ctx, uid)
	if err != nil {
		return StateResponse{}, err
	}
	owned := ownedSet(grants)
	granted := grantTimes(grants)
	now := s.now().UTC()

	var (
		final     *Doc
		rejected  []Rejected
		duplicate bool
	)
	err = s.repo.Update(ctx, uid, func() Doc { return *newDoc(now) }, func(doc *Doc) (bool, error) {
		rejected, duplicate = applyReport(doc, owned, granted, req, now)
		final = doc
		return !duplicate, nil
	})
	if err != nil {
		return StateResponse{}, err
	}
	res := buildState(final, grants, now)
	res.Rejected = rejected
	res.Duplicate = duplicate
	return res, nil
}

func (s *service) AdminSet(ctx context.Context, req AdminSetRequest) (StateResponse, error) {
	req.UID = strings.TrimSpace(req.UID)
	if req.UID == "" || len(req.UID) > 128 {
		return StateResponse{}, fmt.Errorf("%w: uid", ErrInvalidRequest)
	}
	if !grant.KnownPets[req.Pet] {
		return StateResponse{}, fmt.Errorf("%w: unknown pet %q", ErrInvalidRequest, req.Pet)
	}
	if req.BondDays == nil && !req.ClearLastBondAt {
		return StateResponse{}, fmt.Errorf("%w: nothing to change", ErrInvalidRequest)
	}
	if req.BondDays != nil && (*req.BondDays < 0 || *req.BondDays > MaxAdminBondDays) {
		return StateResponse{}, fmt.Errorf("%w: bondDays must be 0..%d", ErrInvalidRequest, MaxAdminBondDays)
	}
	grants, err := s.repo.Grants(ctx, req.UID)
	if err != nil {
		return StateResponse{}, err
	}
	now := s.now().UTC()
	var final *Doc
	err = s.repo.Update(ctx, req.UID, func() Doc { return *newDoc(now) }, func(doc *Doc) (bool, error) {
		pb := doc.Pets[req.Pet]
		if pb == nil {
			pb = &PetBond{}
			doc.Pets[req.Pet] = pb
		}
		if req.BondDays != nil {
			pb.BondDays = *req.BondDays
		}
		if req.ClearLastBondAt {
			pb.LastBondAt = nil
		}
		doc.UpdatedAt = now
		final = doc
		return true, nil
	})
	if err != nil {
		return StateResponse{}, err
	}
	return buildState(final, grants, now), nil
}
