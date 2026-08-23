package auth

import (
	"context"
	"time"
)

type Service interface {
	// Login(ctx context.Context, userID, password string) (accessToken string, refreshToken string, err error)
	// Logout(ctx context.Context, userID string) error
	// LogoutAllDevices(ctx context.Context, userID string) error
	Refresh(ctx context.Context, refreshTokenStr string) (newAccessToken string, newRefreshToken string, err error)
	ValidateAccessToken(ctx context.Context, accessTokenStr string) (*AccessClaims, error)
}

type serviceImpl struct {
	tokenMgr TokenManager      // Interface from jwt.go
	repo     SessionRepository // Interface from repo
}

func NewService(tm TokenManager, repo SessionRepository) Service {
	return &serviceImpl{
		tokenMgr: tm,
		repo:     repo,
	}
}

// func (s *serviceImpl) Login(ctx context.Context, userID, password string) (string, string, error) {

// }

func (s *serviceImpl) ValidateAccessToken(ctx context.Context, accessTokenStr string) (*AccessClaims, error) {
	return s.tokenMgr.ValidateAccessToken(accessTokenStr)
}

func (s *serviceImpl) Refresh(ctx context.Context, refreshTokenStr string) (string, string, error) {
	// 1. Statelessly parse and verify the Refresh Token via jwt.go
	claims, err := s.tokenMgr.ValidateRefreshToken(refreshTokenStr)
	if err != nil {
		return "", "", err
	}

	// 2. Fetch the stored session from Redis via repo using claims.UserID
	session, err := s.repo.Get(ctx, claims.UserID, claims.ID)
	if err != nil {
		return "", "", err
	}

	now := time.Now()
	// 3. Compare JTIs (Check if token is blacklisted/revoked)
	if !session.Matches(claims, now) {
		return "", "", ErrTokenRevoked
	}

	roles := []string{"player"}
	// 4. Generate new token pair via jwt.go
	newAT, newRT, newJTI, err := s.tokenMgr.GenerateTokenPair(claims.UserID, roles)
	if err != nil {
		return "", "", err
	}

	// 5. Update active session in Redis via repo
	newSession := session.Rotate(newJTI, s.tokenMgr.RefreshExpiration(now))
	if err := s.repo.Save(ctx, &newSession, s.tokenMgr.RefreshTTL()); err != nil {
		return "", "", err
	}

	// 6. Clean up the old session key immediately
	_ = s.repo.Delete(ctx, claims.UserID, claims.ID)

	return newAT, newRT, nil
}
