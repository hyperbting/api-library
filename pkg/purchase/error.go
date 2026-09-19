package purchase

import "errors"

var (
	ErrInsufficientBalance   = errors.New("insufficient balance")
	ErrBalanceLocked         = errors.New("balance locked")
	ErrBalanceNotFound       = errors.New("balance not found")
	ErrBalanceAlreadyExists  = errors.New("balance already exists")
	ErrBalanceUpdateFailed   = errors.New("balance update failed")
	ErrBalanceDeleteFailed   = errors.New("balance delete failed")
	ErrBalanceGetFailed      = errors.New("balance get failed")
	ErrBalanceCreateFailed   = errors.New("balance create failed")
	ErrBalanceReadFailed     = errors.New("balance read failed")
	ErrBalanceWriteFailed    = errors.New("balance write failed")
	ErrBalanceLockFailed     = errors.New("balance lock failed")
	ErrBalanceUnlockFailed   = errors.New("balance unlock failed")
	ErrBalanceReleaseFailed  = errors.New("balance release failed")
	ErrBalanceRollbackFailed = errors.New("balance rollback failed")
	ErrBalanceCommitFailed   = errors.New("balance commit failed")
)
