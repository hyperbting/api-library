package purchase

import (
	"time"
)

type PaymentSource string
type PaymentStatus string

const (
	SourceiOS        PaymentSource = "ios"
	SourceGooglePlay PaymentSource = "google_play"
	SourceAdMob      PaymentSource = "ad_mob"

	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusCompleted PaymentStatus = "completed"
	PaymentStatusFailed    PaymentStatus = "failed"
)

type PlatformTransaction struct {
	ID             uint          `json:"id" gorm:"primaryKey;autoIncrement;type:bigint"`
	UserID         uint          `json:"user_id" gorm:"type:bigint;not null;index:idx_user_created,priority:1"`
	ItemID         uint          `json:"item_id" gorm:"type:bigint;not null;default:0"`
	Amount         uint          `json:"amount" gorm:"type:bigint;not null;default:0"` // Fiat price in cents (e.g., 999 = $9.99 USD)
	Currency       string        `json:"currency" gorm:"type:varchar(3);not null;default:'USD'"`
	Source         PaymentSource `json:"source" gorm:"type:varchar(20);not null;index:idx_source_status,priority:1"`
	Status         PaymentStatus `json:"status" gorm:"type:varchar(20);not null;default:'pending';index:idx_source_status,priority:2;index:idx_status_created,priority:1"`
	TransactionRef string        `json:"transaction_ref,omitempty" gorm:"type:varchar(255);uniqueIndex:idx_platform_tx_ref"`

	CreatedAt time.Time `json:"created_at" gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;index:idx_user_created,priority:2,sort:desc;index:idx_status_created,priority:2"`
}

func (PlatformTransaction) TableName() string {
	return "platform_transactions"
}
