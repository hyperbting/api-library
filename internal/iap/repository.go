package iap

import (
	"context"
	"errors"

	"google.golang.org/api/iterator"

	"api-library/internal/grant"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrOrderUsed: the store order was already delivered to another account.
var ErrOrderUsed = errors.New("order delivered to another account")

type Repository interface {
	// Deliver runs fn in one transaction over the user's grants document and the order document. fn reports
	// whether it changed the grants; the order is written together with them (it is created, never replaced).
	// ErrOrderUsed when the order was delivered to another account.
	Deliver(ctx context.Context, uid, orderID string, fn func(grants *grant.Doc) (write bool, order *Order, err error)) error
	// Orders returns the newest delivered orders, newest first (admin panel).
	Orders(ctx context.Context, limit int) ([]Order, error)
}

type firestoreRepo struct {
	client *firestore.Client
}

func NewFirestoreRepository(client *firestore.Client) Repository {
	return &firestoreRepo{client: client}
}

func (r *firestoreRepo) Deliver(ctx context.Context, uid, orderID string, fn func(*grant.Doc) (bool, *Order, error)) error {
	orderRef := r.client.Collection(OrdersCollection).Doc(orderID)
	grantsRef := grant.DocRef(r.client, uid)
	return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		var existing *Order
		snap, err := tx.Get(orderRef)
		switch {
		case status.Code(err) == codes.NotFound:
		case err != nil:
			return err
		default:
			var o Order
			if err := snap.DataTo(&o); err != nil {
				return err
			}
			existing = &o
		}
		if existing != nil && existing.UID != uid {
			return ErrOrderUsed
		}

		grantsSnap, err := tx.Get(grantsRef)
		if err != nil && status.Code(err) != codes.NotFound {
			return err
		}
		grants, err := grant.Decode(grantsSnap)
		if err != nil {
			return err
		}

		write, order, err := fn(grants)
		if err != nil {
			return err
		}
		if !write {
			return nil
		}
		if err := tx.Set(grantsRef, grants); err != nil {
			return err
		}
		if existing == nil && order != nil {
			return tx.Create(orderRef, order)
		}
		return nil
	})
}

func (r *firestoreRepo) Orders(ctx context.Context, limit int) ([]Order, error) {
	it := r.client.Collection(OrdersCollection).OrderBy("createdAt", firestore.Desc).Limit(limit).Documents(ctx)
	defer it.Stop()

	out := []Order{}
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		var o Order
		if err := snap.DataTo(&o); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
}
