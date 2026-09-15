package purchase

import (
	"time"
)

type PlayerBalance struct {
	UserID    uint      `json:"user_id" gorm:"primaryKey;type:bigint;autoIncrement:false"`
	Coins     uint64    `json:"coins" gorm:"type:bigint;not null;default:0"`
	Gems      uint64    `json:"gems" gorm:"type:bigint;not null;default:0"`
	Energy    uint16    `json:"energy" gorm:"type:smallint;not null;default:100"`
	UpdatedAt time.Time `json:"updated_at" gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
}

func (PlayerBalance) TableName() string {
	return "player_balances"
}
