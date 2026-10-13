package news

import (
	"context"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Repository interface {
	// List returns the items, newest first (at most maxItems).
	List(ctx context.Context) ([]Item, error)
	// Get returns one item, or ErrNotFound.
	Get(ctx context.Context, id string) (*Item, error)
	// Create stores a new item and returns its ID.
	Create(ctx context.Context, item *Item) (string, error)
	// Update replaces an existing item; ErrNotFound when it does not exist.
	Update(ctx context.Context, item *Item) error
	// Delete removes an item (no error when it does not exist).
	Delete(ctx context.Context, id string) error
}

type firestoreRepo struct {
	client *firestore.Client
}

func NewFirestoreRepository(client *firestore.Client) Repository {
	return &firestoreRepo{client: client}
}

func (r *firestoreRepo) col() *firestore.CollectionRef { return r.client.Collection(Collection) }

func (r *firestoreRepo) List(ctx context.Context) ([]Item, error) {
	it := r.col().OrderBy("publishedAt", firestore.Desc).Limit(maxItems).Documents(ctx)
	defer it.Stop()

	items := []Item{}
	for {
		snap, err := it.Next()
		if err == iterator.Done {
			return items, nil
		}
		if err != nil {
			return nil, err
		}
		var item Item
		if err := snap.DataTo(&item); err != nil {
			return nil, err
		}
		item.ID = snap.Ref.ID
		items = append(items, item)
	}
}

func (r *firestoreRepo) Get(ctx context.Context, id string) (*Item, error) {
	snap, err := r.col().Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var item Item
	if err := snap.DataTo(&item); err != nil {
		return nil, err
	}
	item.ID = snap.Ref.ID
	return &item, nil
}

func (r *firestoreRepo) Create(ctx context.Context, item *Item) (string, error) {
	ref, _, err := r.col().Add(ctx, item)
	if err != nil {
		return "", err
	}
	return ref.ID, nil
}

func (r *firestoreRepo) Update(ctx context.Context, item *Item) error {
	ref := r.col().Doc(item.ID)
	if _, err := ref.Get(ctx); status.Code(err) == codes.NotFound {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	_, err := ref.Set(ctx, item)
	return err
}

func (r *firestoreRepo) Delete(ctx context.Context, id string) error {
	_, err := r.col().Doc(id).Delete(ctx)
	return err
}
