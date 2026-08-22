package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisKeyPrefixes defines constants for structuring Redis keys
const (
	RedisKeyPrefixes  = "auth:"
	RedisSessionKey   = RedisKeyPrefixes + "session:"   // Key pattern for active sessions
	RedisDeviceKey    = RedisKeyPrefixes + "device:"    // Key pattern for tracking devices per user
	RedisBlacklistKey = RedisKeyPrefixes + "blacklist:" // Key pattern for blacklisted tokens
)

// Key Formatter Helpers
func fmtSessionKey(userID string) string {
	return fmt.Sprintf("%s%s", RedisSessionKey, userID) // Produces "auth:session:player_12345"
}

func fmtDeviceKey(userID string) string {
	return fmt.Sprintf("%s%s", RedisDeviceKey, userID) // Produces "auth:device:player_12345"
}

func fmtBlacklistKey(jti string) string {
	return fmt.Sprintf("%s%s", RedisBlacklistKey, jti) // Produces "auth:blacklist:rt_uuid_998877"
}

type SessionRepository interface {
	Save(ctx context.Context, session *Session, ttl time.Duration) error
	Get(ctx context.Context, userID string) (*Session, error)
	Delete(ctx context.Context, userID string) error
}

type sessionRepoImpl struct {
	rdb *redis.Client
}

func NewSessionRepository(rdb *redis.Client) SessionRepository {
	return &sessionRepoImpl{
		rdb: rdb,
	}
}

func (r *sessionRepoImpl) Save(ctx context.Context, session *Session, ttl time.Duration) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	key := fmtSessionKey(session.UserID)
	return r.rdb.Set(ctx, key, data, ttl).Err()
}

func (r *sessionRepoImpl) Get(ctx context.Context, userID string) (*Session, error) {
	key := fmtSessionKey(userID)
	val, err := r.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var sess Session
	if err := json.Unmarshal([]byte(val), &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

func (r *sessionRepoImpl) Delete(ctx context.Context, userID string) error {
	key := fmtSessionKey(userID)
	return r.rdb.Del(ctx, key).Err()
}
