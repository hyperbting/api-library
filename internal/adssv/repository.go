package adssv

import (
	"context"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Collection is server-only: the security rules deny every client access to it (the catch-all rule).
const Collection = "adSsv"

type firestoreRepo struct {
	client *firestore.Client
}

func NewFirestoreRepository(client *firestore.Client) Repository {
	return &firestoreRepo{client: client}
}

func (r *firestoreRepo) Save(ctx context.Context, rec Record) error {
	_, err := r.client.Collection(Collection).Doc(rec.TransactionID).Create(ctx, rec)
	if status.Code(err) == codes.AlreadyExists {
		return nil
	}
	return err
}

// Has uses two equality filters, which Firestore answers without a composite index.
func (r *firestoreRepo) Has(ctx context.Context, uid, requestID string) (bool, error) {
	it := r.client.Collection(Collection).
		Where("uid", "==", uid).
		Where("requestId", "==", requestID).
		Limit(1).Documents(ctx)
	defer it.Stop()
	_, err := it.Next()
	if err == iterator.Done {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
