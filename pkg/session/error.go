package session

import "errors"

var (
	ErrSessionNotFound    = errors.New("session not found")
	ErrTokenRevoked       = errors.New("token has been revoked")
)
