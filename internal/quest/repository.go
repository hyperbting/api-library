package quest

import (
	"context"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Repository interface {
	// Get returns the user's quest document, or nil when the user has none yet.
	Get(ctx context.Context, uid string) (*QuestsDoc, error)
	// Update runs fn in a transaction on the user's document (a new one when missing).
	// fn may run more than once on contention; it is written only when fn returns true.
	Update(ctx context.Context, uid string, fn func(doc *QuestsDoc) (bool, error)) error
	// Delete removes the user's quest document (no error when it does not exist).
	Delete(ctx context.Context, uid string) error
}

type firestoreRepo struct {
	client         *firestore.Client
	catalogVersion int
}

func NewFirestoreRepository(client *firestore.Client, catalogVersion int) Repository {
	return &firestoreRepo{client: client, catalogVersion: catalogVersion}
}

func (r *firestoreRepo) ref(uid string) *firestore.DocumentRef {
	return r.client.Collection(UsersCollection).Doc(uid).Collection(PrivateCollection).Doc(QuestsDocID)
}

func (r *firestoreRepo) Get(ctx context.Context, uid string) (*QuestsDoc, error) {
	snap, err := r.ref(uid).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decode(snap)
}

func (r *firestoreRepo) Update(ctx context.Context, uid string, fn func(doc *QuestsDoc) (bool, error)) error {
	ref := r.ref(uid)
	return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc := NewQuestsDoc(r.catalogVersion)
		snap, err := tx.Get(ref)
		switch {
		case status.Code(err) == codes.NotFound:
		case err != nil:
			return err
		default:
			if doc, err = decode(snap); err != nil {
				return err
			}
		}

		write, err := fn(doc)
		if err != nil || !write {
			return err
		}
		return tx.Set(ref, doc)
	})
}

func (r *firestoreRepo) Delete(ctx context.Context, uid string) error {
	_, err := r.ref(uid).Delete(ctx)
	return err
}

func decode(snap *firestore.DocumentSnapshot) (*QuestsDoc, error) {
	var doc QuestsDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, err
	}
	if doc.Quests == nil {
		doc.Quests = map[string]*Progress{}
	}
	return &doc, nil
}
