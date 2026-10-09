package session

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TokenManager defines the contract for creating and validating tokens[cite: 1]
type TokenManager interface {
	GenerateTokenPair(userID string, roles []string) (tp *TokenPair, err error)
	// GenerateAccessToken issues a standalone access token with no refresh token
	// and no persisted session (used by stateless email/password logins).
	GenerateAccessToken(userID string, roles []string) (at *AccessToken, err error)
	ValidateAccessToken(tokenStr string) (*AccessClaims, error)
	ValidateRefreshToken(tokenStr string) (*RefreshClaims, error)
	// RefreshTTL() time.Duration
	// RefreshExpiration(issuedAt time.Time) time.Time
}

// Config holds settings for JWT generation
type Config struct {
	SecretKey  string        `mapstructure:"secret_key"`
	Issuer     string        `mapstructure:"issuer"`
	AccessTTL  time.Duration `mapstructure:"access_ttl"`
	RefreshTTL time.Duration `mapstructure:"refresh_ttl"`
}

// manager is the unexported implementation of TokenManager[cite: 1]
type manager struct {
	secretKey  []byte
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewTokenManager creates a new instance of TokenManager[cite: 1]
func NewTokenManager(cfg *Config) (TokenManager, error) {
	if cfg == nil {
		return nil, ErrConfigNil
	}
	if cfg.SecretKey == "" {
		return nil, ErrSecretKeyEmpty
	}
	if cfg.Issuer == "" {
		return nil, ErrIssuerEmpty
	}
	if cfg.AccessTTL == 0 {
		return nil, ErrTTLEmpty
	}
	if cfg.RefreshTTL == 0 {
		return nil, ErrTTLEmpty
	}

	return &manager{secretKey: []byte(cfg.SecretKey), issuer: cfg.Issuer, accessTTL: cfg.AccessTTL, refreshTTL: cfg.RefreshTTL}, nil
}

// func (m *manager) AccessTTL() time.Duration {
// 	return m.accessTTL
// }

// func (m *manager) AccessExpiration(issuedAt time.Time) time.Time {
// 	return issuedAt.Add(m.accessTTL)
// }

// func (m *manager) RefreshTTL() time.Duration {
// 	return m.refreshTTL
// }

// func (m *manager) RefreshExpiration(issuedAt time.Time) time.Time {
// 	return issuedAt.Add(m.refreshTTL)
// }

type TokenPair struct {
	AccessToken      string
	RefreshToken     string
	JTI              string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

// buildAccessToken signs a short-lived access token for the given user/roles at time now.
func (m *manager) buildAccessToken(userID string, roles []string, now time.Time) (string, time.Time, error) {
	accessClaims := AccessClaims{
		UserID: userID,
		Roles:  roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTTL)),
		},
	}

	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(m.secretKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign access token: %w", err)
	}
	return tokenStr, accessClaims.ExpiresAt.Time, nil
}

// GenerateAccessToken issues a standalone access token, no refresh token and no session store.
func (m *manager) GenerateAccessToken(userID string, roles []string) (*AccessToken, error) {
	tokenStr, expiresAt, err := m.buildAccessToken(userID, roles, time.Now())
	if err != nil {
		return nil, err
	}
	return &AccessToken{Token: tokenStr, ExpiresAt: expiresAt}, nil
}

// GenerateTokenPair creates both short-lived AT and long-lived RT
func (m *manager) GenerateTokenPair(userID string, roles []string) (tp *TokenPair, err error) {
	now := time.Now()

	// 1. Generate the access token
	accessTokenJWT, accessExpiresAt, err := m.buildAccessToken(userID, roles, now)
	if err != nil {
		return nil, err
	}

	// 2. Generate Refresh Token Claims (with unique JTI)
	jti := uuid.NewString()
	refreshClaims := RefreshClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.refreshTTL)),
		},
	}

	var refreshTokenJWT string
	if refreshTokenJWT, err = jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString(m.secretKey); err != nil {
		err = fmt.Errorf("failed to sign refresh token: %w", err)
		return
	}

	tp = &TokenPair{
		AccessToken:      accessTokenJWT,
		RefreshToken:     refreshTokenJWT,
		JTI:              jti,
		AccessExpiresAt:  accessExpiresAt,
		RefreshExpiresAt: refreshClaims.ExpiresAt.Time,
	}

	return
}

// ValidateAccessToken parses and validates an incoming Access Token string
func (m *manager) ValidateAccessToken(tokenStr string) (*AccessClaims, error) {
	claims := &AccessClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, m.keyFunc)
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid or expired access token: %w", err)
	}
	return claims, nil
}

// ValidateRefreshToken parses and validates an incoming Refresh Token string
func (m *manager) ValidateRefreshToken(tokenStr string) (*RefreshClaims, error) {
	claims := &RefreshClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, m.keyFunc)
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid or expired refresh token: %w", err)
	}
	return claims, nil
}

// keyFunc validates the signing algorithm (prevents 'none' algorithm attacks)
func (m *manager) keyFunc(token *jwt.Token) (any, error) {
	if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	}
	return m.secretKey, nil
}
