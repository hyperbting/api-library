package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// TokenManager defines the contract for creating and validating tokens[cite: 1]
type TokenManager interface {
	GenerateTokenPair(userID string, roles []string) (accessToken string, refreshToken string, jti string, err error)
	ValidateAccessToken(tokenStr string) (*AccessClaims, error)
	ValidateRefreshToken(tokenStr string) (*RefreshClaims, error)
	RefreshTTL() time.Duration
	RefreshExpiration(issuedAt time.Time) time.Time
}

// Config holds settings for JWT generation
type Config struct {
	SecretKey  string
	Issuer     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

// manager is the unexported implementation of TokenManager[cite: 1]
type manager struct {
	secretKey  []byte
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewTokenManager creates a new instance of TokenManager[cite: 1]
func NewTokenManager(cfg Config) TokenManager {
	if cfg.SecretKey == "" {
		panic("JWT secret key cannot be empty")
	}
	return &manager{
		secretKey:  []byte(cfg.SecretKey),
		issuer:     cfg.Issuer,
		accessTTL:  cfg.AccessTTL,
		refreshTTL: cfg.RefreshTTL,
	}
}

func (m *manager) RefreshTTL() time.Duration {
	return m.refreshTTL
}

func (m *manager) RefreshExpiration(issuedAt time.Time) time.Time {
	return issuedAt.Add(m.refreshTTL)
}

// GenerateTokenPair creates both short-lived AT and long-lived RT
func (m *manager) GenerateTokenPair(userID string, roles []string) (accessTokenJWT string, refreshTokenJWT string, jti string, err error) {
	now := time.Now()

	// 1. Generate Access Token Claims
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
	if accessTokenJWT, err = jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(m.secretKey); err != nil {
		err = fmt.Errorf("failed to sign access token: %w", err)
		return
	}

	// 2. Generate Refresh Token Claims (with unique JTI)
	jti = uuid.NewString()
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
	if refreshTokenJWT, err = jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString(m.secretKey); err != nil {
		err = fmt.Errorf("failed to sign refresh token: %w", err)
		return
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
