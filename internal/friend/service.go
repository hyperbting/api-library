package friend

import "context"

type Service interface {
	FollowUser(ctx context.Context, actorID, targetID uint64) error
	BlockUser(ctx context.Context, actorID, targetID uint64) error
	GetFriends(ctx context.Context, userID uint64) ([]uint64, error)
}

func NewService(repo RelationshipRepository) Service {
	return &friendServiceImpl{repo: repo}
}

type friendServiceImpl struct {
	repo RelationshipRepository
}

func (s *friendServiceImpl) FollowUser(ctx context.Context, actorID, targetID uint64) error {
	if actorID == targetID {
		return ErrSelfFollow // Domain validation stays in Service
	}
	return s.repo.FollowUser(ctx, actorID, targetID)
}

func (s *friendServiceImpl) BlockUser(ctx context.Context, actorID, targetID uint64) error {
	if actorID == targetID {
		return ErrSelfBlock
	}
	return s.repo.BlockUser(ctx, actorID, targetID)
}

func (s *friendServiceImpl) GetFriends(ctx context.Context, userID uint64) ([]uint64, error) {
	return s.repo.GetFriends(ctx, userID)
}
