package redeem

import (
	"context"

	"api-library/internal/grant"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Repository interface {
	// Redeem runs fn in one transaction over the code (nil when it does not exist, or when code is "")
	// and the user's grants document (new when missing). fn may run more than once on contention.
	Redeem(ctx context.Context, uid, code string, fn func(c *Code, grants *grant.Doc) (writeCode, writeGrants bool, err error)) error
	// Create stores a new code; ErrCodeExists when the name is taken.
	Create(ctx context.Context, c *Code) error
	// SetDisabled flips a code on or off; ErrCodeNotFound when it does not exist.
	SetDisabled(ctx context.Context, code string, disabled bool) error
	List(ctx context.Context) ([]Code, error)
}

type firestoreRepo struct {
	client *firestore.Client
}

func NewFirestoreRepository(client *firestore.Client) Repository {
	return &firestoreRepo{client: client}
}

func (r *firestoreRepo) codeRef(code string) *firestore.DocumentRef {
	return r.client.Collection(CodesCollection).Doc(code)
}

func (r *firestoreRepo) Redeem(ctx context.Context, uid, code string, fn func(*Code, *grant.Doc) (bool, bool, error)) error {
	grantsRef := grant.DocRef(r.client, uid)
	return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		var c *Code
		var codeRef *firestore.DocumentRef
		if code != "" {
			codeRef = r.codeRef(code)
			snap, err := tx.Get(codeRef)
			switch {
			case status.Code(err) == codes.NotFound:
			case err != nil:
				return err
			default:
				var doc Code
				if err := snap.DataTo(&doc); err != nil {
					return err
				}
				c = &doc
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

		writeCode, writeGrants, err := fn(c, grants)
		if err != nil {
			return err
		}
		if writeCode && c != nil {
			if err := tx.Set(codeRef, c); err != nil {
				return err
			}
		}
		if writeGrants {
			return tx.Set(grantsRef, grants)
		}
		return nil
	})
}

func (r *firestoreRepo) Create(ctx context.Context, c *Code) error {
	_, err := r.codeRef(c.Code).Create(ctx, c)
	if status.Code(err) == codes.AlreadyExists {
		return ErrCodeExists
	}
	return err
}

func (r *firestoreRepo) SetDisabled(ctx context.Context, code string, disabled bool) error {
	_, err := r.codeRef(code).Update(ctx, []firestore.Update{{Path: "disabled", Value: disabled}})
	if status.Code(err) == codes.NotFound {
		return ErrCodeNotFound
	}
	return err
}

func (r *firestoreRepo) List(ctx context.Context) ([]Code, error) {
	it := r.client.Collection(CodesCollection).OrderBy("createdAt", firestore.Desc).Limit(500).Documents(ctx)
	defer it.Stop()

	out := []Code{}
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		var c Code
		if err := snap.DataTo(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
}
