package app

import (
	"api-library/internal/friend"
	"api-library/internal/presence"
	"api-library/internal/purchase"
	"api-library/internal/user"
	"api-library/pkg/session"
	"database/sql"
	"log"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// Container 僅持有核心數據與業務邏輯實例
type Container struct {
	Cache      *redis.Client
	DB         *gorm.DB
	DBReadOnly *gorm.DB
	ESClient   *elasticsearch.TypedClient
	TokenMgr   session.TokenManager

	SessionSrv session.Service
	UserSrv    user.Service
	FriendSrv  friend.Service

	SessionRepo  session.SessionRepository
	FriendRepo   friend.RelationshipRepository
	PresenceRepo presence.Repository
	PurchaseRepo purchase.PurchaseRepository
}

func (c *Container) Close() {
	var masterSQLDB *sql.DB

	if c.DB != nil {
		if sqlDB, err := c.DB.DB(); err == nil {
			masterSQLDB = sqlDB
			if err := sqlDB.Close(); err != nil {
				log.Printf("failed to close master db: %v", err)
			}
		}
	}

	if c.DBReadOnly != nil {
		if sqlDB, err := c.DBReadOnly.DB(); err == nil {
			if sqlDB != masterSQLDB {
				if err := sqlDB.Close(); err != nil {
					log.Printf("failed to close readonly db: %v", err)
				}
			}
		}
	}

	if c.Cache != nil {
		if err := c.Cache.Close(); err != nil {
			log.Printf("failed to close redis: %v", err)
		}
	}

	// if c.ESClient != nil {
	// 	if err := c.ESClient.Close(context.Background()); err != nil {
	// 		log.Printf("failed to close elasticsearch: %v", err)
	// 	}
	// }
}

func NewContainer(cache *redis.Client, gDb *gorm.DB, gDbRo *gorm.DB, es *elasticsearch.TypedClient, tm session.TokenManager) *Container {
	// Repositories
	sessionRepo := session.NewSessionRepository(cache)
	friendRepo := friend.NewRepository(gDb)
	presenceRepo := presence.NewRepository(cache)
	purchaseRepo := purchase.NewRepository(gDb)
	userRepo := user.NewRepository(gDbRo, gDb)

	// Services
	sessionSrv := session.NewService(tm, sessionRepo)
	friendSrv := friend.NewService(friendRepo)
	// purchaseSrv := purchase.NewService(gDb)
	// presenceSrv := presence.NewService(presenceRepo)
	userSrv := user.NewService(userRepo, sessionSrv)

	return &Container{
		TokenMgr:   tm,
		Cache:      cache,
		DB:         gDb,
		DBReadOnly: gDbRo,
		ESClient:   es,

		SessionRepo:  sessionRepo,
		FriendRepo:   friendRepo,
		PresenceRepo: presenceRepo,
		PurchaseRepo: purchaseRepo,

		SessionSrv: sessionSrv,
		FriendSrv:  friendSrv,
		// PresenceSrv: presenceSrv,
		// PurchaseSrv: purchaseSrv,
		UserSrv: userSrv,
	}
}
