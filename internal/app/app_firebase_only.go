package app

import (
	"api-library/internal/config"
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
)

type FirebasseContainer struct {
	FbApp           *firebase.App
	StorageBucket   *storage.BucketHandle
	Firestore       *firestore.Client
	MessagingClient *messaging.Client
	//FirebaseAuth    *auth.Client
	//StorageClient *storage.Client
}

func NewFirebaseContainer(ctx context.Context, appCfg *config.AppConfig) (*FirebasseContainer, error) {
	if appCfg.Firebase == nil || !appCfg.Firebase.Enabled {
		return nil, nil
	}

	var err error
	fbApp, err := firebase.NewApp(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("Firebase init failed: %w", err)
	}

	container := &FirebasseContainer{}
	// Firestore Database Service
	if appCfg.Firebase.UseFirestore {
		container.Firestore, err = fbApp.Firestore(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to init firebase firestore: %w", err)
		}
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

func (c *FirebasseContainer) Close() {
	// nothing
}
