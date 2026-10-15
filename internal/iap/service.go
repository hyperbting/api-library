package iap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"api-library/internal/grant"
	"api-library/internal/item"
)

type Service interface {
	// Purchase verifies a store purchase and delivers it as a grant. A refused purchase is ok=false with an
	// error code; ErrStoreUnavailable (or any other error) means "try again later" and charges nothing.
	Purchase(ctx context.Context, uid string, req PurchaseRequest) (*PurchaseResponse, error)
	// Orders lists the newest delivered store orders of all players (admin panel).
	Orders(ctx context.Context, limit int) ([]Order, error)
}

type service struct {
	repo     Repository
	catalog  *item.Catalog
	verifier Verifier
	now      func() time.Time
}

func NewService(repo Repository, catalog *item.Catalog, verifier Verifier) Service {
	return &service{repo: repo, catalog: catalog, verifier: verifier, now: time.Now}
}

func grantID(orderID string) string { return "iap:" + orderID }

func (s *service) Purchase(ctx context.Context, uid string, req PurchaseRequest) (*PurchaseResponse, error) {
	product, ok := s.catalog.Product(req.SKU)
	if !ok {
		return &PurchaseResponse{ErrorCode: CodeUnknownProduct}, nil
	}
	rewards, err := s.catalog.Resolve(product.Items)
	if err != nil {
		return nil, fmt.Errorf("product %s: %w", product.SKU, err)
	}

	receipt, err := s.verifier.Verify(ctx, req.SKU, req.PurchaseToken)
	if err != nil {
		return nil, err
	}
	switch {
	case receipt.Pending:
		return &PurchaseResponse{ErrorCode: CodePending}, nil
	case receipt.Canceled:
		return &PurchaseResponse{ErrorCode: CodeCanceled}, nil
	case !receipt.Purchased:
		return &PurchaseResponse{ErrorCode: CodeInvalidReceipt}, nil
	}

	id := grantID(receipt.OrderID)
	var res *PurchaseResponse
	err = s.repo.Deliver(ctx, uid, receipt.OrderID, func(grants *grant.Doc) (bool, *Order, error) {
		// fn may be retried by the transaction: build the response from scratch each time.
		now := s.now()
		res = &PurchaseResponse{}

		// The same order again (a retry, a second device of the same account): hand back the grant.
		if g := grants.Grants[id]; g != nil {
			stored := *g
			res.OK = true
			res.Grant = &stored
			return false, nil, nil
		}

		out := append([]grant.Reward(nil), rewards...)
		if product.FirstPurchaseDouble && !boughtBefore(grants, req.SKU) {
			for i := range out {
				if out[i].Type == grant.RewardGem {
					out[i].Amount *= 2
				}
			}
			res.FirstPurchaseBonus = true
		}
		g := grant.Grant{
			ID:        id,
			Source:    grant.SourceIAP,
			SourceRef: req.SKU,
			Rewards:   out,
			CreatedAt: now,
		}
		if _, err := grants.Add(g); err != nil {
			return false, nil, err
		}
		grants.UpdatedAt = now
		stored := *grants.Grants[id]
		res.OK = true
		res.Grant = &stored
		return true, &Order{OrderID: receipt.OrderID, UID: uid, SKU: req.SKU, GrantID: id, CreatedAt: now}, nil
	})
	if errors.Is(err, ErrOrderUsed) {
		return &PurchaseResponse{ErrorCode: CodeOrderUsed}, nil
	}
	if err != nil {
		return nil, err
	}

	// The grant is stored: tell the store the product was delivered. A failure here is only logged: the
	// grant exists, and the next purchase attempt with the same token consumes it again.
	if !receipt.Consumed {
		if err := s.verifier.Consume(ctx, req.SKU, req.PurchaseToken); err != nil {
			log.Printf("[iap] consume uid=%s sku=%s order=%s: %v", uid, req.SKU, receipt.OrderID, err)
		}
	}
	return res, nil
}

// boughtBefore reports whether the account already has an IAP grant for this SKU.
func boughtBefore(grants *grant.Doc, sku string) bool {
	for _, g := range grants.Grants {
		if g.Source == grant.SourceIAP && g.SourceRef == sku {
			return true
		}
	}
	return false
}

// MaxOrdersListed bounds the admin list.
const MaxOrdersListed = 200

func (s *service) Orders(ctx context.Context, limit int) ([]Order, error) {
	if limit < 1 || limit > MaxOrdersListed {
		limit = 50
	}
	return s.repo.Orders(ctx, limit)
}
