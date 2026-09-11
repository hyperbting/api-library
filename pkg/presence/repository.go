package presence

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	redisPrefix        = "presence:"
	defaultPresenceTTL = 30 * time.Second
)

// Repository Interface
type Repository interface {
	SavePresence(ctx context.Context, playerUUID string, presence *HeartbeatPayload) error
	GetPresence(ctx context.Context, playerUUID string) (*HeartbeatPayload, error)
	DeletePresence(ctx context.Context, playerUUID string) error
}

func NewRepository(rdb *redis.Client) Repository {
	return &repositoryImpl{
		rdb: rdb,
	}
}

type repositoryImpl struct {
	rdb *redis.Client
}

// Helper function to format keys
func (r *repositoryImpl) fmtPresenceKey(playerUUID string) string {
	return fmt.Sprintf("%s%s", redisPrefix, playerUUID)
}

func (r *repositoryImpl) SavePresence(ctx context.Context, playerUUID string, presence *HeartbeatPayload) error {
	data, err := json.Marshal(presence)
	if err != nil {
		return err
	}
	key := r.fmtPresenceKey(playerUUID)
	return r.rdb.Set(ctx, key, data, defaultPresenceTTL).Err()
}

func (r *repositoryImpl) GetPresence(ctx context.Context, playerUUID string) (*HeartbeatPayload, error) {
	key := r.fmtPresenceKey(playerUUID)
	val, err := r.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil // Expired / Not found
	} else if err != nil {
		return nil, err
	}

	var presence HeartbeatPayload
	if err := json.Unmarshal([]byte(val), &presence); err != nil {
		return nil, err
	}
	return &presence, nil
}

func (r *repositoryImpl) DeletePresence(ctx context.Context, playerUUID string) error {
	key := r.fmtPresenceKey(playerUUID)
	return r.rdb.Del(ctx, key).Err()
}
