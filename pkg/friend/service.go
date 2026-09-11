package friend

import "context"

type Service struct {
	repo RelationshipRepository
}

func NewService(repo RelationshipRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) FollowUser(ctx context.Context, actorID, targetID uint64) error {
	if actorID == targetID {
		return ErrSelfFollow // Domain validation stays in Service
	}
	return s.repo.FollowUser(ctx, actorID, targetID)
}

func (s *Service) BlockUser(ctx context.Context, actorID, targetID uint64) error {
	if actorID == targetID {
		return ErrSelfBlock
	}
	return s.repo.BlockUser(ctx, actorID, targetID)
}

func (s *Service) GetFriends(ctx context.Context, userID uint64) ([]uint64, error) {
	return s.repo.GetFriends(ctx, userID)
}
