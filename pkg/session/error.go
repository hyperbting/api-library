package session

import "errors"

var (
	ErrSecretKeyEmpty = errors.New("JWT secret key cannot be empty")
	ErrIssuerEmpty    = errors.New("JWT issuer cannot be empty")
	ErrTTLEmpty       = errors.New("JWT TTL cannot be empty")

	ErrSessionNotFound = errors.New("session not found")
	ErrTokenRevoked    = errors.New("token has been revoked")
)
