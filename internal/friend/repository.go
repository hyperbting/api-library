package friend

import (
	"context"
	"database/sql"
	"fmt"
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
	db *sql.DB
}

func NewPGRelationshipRepository(db *sql.DB) RelationshipRepository {
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

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Check if Actor has blocked Target, OR Target has blocked Actor
	checkQuery := `
		SELECT actor_id, target_id 
		FROM user_relationships 
		WHERE (actor_id = $1 AND target_id = $2 AND type = $3)
		   OR (actor_id = $2 AND target_id = $1 AND type = $3)`

	rows, err := tx.QueryContext(ctx, checkQuery, actorID, targetID, RelTypeBlock)
	if err != nil {
		return fmt.Errorf("failed to check block status: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var blkActor, blkTarget uint64
		if err := rows.Scan(&blkActor, &blkTarget); err != nil {
			return err
		}
		if blkActor == actorID {
			return ErrUserBlocked // You blocked them, unblock first
		}
		return ErrTargetBlockedYou // They blocked you, deny action
	}

	// 2. Insert the follow relationship atomically
	insertQuery := `
		INSERT INTO user_relationships (actor_id, type, target_id) 
		VALUES ($1, $3, $2) 
		ON CONFLICT (actor_id, type, target_id) DO NOTHING`

	if _, err := tx.ExecContext(ctx, insertQuery, actorID, targetID, RelTypeFollow); err != nil {
		return fmt.Errorf("failed to insert follow: %w", err)
	}

	return tx.Commit()
}

// BlockUser handles Player A blocking Player B (cleans up any existing follows in either direction)
func (r *pgRelationshipRepoImpl) BlockUser(ctx context.Context, actorID, targetID uint64) error {
	if actorID == targetID {
		return ErrSelfBlock
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Delete existing follow rows in BOTH directions (A->B and B->A)
	deleteFollowsQuery := `
		DELETE FROM user_relationships 
		WHERE type = $3 
		  AND ((actor_id = $1 AND target_id = $2) OR (actor_id = $2 AND target_id = $1))`

	if _, err := tx.ExecContext(ctx, deleteFollowsQuery, actorID, targetID, RelTypeFollow); err != nil {
		return fmt.Errorf("failed to clean existing follows: %w", err)
	}

	// 2. Insert the block relationship
	insertBlockQuery := `
		INSERT INTO user_relationships (actor_id, type, target_id) 
		VALUES ($1, $3, $2) 
		ON CONFLICT (actor_id, type, target_id) DO NOTHING`

	if _, err := tx.ExecContext(ctx, insertBlockQuery, actorID, targetID, RelTypeBlock); err != nil {
		return fmt.Errorf("failed to insert block: %w", err)
	}

	return tx.Commit()
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
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	var ids []uint64
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("row scan failed: %w", err)
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ids, nil
}
