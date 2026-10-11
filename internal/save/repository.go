package save

import (
	"context"

	"api-library/internal/grant"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Repository interface {
	// Get returns the user's save (nil when none) and grants document (always non-nil).
	Get(ctx context.Context, uid string) (*Doc, *grant.Doc, error)
	// Update runs fn in one transaction over both documents (new ones when missing). fn may run more than
	// once on contention. It returns whether to write the save and whether to write the grants.
	Update(ctx context.Context, uid string, fn func(doc *Doc, grants *grant.Doc) (writeSave, writeGrants bool, err error)) error
	// Delete removes the user's save document (no error when it does not exist). Grants stay.
	Delete(ctx context.Context, uid string) error
}

type firestoreRepo struct {
	client *firestore.Client
}

func NewFirestoreRepository(client *firestore.Client) Repository {
	return &firestoreRepo{client: client}
}

func (r *firestoreRepo) saveRef(uid string) *firestore.DocumentRef {
	return r.client.Collection(UsersCollection).Doc(uid).Collection(PrivateCollection).Doc(SaveDocID)
}

func (r *firestoreRepo) Get(ctx context.Context, uid string) (*Doc, *grant.Doc, error) {
	snaps, err := r.client.GetAll(ctx, []*firestore.DocumentRef{r.saveRef(uid), grant.DocRef(r.client, uid)})
	if err != nil {
		return nil, nil, err
	}
	doc, err := decode(snaps[0])
	if err != nil {
		return nil, nil, err
	}
	grants, err := grant.Decode(snaps[1])
	if err != nil {
		return nil, nil, err
	}
	return doc, grants, nil
}

func (r *firestoreRepo) Update(ctx context.Context, uid string, fn func(doc *Doc, grants *grant.Doc) (bool, bool, error)) error {
	saveRef := r.saveRef(uid)
	grantsRef := grant.DocRef(r.client, uid)
	return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		saveSnap, err := tx.Get(saveRef)
		if err != nil && status.Code(err) != codes.NotFound {
			return err
		}
		grantsSnap, err := tx.Get(grantsRef)
		if err != nil && status.Code(err) != codes.NotFound {
			return err
		}

		doc, err := decode(saveSnap)
		if err != nil {
			return err
		}
		if doc == nil {
			doc = &Doc{SchemaVersion: SchemaVersion}
		}
		grants, err := grant.Decode(grantsSnap)
		if err != nil {
			return err
		}

		writeSave, writeGrants, err := fn(doc, grants)
		if err != nil {
			return err
		}
		if writeSave {
			if err := tx.Set(saveRef, doc); err != nil {
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
	_, err := r.saveRef(uid).Delete(ctx)
	return err
}

// decode returns nil for a missing document.
func decode(snap *firestore.DocumentSnapshot) (*Doc, error) {
	if snap == nil || !snap.Exists() {
		return nil, nil
	}
	var doc Doc
	if err := snap.DataTo(&doc); err != nil {
		return nil, err
	}
	if doc.Summary.Venues == nil {
		doc.Summary.Venues = map[string]int{}
	}
	return &doc, nil
}
