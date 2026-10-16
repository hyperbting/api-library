package app

import (
	"api-library/internal/account"
	"api-library/internal/auth"
	"api-library/internal/config"
	"api-library/internal/gem"
	"api-library/internal/iap"
	"api-library/internal/item"
	"api-library/internal/news"
	"api-library/internal/pet"
	"api-library/internal/quest"
	"api-library/internal/redeem"
	"api-library/internal/save"
	"api-library/internal/user"
	"api-library/pkg/session"
	"context"
	"fmt"
	"log"
	"os"

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
	QuestHdl  *quest.Handler
	NewsHdl   *news.Handler
	SaveHdl   *save.Handler
	RedeemHdl *redeem.Handler
	GemHdl    *gem.Handler
	IapHdl    *iap.Handler
	PetHdl    *pet.Handler

	// Linked players (Play Games / Game Center), read from Firebase Auth for the admin panel
	AccountHdl *account.Handler

	// Creates Firebase Auth email/password users (the admin panel's sign-in accounts)
	EmailUserCreator auth.EmailUserCreator

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
	container.EmailUserCreator = auth.NewFirebaseEmailUserCreator(fbAuthClient)
	container.AccountHdl = account.NewHandler(account.NewService(account.FirebaseSource{Client: fbAuthClient}))

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
		container.SaveHdl = save.NewHandler(save.NewService(save.NewFirestoreRepository(container.Firestore)))
		// News: public items for everyone, dev items only for admin accounts (checked per request)
		container.NewsHdl = news.NewHandler(news.NewService(news.NewFirestoreRepository(container.Firestore)), auth.FirestoreAdminChecker(container.Firestore))
		// Item catalog (embedded, validated at boot like the quest table)
		items, err := item.LoadCatalog()
		if err != nil {
			return nil, err
		}
		container.RedeemHdl = redeem.NewHandler(redeem.NewService(redeem.NewFirestoreRepository(container.Firestore), items))
		// Gems (server-owned balance, spends, daily warp allowance) and store purchases that deliver them. The
		// Google Play verifier builds its credentials on the first purchase, so a missing Play API setup never
		// stops the boot.
		// Warps also need the store tutorial's third decoration in the player's quest progress (gem.WarpGate).
		container.GemHdl = gem.NewHandler(gem.NewService(gem.NewFirestoreRepository(container.Firestore), items,
			gem.WithWarpGate(gem.WarpGateFunc(quest.NewWarpGate(questRepo)))))
		playPackage := os.Getenv("PLAY_PACKAGE_NAME")
		if playPackage == "" {
			playPackage = "com.steadyshellshock.lettucemakemoney"
		}
		// The owned-pet list, rebuilt from the grants ledger (starter pets + Pet rewards).
		container.PetHdl = pet.NewHandler(pet.NewService(pet.NewFirestoreRepository(container.Firestore)))
		container.IapHdl = iap.NewHandler(iap.NewService(iap.NewFirestoreRepository(container.Firestore), items, iap.NewGooglePlayVerifier(playPackage)))
		// Email players: stateless access tokens only (no Redis), users in Firestore
		if appCfg.SessionKeyUnsafe() {
			// config.yaml's key is public: tokens signed with it are forgeable, so there is no email login.
			log.Printf("[BOOT] email login is OFF: session.secret_key is the public development key; set SESSION_SECRET_KEY (local runs: app.allow_dev_session_key)")
		} else if appCfg.SessionJWT != nil {
			tokenMgr, err := session.NewTokenManager(appCfg.SessionJWT)
			if err != nil {
				return nil, fmt.Errorf("session token manager init failed: %w", err)
			}
			userSrv := user.NewService(user.NewFirestoreRepository(container.Firestore), session.NewService(tokenMgr, nil))
			container.UserHdl = user.NewHandler(userSrv)
		}

		log.Printf("[BOOT] quest catalog version %d, %d quests", catalog.Version, len(catalog.Quests))
		log.Printf("[BOOT] item catalog version %d, %d items", items.Version, len(items.Items))
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
