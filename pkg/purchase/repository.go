package purchase

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PurchaseRepository interface {
	// CreatePlatformTransaction(ctx context.Context, tx *PlatformTransaction) error
	// CreateCoinTransaction(ctx context.Context, tx *CoinTransaction) error

	GetBalance(ctx context.Context, userID uint64) (uint64, error)
	GetLatestTransactionBalance(ctx context.Context, userID uint64) (uint64, error)
	ModifyBalanceTx(ctx context.Context, tx *gorm.DB, userID uint64, amount int64, reason string) (*model.CoinTransaction, error)
	GetLedgerHistory(ctx context.Context, userID uint64, limit, offset int) ([]model.CoinTransaction, error)
}

type purchaseRepositoryImpl struct {
	db *gorm.DB
}

func NewPurchaseRepository(db *gorm.DB) PurchaseRepository {
	return &purchaseRepositoryImpl{
		db: db,
	}
}

// GetBalance reads the hot-path single row balance directly from player_balances
func (r *purchaseRepositoryImpl) GetBalance(ctx context.Context, userID uint64) (uint64, error) {
	var balance model.PlayerBalance
	err := r.db.WithContext(ctx).
		Select("coins").
		Where("user_id = ?", userID).
		First(&balance).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return balance.Coins, err
}

// GetLatestTransactionBalance fetches the last balance state strictly from the ledger
func (r *purchaseRepositoryImpl) GetLatestTransactionBalance(ctx context.Context, userID uint64) (uint64, error) {
	var lastTx model.CoinTransaction
	err := r.db.WithContext(ctx).
		Select("balance_after").
		Where("user_id = ?", userID).
		Order("created_at DESC, id DESC").
		First(&lastTx).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return lastTx.BalanceAfter, err
}

// ModifyBalanceTx handles atomic credit/debit, row locking, and ledger entry generation
func (r *purchaseRepositoryImpl) ModifyBalanceTx(ctx context.Context, tx *gorm.DB, userID uint64, amount int64, reason string) (*model.CoinTransaction, error) {
	var balance model.PlayerBalance

	// 1. Lock the balance row for UPDATE to prevent race conditions
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ?", userID).
		FirstOrCreate(&balance, model.PlayerBalance{UserID: userID, Coins: 0}).Error
	if err != nil {
		return nil, fmt.Errorf("failed to lock balance row: %w", err)
	}

	// 2. Compute new balance and check for underflow
	var newBalance uint64
	if amount < 0 {
		absAmount := uint64(-amount)
		if balance.Coins < absAmount {
			return nil, ErrInsufficientBalance
		}
		newBalance = balance.Coins - absAmount
	} else {
		newBalance = balance.Coins + uint64(amount)
	}

	// 3. Update player balance table
	if err := tx.WithContext(ctx).Model(&balance).Update("coins", newBalance).Error; err != nil {
		return nil, fmt.Errorf("failed to update player balance: %w", err)
	}

	// 4. Create immutable ledger record
	coinTx := model.CoinTransaction{
		UserID:       userID,
		Amount:       amount,
		BalanceAfter: newBalance,
		Reason:       reason,
	}

	if err := tx.WithContext(ctx).Create(&coinTx).Error; err != nil {
		return nil, fmt.Errorf("failed to insert coin transaction: %w", err)
	}

	return &coinTx, nil
}

// GetLedgerHistory retrieves paginated transactions using the idx_user_ledger index
func (r *purchaseRepositoryImpl) GetLedgerHistory(ctx context.Context, userID uint64, limit, offset int) ([]model.CoinTransaction, error) {
	var txs []model.CoinTransaction
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&txs).Error

	if err != nil {
		return nil, fmt.Errorf("failed to fetch ledger history: %w", err)
	}
	return txs, nil
}
