package friend

import (
	"context"
	"database/sql"
)

type PGRelationshipRepository struct {
	db *sql.DB
}

func NewPGRelationshipRepository(db *sql.DB) *PGRelationshipRepository {
	return &PGRelationshipRepository{db: db}
}

// Fetch who I follow or block (Uses Index-Only Scan on Primary Key)
func (r *PGRelationshipRepository) GetOutboundTargets(ctx context.Context, actorID string, relType RelationshipType) ([]string, error) {
	query := `
		SELECT target_id 
		FROM user_relationships 
		WHERE actor_id = $1 AND type = $2`

	rows, err := r.db.QueryContext(ctx, query, actorID, relType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targetIDs []string
	for rows.Next() {
		var targetID string
		if err := rows.Scan(&targetID); err != nil {
			return nil, err
		}
		targetIDs = append(targetIDs, targetID)
	}
	return targetIDs, rows.Err()
}

// Fetch who follows me (Uses idx_target_type_actor Index-Only Scan)
func (r *PGRelationshipRepository) GetInboundActors(ctx context.Context, targetID string, relType RelationshipType) ([]string, error) {
	query := `
		SELECT actor_id 
		FROM user_relationships 
		WHERE target_id = $1 AND type = $2`

	rows, err := r.db.QueryContext(ctx, query, targetID, relType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actorIDs []string
	for rows.Next() {
		var actorID string
		if err := rows.Scan(&actorID); err != nil {
			return nil, err
		}
		actorIDs = append(actorIDs, actorID)
	}
	return actorIDs, rows.Err()
}

// Insert single relationship record in Postgres
func (r *PGRelationshipRepository) AddRelationship(ctx context.Context, actorID, targetID string, relType RelationshipType) error {
	query := `
		INSERT INTO user_relationships (actor_id, target_id, type)
		VALUES ($1, $2, $3)
		ON CONFLICT (actor_id, type, target_id) 
		DO UPDATE SET created_at = CURRENT_TIMESTAMP`

	_, err := r.db.ExecContext(ctx, query, actorID, targetID, relType)
	return err
}
