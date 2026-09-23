package friend

import "time"

type RelationshipType string

const (
	RelTypeFollow RelationshipType = "fol"
	RelTypeBlock  RelationshipType = "blk"
)

type UserRelationship struct {
	ActorID   uint64           `gorm:"type:bigint;primaryKey;autoIncrement:false;column:actor_id;index:tid_act_atr,priority:3"`
	Type      RelationshipType `gorm:"type:varchar(16);primaryKey;column:type;index:tid_act_atr,priority:2"`
	TargetID  uint64           `gorm:"type:bigint;primaryKey;autoIncrement:false;column:target_id;index:tid_act_atr,priority:1"`
	CreatedAt time.Time        `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP;column:created_at"`
}

func (UserRelationship) TableName() string {
	return "user_relationships"
}
