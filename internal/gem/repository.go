package gem

import (
	"context"
	"time"

	"api-library/internal/grant"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DocRef is the user's gems document.
func DocRef(client *firestore.Client, uid string) *firestore.DocumentRef {
	return client.Collection(grant.UsersCollection).Doc(uid).Collection(grant.PrivateCollection).Doc(GemsDocID)
}

type Repository interface {
	// Run runs fn in one transaction over the user's gems and grants documents (new ones when missing). fn may
	// run more than once on contention. Only the documents fn asks for are written.
	Run(ctx context.Context, uid string, fn func(gems *Doc, grants *grant.Doc) (writeGems, writeGrants bool, err error)) error
}

type firestoreRepo struct {
	client *firestore.Client
}

func NewFirestoreRepository(client *firestore.Client) Repository {
	return &firestoreRepo{client: client}
}

func (r *firestoreRepo) Run(ctx context.Context, uid string, fn func(*Doc, *grant.Doc) (bool, bool, error)) error {
	gemsRef := DocRef(r.client, uid)
	grantsRef := grant.DocRef(r.client, uid)
	return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		gems := NewDoc()
		snap, err := tx.Get(gemsRef)
		switch {
		case status.Code(err) == codes.NotFound:
		case err != nil:
			return err
		default:
			if err := snap.DataTo(gems); err != nil {
				return err
			}
			if gems.Credited == nil {
				gems.Credited = map[string]time.Time{}
			}
			if gems.Spends == nil {
				gems.Spends = map[string]*SpendRecord{}
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

		writeGems, writeGrants, err := fn(gems, grants)
		if err != nil {
			return err
		}
		if writeGems {
			if err := tx.Set(gemsRef, gems); err != nil {
				return err
			}
		}
		if writeGrants {
			return tx.Set(grantsRef, grants)
		}
		return nil
	})
}
