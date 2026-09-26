package meta

import "errors"

var (
	ErrBanned       = errors.New("player banned")
	ErrDeviceBanned = errors.New("device banned")
	ErrNotFound     = errors.New("not found")

	ErrSecretNotMatched   = errors.New("secret not found")
	ErrEmptyDeveloperLoad = errors.New("developer_payload is empty")
	//ErrEventNotFound = errors.New("webhook event not found in context")
	//ErrInvalidEvent = errors.New("invalid event type in context")
	ErrRawBodyNotFound       = errors.New("raw body not found in fiber context")
	ErrVerifyTokenMismatched = errors.New("verify token mismatch")
)
