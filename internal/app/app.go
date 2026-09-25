package app

import (
	"api-library/internal/auth"
	"api-library/internal/config"
	"api-library/internal/friend"
	"api-library/internal/infrastructure"
	"api-library/internal/presence"
	"api-library/internal/purchase"
	"api-library/internal/user"
	"api-library/pkg/platform_helper/meta"
	"api-library/pkg/session"
	"database/sql"
	"fmt"
	"log"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/gofiber/fiber/v3"
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

	//Handlers
	MetaVerifyHdl fiber.Handler

	// Middlewares
	AuthMw        fiber.Handler
	MetaPaymentWH fiber.Handler

	// Services
	SessionSrv session.Service
	UserSrv    user.Service
	FriendSrv  friend.Service

	// Repositories
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

func NewContainer(appCfg *config.AppConfig) (*Container, error) {
	// init infrastructures
	cache, err := infrastructure.NewRedis(appCfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("Cache init failed: %v", err)
	}

	db, err := infrastructure.NewPostgreSQL(&appCfg.Database.MasterDB)
	if err != nil {
		return nil, fmt.Errorf("Master DB init failed: %v", err)
	}

	dbRo, err := infrastructure.NewPostgreSQL(&appCfg.Database.ReadOnlyDB)
	if err != nil {
		return nil, fmt.Errorf("RO DB init failed: %v", err)
	}

	esClient, err := infrastructure.NewElasticsearch(appCfg.Elastic)
	if err != nil {
		return nil, fmt.Errorf("Cronjob ES init failed: %v", err)
	}

	tknManager, err := session.NewTokenManager(appCfg.SessionJWT)
	if err != nil {
		return nil, fmt.Errorf("Session Manager init failed: %v", err)
	}

	// Repositories
	sessionRepo := session.NewSessionRepository(cache)
	friendRepo := friend.NewRepository(db)
	presenceRepo := presence.NewRepository(cache)
	purchaseRepo := purchase.NewRepository(db)
	userRepo := user.NewRepository(dbRo, db)

	// Services
	sessionSrv := session.NewService(tknManager, sessionRepo)
	friendSrv := friend.NewService(friendRepo)
	// purchaseSrv := purchase.NewService(db)
	// presenceSrv := presence.NewService(presenceRepo)
	userSrv := user.NewService(userRepo, sessionSrv)

	//Middleware
	authMw := auth.AuthMiddleware(tknManager)
	//metaPaymentWH := meta.NewMetaPaymentWebhookMiddleware(appCfg.Webhook.MetaPayment, purchaseSrv)

	//Handler
	metaVerifyHdl := meta.NewMetaWebhookVerifyHandler(appCfg.Webhook.MetaPayment.VerifyToken)

	return &Container{
		TokenMgr:   tknManager,
		Cache:      cache,
		DB:         db,
		DBReadOnly: dbRo,
		ESClient:   esClient,

		SessionRepo:  sessionRepo,
		FriendRepo:   friendRepo,
		PresenceRepo: presenceRepo,
		PurchaseRepo: purchaseRepo,

		SessionSrv: sessionSrv,
		FriendSrv:  friendSrv,
		// PresenceSrv: presenceSrv,
		// PurchaseSrv: purchaseSrv,
		UserSrv: userSrv,

		AuthMw: authMw,
		//MetaPaymentWH: metaPaymentWH,

		MetaVerifyHdl: metaVerifyHdl,
	}, nil
}
