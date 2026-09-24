package friend

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RelationshipRepository interface {
	FollowUser(ctx context.Context, actorID, targetID uint64) error
	BlockUser(ctx context.Context, actorID, targetID uint64) error
	GetFriends(ctx context.Context, userID uint64) ([]uint64, error)
	GetUnreciprocatedFollows(ctx context.Context, userID uint64) ([]uint64, error)
	GetBlockedUsers(ctx context.Context, userID uint64) ([]uint64, error)
	GetBlockingUsers(ctx context.Context, userID uint64) ([]uint64, error)
}

type pgRelationshipRepoImpl struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) RelationshipRepository {
	return &pgRelationshipRepoImpl{db: db}
}

// // Fetch who I follow or block (Uses Index-Only Scan on Primary Key)
// func (r *pgRelationshipRepoImpl) GetOutboundTargets(ctx context.Context, actorID string, relType RelationshipType) ([]string, error) {
// 	query := `
// 		SELECT target_id
// 		FROM user_relationships
// 		WHERE actor_id = $1 AND type = $2`

// 	rows, err := r.db.QueryContext(ctx, query, actorID, relType)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer rows.Close()

// 	var targetIDs []string
// 	for rows.Next() {
// 		var targetID string
// 		if err := rows.Scan(&targetID); err != nil {
// 			return nil, err
// 		}
// 		targetIDs = append(targetIDs, targetID)
// 	}
// 	return targetIDs, rows.Err()
// }

// // Fetch who follows me (Uses idx_target_type_actor Index-Only Scan)
// func (r *pgRelationshipRepoImpl) GetInboundActors(ctx context.Context, targetID string, relType RelationshipType) ([]string, error) {
// 	query := `
// 		SELECT actor_id
// 		FROM user_relationships
// 		WHERE target_id = $1 AND type = $2`

// 	rows, err := r.db.QueryContext(ctx, query, targetID, relType)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer rows.Close()

// 	var actorIDs []string
// 	for rows.Next() {
// 		var actorID string
// 		if err := rows.Scan(&actorID); err != nil {
// 			return nil, err
// 		}
// 		actorIDs = append(actorIDs, actorID)
// 	}
// 	return actorIDs, rows.Err()
// }

// // Insert single relationship record in Postgres
// func (r *pgRelationshipRepoImpl) AddRelationship(ctx context.Context, actorID, targetID string, relType RelationshipType) error {
// 	query := `
// 		INSERT INTO user_relationships (actor_id, target_id, type)
// 		VALUES ($1, $2, $3)
// 		ON CONFLICT (actor_id, type, target_id)
// 		DO UPDATE SET created_at = CURRENT_TIMESTAMP`

// 	_, err := r.db.ExecContext(ctx, query, actorID, targetID, relType)
// 	return err
// }

// FollowUser handles the logic for Player A attempting to follow Player B
func (r *pgRelationshipRepoImpl) FollowUser(ctx context.Context, actorID, targetID uint64) error {
	if actorID == targetID {
		return ErrSelfFollow
	}

	// 啟動 GORM 事務
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 檢查 Block 關係 (於同一 tx 內執行)
		var count int64
		err := tx.Model(&UserRelationship{}).
			Where("(actor_id = ? AND target_id = ? AND type = ?) OR (actor_id = ? AND target_id = ? AND type = ?)",
				actorID, targetID, RelTypeBlock,
				targetID, actorID, RelTypeBlock,
			).Count(&count).Error

		if err != nil {
			return err
		}
		if count > 0 {
			return ErrUserBlocked // 被封鎖或已封鎖對方
		}

		// 2. 寫入 Follow 關係
		rel := UserRelationship{
			ActorID:  actorID,
			TargetID: targetID,
			Type:     RelTypeFollow,
		}

		// 使用 Clause 處理併發插入衝突 (ON CONFLICT DO NOTHING)
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rel)
		if result.Error != nil {
			return result.Error
		}

		// 若 RowsAffected == 0，代表紀錄已存在 (重複追蹤)
		if result.RowsAffected == 0 {
			return ErrAlreadyFollowing
		}

		// 3. (可選) 於同一 tx 內更新 User 追蹤數/粉絲數
		// if err := tx.Model(&User{})... ; err != nil { return err }

		return nil // 回傳 nil，GORM 會自動執行 tx.Commit()
	})
}

// BlockUser handles Player A blocking Player B (cleans up any existing follows in either direction)
func (r *pgRelationshipRepoImpl) BlockUser(ctx context.Context, actorID, targetID uint64) error {
	if actorID == targetID {
		return ErrSelfBlock
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 清除雙向的 Follow 關係 (A -> B 與 B -> A)
		err := tx.Where("type = ?", RelTypeFollow).
			Where(
				"(actor_id = ? AND target_id = ?) OR (actor_id = ? AND target_id = ?)",
				actorID, targetID, targetID, actorID,
			).
			Delete(&UserRelationship{}).Error

		if err != nil {
			return fmt.Errorf("failed to clean existing follows: %w", err)
		}

		// 2. 建立 Block 關係
		blockRel := UserRelationship{
			ActorID:  actorID,
			TargetID: targetID,
			Type:     RelTypeBlock,
		}

		// 使用 Clause 處理 ON CONFLICT DO NOTHING
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&blockRel)
		if result.Error != nil {
			return fmt.Errorf("failed to insert block: %w", result.Error)
		}

		// 若 RowsAffected == 0 代表該 Block 紀錄已存在
		if result.RowsAffected == 0 {
			return ErrAlreadyBlocked
		}

		return nil
	})
}

// GetFriends returns mutual follow user IDs
func (r *pgRelationshipRepoImpl) GetFriends(ctx context.Context, userID uint64) ([]uint64, error) {
	query := `
		SELECT a.target_id 
		FROM user_relationships a
		JOIN user_relationships b 
		  ON a.target_id = b.actor_id 
		 AND a.actor_id = b.target_id
		WHERE a.actor_id = $1 
		  AND a.type = $2 
		  AND b.type = $2`

	return r.scanIDs(ctx, query, userID, RelTypeFollow)
}

// GetUnreciprocatedFollows returns users followed by userID who do not follow back
func (r *pgRelationshipRepoImpl) GetUnreciprocatedFollows(ctx context.Context, userID uint64) ([]uint64, error) {
	query := `
		SELECT target_id 
		FROM user_relationships 
		WHERE actor_id = $1 AND type = $2
		EXCEPT
		SELECT actor_id 
		FROM user_relationships 
		WHERE target_id = $1 AND type = $2`

	return r.scanIDs(ctx, query, userID, RelTypeFollow)
}

// GetBlockedUsers returns users explicitly blocked by userID (Outbound)
func (r *pgRelationshipRepoImpl) GetBlockedUsers(ctx context.Context, userID uint64) ([]uint64, error) {
	query := `SELECT target_id FROM user_relationships WHERE actor_id = $1 AND type = $2`
	return r.scanIDs(ctx, query, userID, RelTypeBlock)
}

// GetBlockingUsers returns users who have blocked userID (Inbound)
func (r *pgRelationshipRepoImpl) GetBlockingUsers(ctx context.Context, userID uint64) ([]uint64, error) {
	query := `SELECT actor_id FROM user_relationships WHERE target_id = $1 AND type = $2`
	return r.scanIDs(ctx, query, userID, RelTypeBlock)
}

// Helper scanner function for returning standard uint64 ID slices
func (r *pgRelationshipRepoImpl) scanIDs(ctx context.Context, query string, args ...interface{}) ([]uint64, error) {
	var ids []uint64

	// 使用 GORM 的 Raw + Scan，自動處理 context、rows.Close() 與 rows.Err()
	err := r.db.WithContext(ctx).Raw(query, args...).Scan(&ids).Error
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	return ids, nil
}
