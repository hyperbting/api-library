package quest

import "errors"

var (
	ErrInvalidCatalog = errors.New("invalid quest catalog")
	ErrInvalidRequest = errors.New("invalid quest request")

	// Admin edits only.
	ErrUnknownQuest = errors.New("unknown quest")
	ErrInvalidStep  = errors.New("step index out of range")
)
