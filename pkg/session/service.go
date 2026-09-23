package session

import (
	"context"
	"time"
)

// Service defines the session and token management contract.
// It is intentionally independent of any user/identity package so it can be
// reused as a standalone library.
type Service interface {
	// CreateSession generates a token pair and persists the session for a user.
	CreateSession(ctx context.Context, userID string, roles []string) (*TokenPair, error)

	// ValidateAccessToken validates an access token and returns its claims.
	ValidateAccessToken(ctx context.Context, accessTokenStr string) (*AccessClaims, error)

	// RefreshToken rotates the refresh token, returning a new token pair.
	RefreshToken(ctx context.Context, refreshTokenStr string) (accessToken, refreshToken string, err error)

	// RemoveSession destroys an active session identified by (userID, jti).
	RemoveSession(ctx context.Context, userID, jti string) error
}

type authServiceImpl struct {
	tokenMgr TokenManager
	repo     SessionRepository
}

func NewService(tm TokenManager, repo SessionRepository) Service {
	return &authServiceImpl{
		tokenMgr: tm,
		repo:     repo,
	}
}

// CreateSession generates a token pair and stores the session in Redis.
func (s *authServiceImpl) CreateSession(ctx context.Context, userID string, roles []string) (*TokenPair, error) {
	tp, err := s.tokenMgr.GenerateTokenPair(userID, roles)
	if err != nil {
		return nil, err
	}

	session := &Session{
		UserID:    userID,
		Roles:     roles,
		JTI:       tp.JTI,
		ExpiresAt: tp.RefreshExpiresAt,
	}

	if err := s.repo.SaveSession(ctx, session); err != nil {
		return nil, err
	}

	return tp, nil
}

// ValidateAccessToken validates an access token and returns its claims.
func (s *authServiceImpl) ValidateAccessToken(ctx context.Context, accessTokenStr string) (*AccessClaims, error) {
	return s.tokenMgr.ValidateAccessToken(accessTokenStr)
}

// RefreshToken rotates the refresh token, returning a new token pair.
func (s *authServiceImpl) RefreshToken(ctx context.Context, refreshTokenStr string) (accessToken, refreshToken string, err error) {
	// 1. Statelessly parse and verify the Refresh Token
	claims, err := s.tokenMgr.ValidateRefreshToken(refreshTokenStr)
	if err != nil {
		return "", "", err
	}

	// 2. Fetch the stored session from Redis
	session, err := s.repo.Get(ctx, claims.UserID, claims.ID)
	if err != nil {
		return "", "", err
	}

	// 3. Compare JTIs (check if token is blacklisted/revoked)
	if !session.Matches(claims, time.Now()) {
		return "", "", ErrTokenRevoked
	}

	// 4. Generate a new token pair using the roles stored in the session
	tp, err := s.tokenMgr.GenerateTokenPair(claims.UserID, session.Roles)
	if err != nil {
		return "", "", err
	}

	// 5. Update the active session in Redis
	newSession := session.Rotate(tp.JTI, tp.RefreshExpiresAt)
	if err := s.repo.SaveSession(ctx, newSession); err != nil {
		return "", "", err
	}

	// 6. Clean up the old session key immediately
	_ = s.repo.Delete(ctx, claims.UserID, claims.ID)

	return tp.AccessToken, tp.RefreshToken, nil
}

// Logout destroys an active session.
func (s *authServiceImpl) RemoveSession(ctx context.Context, userID, jti string) error {
	return s.repo.Delete(ctx, userID, jti)
}
