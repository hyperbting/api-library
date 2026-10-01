package app

import "errors"

var (
	ErrGameNotFound    = errors.New("game not found")
	ErrAppNotFound     = errors.New("app not found")
	ErrUserNotFound    = errors.New("user not found")
	ErrUserMismatch    = errors.New("user mismatch")
	ErrSessionNotFound = errors.New("session not found")
)
