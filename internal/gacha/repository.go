package gacha

import (
	"context"
	"time"

	"api-library/internal/gem"
	"api-library/internal/grant"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DocRef is the user's gacha document.
func DocRef(client *firestore.Client, uid string) *firestore.DocumentRef {
	return client.Collection(grant.UsersCollection).Doc(uid).Collection(grant.PrivateCollection).Doc(DocID)
}

type Repository interface {
	// Run runs fn in one transaction over the user's gacha, gems and grants documents (new ones when missing), so a
	// gem spend and the roll it pays for commit together. fn may run more than once on contention. Only the
	// documents fn asks for are written.
	Run(ctx context.Context, uid string, fn func(d *Doc, gems *gem.Doc, grants *grant.Doc) (writeGacha, writeGems, writeGrants bool, err error)) error
}

type firestoreRepo struct {
	client *firestore.Client
}

func NewFirestoreRepository(client *firestore.Client) Repository {
	return &firestoreRepo{client: client}
}

func (r *firestoreRepo) Run(ctx context.Context, uid string, fn func(*Doc, *gem.Doc, *grant.Doc) (bool, bool, bool, error)) error {
	gachaRef := DocRef(r.client, uid)
	gemsRef := gem.DocRef(r.client, uid)
	grantsRef := grant.DocRef(r.client, uid)
	return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		d := NewDoc()
		snap, err := tx.Get(gachaRef)
		switch {
		case status.Code(err) == codes.NotFound:
		case err != nil:
			return err
		default:
			if err := snap.DataTo(d); err != nil {
				return err
			}
			d.Normalize()
		}

		gems := gem.NewDoc()
		gemsSnap, err := tx.Get(gemsRef)
		switch {
		case status.Code(err) == codes.NotFound:
		case err != nil:
			return err
		default:
			if err := gemsSnap.DataTo(gems); err != nil {
				return err
			}
			if gems.Credited == nil {
				gems.Credited = map[string]time.Time{}
			}
			if gems.Spends == nil {
				gems.Spends = map[string]*gem.SpendRecord{}
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

		writeGacha, writeGems, writeGrants, err := fn(d, gems, grants)
		if err != nil {
			return err
		}
		if writeGacha {
			if err := tx.Set(gachaRef, d); err != nil {
				return err
			}
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
