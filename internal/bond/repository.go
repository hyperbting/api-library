package bond

import (
	"context"

	"api-library/internal/grant"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Repository interface {
	// Get returns the user's bond document, nil when there is none.
	Get(ctx context.Context, uid string) (*Doc, error)
	// Grants reads the user's grants ledger with a plain read (a fresh Doc when missing). The ledger only grows, so a
	// read outside the bond transaction can at worst miss a pet granted a moment ago.
	Grants(ctx context.Context, uid string) (*grant.Doc, error)
	// Update runs fn in one transaction over the bond document only; fn gets a new document (CreatedAt = now) when
	// none exists. fn may run more than once on contention. The document is written when fn returns write = true.
	Update(ctx context.Context, uid string, fresh func() Doc, fn func(doc *Doc) (write bool, err error)) error
}

type firestoreRepo struct{ client *firestore.Client }

func NewFirestoreRepository(client *firestore.Client) Repository {
	return &firestoreRepo{client: client}
}

func (r *firestoreRepo) ref(uid string) *firestore.DocumentRef {
	return r.client.Collection(UsersCollection).Doc(uid).Collection(PrivateCollection).Doc(BondDocID)
}

func (r *firestoreRepo) Get(ctx context.Context, uid string) (*Doc, error) {
	snap, err := r.ref(uid).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decode(snap)
}

func (r *firestoreRepo) Grants(ctx context.Context, uid string) (*grant.Doc, error) {
	snap, err := grant.DocRef(r.client, uid).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return grant.NewDoc(), nil
	}
	if err != nil {
		return nil, err
	}
	return grant.Decode(snap)
}

func (r *firestoreRepo) Update(ctx context.Context, uid string, fresh func() Doc, fn func(doc *Doc) (bool, error)) error {
	ref := r.ref(uid)
	return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil && status.Code(err) != codes.NotFound {
			return err
		}
		doc, err := decode(snap)
		if err != nil {
			return err
		}
		if doc == nil {
			d := fresh()
			doc = &d
		}
		write, err := fn(doc)
		if err != nil || !write {
			return err
		}
		return tx.Set(ref, doc)
	})
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
	if doc.Pets == nil {
		doc.Pets = map[string]*PetBond{}
	}
	return &doc, nil
}
