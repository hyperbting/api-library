package user

import "errors"

var (
	ErrInvalidPlatform        = errors.New("invalid or unsupported device platform")
	ErrInvalidIdentity        = errors.New("invalid user identity string format")
	ErrUnknownPlatformAcronym = errors.New("unknown platform acronym")
)
