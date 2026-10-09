package app

import (
	"api-library/internal/auth"
	"api-library/internal/config"
	"api-library/internal/quest"
	"api-library/internal/user"
	"api-library/pkg/session"
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/gofiber/fiber/v3"
)

type FirebaseContainer struct {
	FbApp           *firebase.App
	StorageBucket   *storage.BucketHandle
	Firestore       *firestore.Client
	MessagingClient *messaging.Client

	// Verifies Firebase ID tokens (the game client always sends one, whatever app.auth is)
	FirebaseAuthMw fiber.Handler

	// Game services (nil when Firestore is disabled)
	QuestHdl *quest.Handler

	// Email/password login and register, stored in Firestore (nil when Firestore or the session config is missing)
	UserHdl *user.Handler
	//FirebaseAuth    *auth.Client
	//StorageClient *storage.Client
}

// NewFirebaseContainer returns (nil, nil) when Firebase is disabled; callers must nil-check the result
func NewFirebaseContainer(ctx context.Context, appCfg *config.AppConfig) (*FirebaseContainer, error) {
	if !appCfg.FirebaseEnabled() {
		return nil, nil
	}

	var err error
	fbApp, err := firebase.NewApp(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("Firebase init failed: %w", err)
	}

	container := &FirebaseContainer{
		FbApp: fbApp,
	}

	fbAuthClient, err := fbApp.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to init firebase auth: %w", err)
	}
	authProvider, err := auth.NewFirebaseAuthProvider(fbAuthClient)
	if err != nil {
		return nil, fmt.Errorf("failed to init firebase auth provider: %w", err)
	}
	container.FirebaseAuthMw = authProvider.Middleware()

	// Firestore Database Service
	if appCfg.Firebase.UseFirestore {
		container.Firestore, err = fbApp.Firestore(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to init firebase firestore: %w", err)
		}

		// Quest catalog is embedded at build time; an invalid one fails the boot
		catalog, err := quest.LoadCatalog()
		if err != nil {
			return nil, err
		}
		questRepo := quest.NewFirestoreRepository(container.Firestore, catalog.Version)
		container.QuestHdl = quest.NewHandler(quest.NewService(questRepo, catalog))
		// Email players: stateless access tokens only (no Redis), users in Firestore
		if appCfg.SessionJWT != nil {
			tokenMgr, err := session.NewTokenManager(appCfg.SessionJWT)
			if err != nil {
				return nil, fmt.Errorf("session token manager init failed: %w", err)
			}
			userSrv := user.NewService(user.NewFirestoreRepository(container.Firestore), session.NewService(tokenMgr, nil))
			container.UserHdl = user.NewHandler(userSrv)
		}

		log.Printf("[BOOT] quest catalog version %d, %d quests", catalog.Version, len(catalog.Quests))
	}

	// Cloud Storage Service
	if appCfg.Firebase.UseStorage {
		fbStorageClient, err := fbApp.Storage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to init firebase storage: %w", err)
		}

		// 直接取得指定 Bucket 的 Handle（回傳型態即為 *storage.BucketHandle）
		container.StorageBucket, err = fbStorageClient.Bucket(appCfg.Firebase.StorageBucket)
		if err != nil {
			return nil, fmt.Errorf("failed to get bucket handle: %w", err)
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
			return nil, fmt.Errorf("failed to init firebase messaging: %w", err)
		}
	}

	return container, nil
}

func (c *FirebaseContainer) Close() {
	if c == nil {
		return
	}

	if c.Firestore != nil {
		if err := c.Firestore.Close(); err != nil {
			log.Printf("failed to close firestore: %v", err)
		}
	}
}
