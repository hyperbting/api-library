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
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
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

	// Firebase OPTIONAL Service (Nil if not Initidalize)
	//FirebaseAuth    *auth.Client
	StorageBucket *storage.BucketHandle
	//StorageClient *storage.Client
	Firestore       *firestore.Client
	MessagingClient *messaging.Client

	//Handlers
	MetaVerifyHdl fiber.Handler

	// Middlewares
	AuthMw        fiber.Handler
	MetaPaymentMw fiber.Handler

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

func NewContainer(appCfg *config.AppConfig) (ac *Container, err error) {

	// init infrastructures
	cache, err := infrastructure.NewRedis(appCfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("Cache init failed: %v", err)
	}

	db, err := infrastructure.NewPostgreSQL(&appCfg.Database.MasterDB)
	if err != nil {
		return nil, fmt.Errorf("Master DB init failed: %w", err)
	}

	dbRo, err := infrastructure.NewPostgreSQL(&appCfg.Database.ReadOnlyDB)
	if err != nil {
		return nil, fmt.Errorf("RO DB init failed: %w", err)
	}

	esClient, err := infrastructure.NewElasticsearch(appCfg.Elastic)
	if err != nil {
		return nil, fmt.Errorf("Cronjob ES init failed: %w", err)
	}

	tknManager, err := session.NewTokenManager(appCfg.SessionJWT)
	if err != nil {
		return nil, fmt.Errorf("Session Manager init failed: %w", err)
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
	//metaPaymentMw := meta.NewMetaPaymentWebhookMiddleware(appCfg.Webhook.MetaPayment, purchaseSrv)

	//Handler
	metaVerifyHdl := meta.NewMetaWebhookVerifyHandler(appCfg.Webhook.MetaPayment.VerifyToken)

	ac = &Container{
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

		//MetaPaymentMw: metaPaymentMw,

		MetaVerifyHdl: metaVerifyHdl,
	}

	ctx := context.Background()

	var fbApp *firebase.App
	if appCfg.Firebase != nil && appCfg.Firebase.Enabled {
		fbApp, err = firebase.NewApp(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("Firebase init failed: %w", err)
		}

		err = OptionalFirebase(ctx, appCfg, fbApp, ac)
		if err != nil {
			return nil, err
		}
	}

	var ap auth.AuthProvider
	switch appCfg.App.AuthMode {
	case "firebase":
		if fbApp == nil {
			return nil, errors.New("auth mode is set to 'firebase', but Firebase is not enabled or initialized")
		}
		fbAuthClient, err := fbApp.Auth(ctx)
		if err != nil {
			return nil, fmt.Errorf("Firebase Auth init failed: %w", err)
		}
		ap, err = auth.NewFirebaseAuthProvider(fbAuthClient)
		if err != nil {
			return nil, fmt.Errorf("FirebaseAuthProvider init failed: %w", err)
		}
	case "custom":
		fallthrough
	default:
		ap = auth.NewCustomSessionProvider(tknManager)
	}
	ac.AuthMw = ap.Middleware()

	return ac, nil
}

func OptionalFirebase(ctx context.Context, appCfg *config.AppConfig, fbApp *firebase.App, container *Container) (err error) {
	if appCfg.Firebase == nil || !appCfg.Firebase.Enabled || fbApp == nil {
		return nil
	}

	// Firestore Database Service
	if appCfg.Firebase.UseFirestore {
		container.Firestore, err = fbApp.Firestore(ctx)
		if err != nil {
			return fmt.Errorf("failed to init firebase firestore: %w", err)
		}
	}

	// Cloud Storage Service
	if appCfg.Firebase.UseStorage {
		fbStorageClient, err := fbApp.Storage(ctx)
		if err != nil {
			return fmt.Errorf("failed to init firebase storage: %w", err)
		}

		// 直接取得指定 Bucket 的 Handle（回傳型態即為 *storage.BucketHandle）
		container.StorageBucket, err = fbStorageClient.Bucket(appCfg.Firebase.StorageBucket)
		if err != nil {
			return fmt.Errorf("failed to get bucket handle: %w", err)
		}

		// var storageClient *storage.Client
		// // 1. 取得 Firebase Storage Wrapper Client
		// fbStorageClient, err := fbApp.Storage(ctx)
		// if err != nil {
		// 	return nil, fmt.Errorf("failed to init firebase storage: %w", err)
		// }

		// // 2. 呼叫 .Client(ctx) 取得原生的 *storage.Client
		// storageClient, err = fbStorageClient.Client(ctx)
		// if err != nil {
		// 	return nil, fmt.Errorf("failed to get native gcs client: %w", err)
		// }

		// _ = storageClient // 成功取得 *storage.Client
	}

	// Firebase Cloud Messaging (FCM) Service
	if appCfg.Firebase.UseMessage {
		container.MessagingClient, err = fbApp.Messaging(ctx)
		if err != nil {
			return fmt.Errorf("failed to init firebase messaging: %w", err)
		}
	}

	return nil
}
