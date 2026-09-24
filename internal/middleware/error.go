package middleware

import "errors"

var (
	// HTTP-layer errors for the middleware
	ErrMissingAuthHeader  = errors.New("missing authorization header")
	ErrInvalidTokenFormat = errors.New("invalid token format, expected 'Bearer <token>'")

	// Session errors (delegated from session package)
	ErrTokenRevoked    = errors.New("token has been revoked")
	ErrSessionNotFound = errors.New("session not found")
)
