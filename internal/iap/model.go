// Package iap turns a store purchase into a grant. The client buys a product through the store (Google Play
// first), sends the purchase token here, and the server asks the store whether it is real before it creates
// the grant (see package grant) with the product's rewards. The grant ID is derived from the store's order ID,
// so a token that is sent twice (a retry, a second device, a replay) delivers once.
//
// Gem packs are the only products today (package item, "products"); the gems themselves are credited from the
// grant by package gem.
package iap

import (
	"errors"
	"time"

	"api-library/internal/grant"
)

// Firestore layout (server-only):
//
//	users/{uid}/private/grants   the grant "iap:<orderId>" (package grant)
//	iapOrders/{orderId}          Order, which account the order was delivered to
const OrdersCollection = "iapOrders"

// Error codes returned in PurchaseResponse.ErrorCode (HTTP 200, ok=false).
const (
	CodeUnknownProduct = "unknown_product"
	CodeInvalidReceipt = "invalid_receipt"
	CodePending        = "pending"
	CodeCanceled       = "canceled"
	CodeOrderUsed      = "order_used"
)

var (
	ErrInvalidRequest = errors.New("invalid purchase request")
	// ErrStoreUnavailable: the store could not be asked. Not the player's fault; the client retries later.
	ErrStoreUnavailable = errors.New("store verification unavailable")
)

// Order records that a store order was delivered, and to whom.
type Order struct {
	OrderID   string    `firestore:"orderId"`
	UID       string    `firestore:"uid"`
	SKU       string    `firestore:"sku"`
	GrantID   string    `firestore:"grantId"`
	CreatedAt time.Time `firestore:"createdAt"`
}

// Receipt is what the store says about a purchase token.
type Receipt struct {
	OrderID string
	// Purchased is a completed, paid purchase. Pending and Canceled are the other store states.
	Purchased bool
	Pending   bool
	Canceled  bool
	// Consumed: the store already treats the product as consumed (the grant was delivered before).
	Consumed bool
}

// POST /api/{version}/iap/purchase
type PurchaseRequest struct {
	SKU           string `json:"sku" validate:"required,max=64"`
	PurchaseToken string `json:"purchaseToken" validate:"required,max=2048"`
}

type PurchaseResponse struct {
	OK        bool   `json:"ok"`
	ErrorCode string `json:"errorCode,omitempty"`
	// The grant that carries the gems. The same one comes back when the token is sent again.
	Grant *grant.Grant `json:"grant,omitempty"`
	// True when this grant got the first-purchase bonus (the gems are doubled).
	FirstPurchaseBonus bool `json:"firstPurchaseBonus,omitempty"`
}
