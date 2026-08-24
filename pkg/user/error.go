package user

import "errors"

var (
	ErrInvalidPlatform        = errors.New("invalid or unsupported device platform")
	ErrInvalidIdentity        = errors.New("invalid user identity string format")
	ErrUnknownPlatformAcronym = errors.New("unknown platform acronym")
	ErrUnknownUserID          = errors.New("unknown User ID")
	ErrPasswordHashFailed     = errors.New("failed to hash password")
	ErrNoPassword             = errors.New("user has no password set")
	ErrUserIDMustBeNum        = errors.New("User ID must be a number")
	ErrPlatformMustBeString   = errors.New("Platform must be a string")
	ErrPasswordEmpty          = errors.New("password cannot be empty")

	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrEmailAlreadyExists = errors.New("email is already registered")
	ErrUserNotFound       = errors.New("user not found")
	ErrEmailNotFound      = errors.New("email not found")
	ErrFailedToCreateUser = errors.New("failed to create user")
)
