package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionNotFound    = errors.New("session not found")
	ErrTokenRevoked       = errors.New("token has been revoked")
	ErrUserAlreadyExists  = errors.New("user already exists")
)
