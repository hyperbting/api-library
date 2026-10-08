package quest

import "errors"

var (
	ErrInvalidCatalog = errors.New("invalid quest catalog")
	ErrInvalidRequest = errors.New("invalid quest request")
)
