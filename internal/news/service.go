package news

import (
	"context"
	"sort"
	"strings"
	"time"
)

type Service interface {
	// ForPlayer is what GET /news returns: not expired, the dev items only when includeDev (an admin caller),
	// pinned first, then newest first.
	ForPlayer(ctx context.Context, includeDev bool) ([]Item, error)
	// All is the panel's list: every item, expired ones included, same order.
	All(ctx context.Context) ([]Item, error)
	// Save creates an item (empty ID) or updates one. The publish time is set on create and kept on update.
	Save(ctx context.Context, req SaveRequest) (*Item, error)
	Delete(ctx context.Context, id string) error
}

type service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) Service {
	return &service{repo: repo, now: time.Now}
}

func (s *service) ForPlayer(ctx context.Context, includeDev bool) ([]Item, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := make([]Item, 0, len(items))
	for _, item := range items {
		if item.Expired(now) {
			continue
		}
		if item.Audience == AudienceDev && !includeDev {
			continue
		}
		out = append(out, item)
	}
	sortItems(out)
	return out, nil
}

func (s *service) All(ctx context.Context) ([]Item, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	sortItems(items)
	return items, nil
}

func (s *service) Save(ctx context.Context, req SaveRequest) (*Item, error) {
	if msg := req.Validate(); msg != "" {
		return nil, &RequestError{Message: msg}
	}

	item := &Item{
		ID:        strings.TrimSpace(req.ID),
		Audience:  req.Audience,
		Title:     req.Title,
		Body:      req.Body,
		Version:   req.Version,
		Pinned:    req.Pinned,
		ExpiresAt: req.ExpiresAt,
	}

	if item.ID == "" {
		item.PublishedAt = s.now()
		id, err := s.repo.Create(ctx, item)
		if err != nil {
			return nil, err
		}
		item.ID = id
		return item, nil
	}

	existing, err := s.repo.Get(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	item.PublishedAt = existing.PublishedAt
	if err := s.repo.Update(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, strings.TrimSpace(id))
}

// RequestError carries the reason a save was refused (shown in the panel).
type RequestError struct{ Message string }

func (e *RequestError) Error() string { return ErrInvalidRequest.Error() + ": " + e.Message }
func (e *RequestError) Unwrap() error { return ErrInvalidRequest }

// Pinned first, then newest.
func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Pinned != items[j].Pinned {
			return items[i].Pinned
		}
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})
}
