package friend

import "errors"

var (
	ErrSelfBlock        = errors.New("cannot block yourself")
	ErrSelfFollow       = errors.New("cannot follow yourself")
	ErrUserBlocked      = errors.New("cannot follow a user you have blocked")
	ErrTargetBlockedYou = errors.New("cannot follow this user")
)
