package meta

import "errors"

var (
	ErrBanned       = errors.New("player banned")
	ErrDeviceBanned = errors.New("device banned")
	ErrNotFound     = errors.New("not found")
)
