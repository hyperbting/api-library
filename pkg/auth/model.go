package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrTokenRevoked    = errors.New("token has been revoked")
	ErrSessionNotFound = errors.New("session not found")
)

// Session represents the value stored in Redis under the key "session:<user_id>"
type Session struct {
	UserID    string    `json:"user_uuid"`
	JTI       string    `json:"jti"`        // Unique ID of the active Refresh Token
	Device    string    `json:"device"`     // Optional metadata (e.g., "iOS", "Web")
	ExpiresAt time.Time `json:"expires_at"` // When the Refresh Token expires
}

func (s *Session) Matches(claims *RefreshClaims, ts time.Time) bool {
	return s.UserID == claims.UserID &&
		s.JTI == claims.ID &&
		ts.Before(s.ExpiresAt)
}

// Rotate creates a new Session copy with updated JTI and Expiration.
func (s *Session) Rotate(newJTI string, expiresAt time.Time) Session {
	cloned := *s
	cloned.JTI = newJTI
	cloned.ExpiresAt = expiresAt
	return cloned
}

// AccessClaims defines the payload inside the short-lived Access Token
type AccessClaims struct {
	UserID string   `json:"user_id"`
	Roles  []string `json:"roles"`
	jwt.RegisteredClaims
}

// RefreshClaims defines the payload inside the long-lived Refresh Token
type RefreshClaims struct {
	UserID               string `json:"user_uuid"` // Standard 'sub' claim or explicit field
	jwt.RegisteredClaims        // Contains 'jti' and 'exp'
}
