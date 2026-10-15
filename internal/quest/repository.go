package quest

import (
	"context"

	"api-library/internal/grant"

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
	// UpdateWithGrants is Update over the quest and the grants document in one transaction, for a step that
	// pays Gem rewards: the step and its grant are stored together or not at all. fn says which of the two
	// documents to write.
	UpdateWithGrants(ctx context.Context, uid string, fn func(doc *QuestsDoc, grants *grant.Doc) (writeQuests, writeGrants bool, err error)) error
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

func (r *firestoreRepo) UpdateWithGrants(ctx context.Context, uid string, fn func(*QuestsDoc, *grant.Doc) (bool, bool, error)) error {
	ref := r.ref(uid)
	grantsRef := grant.DocRef(r.client, uid)
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

		grantsSnap, err := tx.Get(grantsRef)
		if err != nil && status.Code(err) != codes.NotFound {
			return err
		}
		grants, err := grant.Decode(grantsSnap)
		if err != nil {
			return err
		}

		writeQuests, writeGrants, err := fn(doc, grants)
		if err != nil {
			return err
		}
		if writeQuests {
			if err := tx.Set(ref, doc); err != nil {
				return err
			}
		}
		if writeGrants {
			return tx.Set(grantsRef, grants)
		}
		return nil
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
