package app

import "errors"

var (
	ErrGameNotFound    = errors.New("game not found")
	ErrAppNotFound     = errors.New("app not found")
	ErrUserNotFound    = errors.New("user not found")
	ErrUserMismatch    = errors.New("user mismatch")
	ErrSessionNotFound = errors.New("session not found")

	ErrConfigNil         = errors.New("app config is nil")
	ErrDatabaseConfigNil = errors.New("database config is nil")
	ErrSessionSrvNotSet  = errors.New("session service is not set")
)
