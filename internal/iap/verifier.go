package iap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/oauth2/google"
)

// Verifier asks the store about a purchase token.
type Verifier interface {
	Verify(ctx context.Context, sku, token string) (*Receipt, error)
	// Consume tells the store the product was delivered (a consumable that is not consumed or acknowledged
	// within three days is refunded by Google Play).
	Consume(ctx context.Context, sku, token string) error
}

const androidPublisherScope = "https://www.googleapis.com/auth/androidpublisher"

// GooglePlayVerifier calls the Google Play Developer API (purchases.products) with the service's default
// credentials. The Cloud Run runtime account must be invited in Play Console > Users and permissions with
// the "View financial data" and "Manage orders and subscriptions" permissions, and the Google Play Android
// Developer API must be enabled in the project. It is untested against the live API (no credentials in tests).
type GooglePlayVerifier struct {
	PackageName string

	once   sync.Once
	client *http.Client
	err    error
}

func NewGooglePlayVerifier(packageName string) *GooglePlayVerifier {
	return &GooglePlayVerifier{PackageName: packageName}
}

func (v *GooglePlayVerifier) http(ctx context.Context) (*http.Client, error) {
	v.once.Do(func() {
		// The client is built once; its transport refreshes the access token by itself.
		v.client, v.err = google.DefaultClient(context.Background(), androidPublisherScope)
		if v.client != nil {
			v.client.Timeout = 15 * time.Second
		}
	})
	return v.client, v.err
}

func (v *GooglePlayVerifier) productURL(sku, token string) string {
	return fmt.Sprintf("https://androidpublisher.googleapis.com/androidpublisher/v3/applications/%s/purchases/products/%s/tokens/%s",
		url.PathEscape(v.PackageName), url.PathEscape(sku), url.PathEscape(token))
}

// playProduct is the part of purchases.products we read.
type playProduct struct {
	PurchaseState    int    `json:"purchaseState"`    // 0 purchased, 1 canceled, 2 pending
	ConsumptionState int    `json:"consumptionState"` // 0 yet to be consumed, 1 consumed
	OrderID          string `json:"orderId"`
}

func (v *GooglePlayVerifier) Verify(ctx context.Context, sku, token string) (*Receipt, error) {
	client, err := v.http(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.productURL(sku, token), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch {
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		// The store does not know this token (or SKU): a bad or forged receipt.
		return &Receipt{}, nil
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%w: play api %d: %s", ErrStoreUnavailable, resp.StatusCode, truncate(string(body), 200))
	}

	var p playProduct
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	return &Receipt{
		OrderID:   p.OrderID,
		Purchased: p.PurchaseState == 0 && p.OrderID != "",
		Canceled:  p.PurchaseState == 1,
		Pending:   p.PurchaseState == 2,
		Consumed:  p.ConsumptionState == 1,
	}, nil
}

func (v *GooglePlayVerifier) Consume(ctx context.Context, sku, token string) error {
	client, err := v.http(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.productURL(sku, token)+":consume", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStoreUnavailable, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	// 204 consumed; 400 usually means it was already consumed, which is fine.
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest {
		return nil
	}
	return fmt.Errorf("play consume %d: %s", resp.StatusCode, truncate(string(body), 200))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
