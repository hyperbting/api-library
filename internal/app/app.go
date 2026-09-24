package app

import (
	"api-library/internal/friend"
	"api-library/internal/presence"
	"api-library/internal/purchase"
	"api-library/internal/user"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Container 僅持有核心數據與業務邏輯實例
type Container struct {
	Cache      *redis.Client
	DB         *gorm.DB
	DBReadOnly *gorm.DB
	ESClient   *elasticsearch.TypedClient

	UserSrv   user.Service
	FriendSrv friend.Service

	FriendRepo   friend.RelationshipRepository
	PresenceRepo presence.Repository
	PurchaseRepo purchase.PurchaseRepository
}

func (c *Container) Close() {
	if c.DB != nil {
		sqlDB, err := c.DB.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}

	if c.DBReadOnly != nil {
		sqlDB, err := c.DBReadOnly.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}

func NewContainer(cache *redis.Client, gDb *gorm.DB, gDbRo *gorm.DB, es *elasticsearch.TypedClient) *Container {
	// Repositories
	// 	attestationRepo := repository.NewAttestationRepository(es)

	// Services
	// 	attestationSvc := service.NewAttestationService(attestationRepo)

	return &Container{
		Cache:      cache,
		DB:         gDb,
		DBReadOnly: gDbRo,
		ESClient:   es,
		// AttestationSvc: attestationSvc,
	}
}
