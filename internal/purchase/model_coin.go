package purchase

import "time"

type CoinTransaction struct {
	ID           uint      `json:"id" gorm:"primaryKey;autoIncrement;type:bigint"`
	UserID       uint      `json:"user_id" gorm:"type:bigint;not null;index:idx_user_coin_tx"`
	Amount       int64     `json:"amount" gorm:"type:bigint;not null"` // +500 (credit) or -100 (debit)
	BalanceAfter uint64    `json:"balance_after" gorm:"type:bigint;not null"`
	Reason       string    `json:"reason" gorm:"type:varchar(128);not null"` // "platform_fulfill", "shop_item_buy"
	CreatedAt    time.Time `json:"created_at" gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;index:idx_user_created,priority:2,sort:desc;index:idx_status_created,priority:2"`
}

func (CoinTransaction) TableName() string {
	return "coin_transactions"
}
