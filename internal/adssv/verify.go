// Package adssv is the AdMob server-side verification (SSV) callback: when a player finishes a rewarded ad, Google
// calls this service with a signed GET naming the player (user_id) and what the client attached (custom_data = the
// requestId of POST /ads/reward). A verified callback is stored, and the reward route pays only for a requestId that
// has one (package gem, AdVerifier), so a modified client cannot claim a reward without watching an ad.
//
// Signature: ECDSA P-256 over SHA-256 of the query string up to "&signature=", the signature is base64url DER, the
// key is looked up by key_id in Google's published list (KeysURL).
// https://developers.google.com/admob/android/ssv
package adssv

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	KeysURL = "https://www.gstatic.com/admob/reward/verifier-keys.json"

	// MaxAge is how old a callback's timestamp may be. The callback is sent right after the ad; a replayed old
	// URL is refused even though its signature is still good.
	MaxAge = time.Hour
	// futureSkew is the clock difference tolerated for a timestamp ahead of ours.
	futureSkew = 5 * time.Minute
	// keyRefetchGap limits how often an unknown key_id may trigger a download of the key list.
	keyRefetchGap = time.Minute
	// keyTTL is when a cached key list is refreshed without an unknown key_id asking for it.
	keyTTL = 24 * time.Hour
)

var (
	ErrBadSignature = errors.New("bad signature")
	ErrBadRequest   = errors.New("bad callback")
	ErrStale        = errors.New("callback timestamp out of range")
	ErrAdUnit       = errors.New("ad unit not accepted")
	ErrUnknownKey   = errors.New("unknown key id")
	// ErrNoReward is a callback with a good signature that names no player or request: the test call AdMob sends
	// when the callback URL is saved in the console (dummy values, no user_id or custom_data). It is answered 200
	// and nothing is stored, so it can never pay anything.
	ErrNoReward = errors.New("signed callback without a player or request")
)

// Callback is one verified SSV call.
type Callback struct {
	AdUnit        string
	RewardItem    string
	RewardAmount  int
	TransactionID string
	UserID        string
	CustomData    string
	At            time.Time
}

// Verifier checks callbacks. Zero value is not usable: use NewVerifier.
type Verifier struct {
	client  *http.Client
	keysURL string
	now     func() time.Time
	units   map[string]bool

	mu        sync.Mutex
	keys      map[string]*ecdsa.PublicKey
	fetchedAt time.Time
}

type Option func(*Verifier)

// WithHTTPClient and WithKeysURL are for tests.
func WithHTTPClient(c *http.Client) Option { return func(v *Verifier) { v.client = c } }
func WithKeysURL(u string) Option          { return func(v *Verifier) { v.keysURL = u } }
func WithClock(now func() time.Time) Option {
	return func(v *Verifier) { v.now = now }
}

// NewVerifier accepts only callbacks for the given ad units. The list is required: anyone with an AdMob account
// can get Google to sign a callback with a user_id of their choice, so the signature alone proves nothing about
// whose ad it was. An empty list accepts nothing.
func NewVerifier(adUnits []string, opts ...Option) *Verifier {
	v := &Verifier{
		client:  &http.Client{Timeout: 10 * time.Second},
		keysURL: KeysURL,
		now:     time.Now,
		units:   map[string]bool{},
		keys:    map[string]*ecdsa.PublicKey{},
	}
	for _, u := range adUnits {
		if u = strings.TrimSpace(u); u != "" {
			v.units[u] = true
		}
	}
	for _, o := range opts {
		o(v)
	}
	return v
}

// ParseAdUnits splits a comma separated list (the env value). AdMob names a unit "ca-app-pub-<pub>/<unit>" in the
// app, and the callback's ad_unit holds only the unit number, so both forms are accepted for each entry.
func ParseAdUnits(list string) []string {
	var out []string
	for _, u := range strings.Split(list, ",") {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		out = append(out, u)
		if i := strings.LastIndex(u, "/"); i >= 0 && i+1 < len(u) {
			out = append(out, u[i+1:])
		}
	}
	return out
}

// Verify checks the raw query string of a callback and returns what it says.
func (v *Verifier) Verify(ctx context.Context, rawQuery string) (*Callback, error) {
	const marker = "&signature="
	i := strings.Index(rawQuery, marker)
	if i < 0 {
		return nil, fmt.Errorf("%w: no signature", ErrBadRequest)
	}
	signed := rawQuery[:i]
	tail, err := url.ParseQuery(rawQuery[i+1:])
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	sigText, keyID := tail.Get("signature"), tail.Get("key_id")
	if sigText == "" || keyID == "" {
		return nil, fmt.Errorf("%w: no signature or key_id", ErrBadRequest)
	}
	sig, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(sigText, "="))
	if err != nil {
		return nil, fmt.Errorf("%w: signature encoding", ErrBadRequest)
	}

	key, err := v.key(ctx, keyID)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(signed))
	if !ecdsa.VerifyASN1(key, digest[:], sig) {
		return nil, ErrBadSignature
	}

	// Only now are the fields trustworthy.
	q, err := url.ParseQuery(signed)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	cb := &Callback{
		AdUnit:        q.Get("ad_unit"),
		RewardItem:    q.Get("reward_item"),
		TransactionID: q.Get("transaction_id"),
		UserID:        q.Get("user_id"),
		CustomData:    q.Get("custom_data"),
	}
	cb.RewardAmount, _ = strconv.Atoi(q.Get("reward_amount"))
	if cb.TransactionID == "" || cb.UserID == "" || cb.CustomData == "" {
		return nil, ErrNoReward
	}
	ms, err := strconv.ParseInt(q.Get("timestamp"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w: timestamp", ErrBadRequest)
	}
	cb.At = time.UnixMilli(ms)

	if !v.units[cb.AdUnit] {
		return nil, ErrAdUnit
	}
	if now := v.now(); cb.At.Before(now.Add(-MaxAge)) || cb.At.After(now.Add(futureSkew)) {
		return nil, ErrStale
	}
	return cb, nil
}

// key returns the public key for keyID, downloading the list when it is unknown (at most once per keyRefetchGap).
func (v *Verifier) key(ctx context.Context, keyID string) (*ecdsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.now()
	if k, ok := v.keys[keyID]; ok && now.Sub(v.fetchedAt) < keyTTL {
		return k, nil
	}
	if !v.fetchedAt.IsZero() && now.Sub(v.fetchedAt) < keyRefetchGap {
		if k, ok := v.keys[keyID]; ok {
			return k, nil
		}
		return nil, ErrUnknownKey
	}
	keys, err := v.fetch(ctx)
	if err != nil {
		if k, ok := v.keys[keyID]; ok { // keep serving a known key while Google's list is unreachable
			return k, nil
		}
		return nil, err
	}
	v.keys, v.fetchedAt = keys, now
	if k, ok := keys[keyID]; ok {
		return k, nil
	}
	return nil, ErrUnknownKey
}

func (v *Verifier) fetch(ctx context.Context) (map[string]*ecdsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.keysURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("verifier keys: status %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var list struct {
		Keys []struct {
			KeyID  json.Number `json:"keyId"`
			Base64 string      `json:"base64"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	keys := make(map[string]*ecdsa.PublicKey, len(list.Keys))
	for _, k := range list.Keys {
		der, err := base64.StdEncoding.DecodeString(k.Base64)
		if err != nil {
			continue
		}
		pub, err := x509.ParsePKIXPublicKey(der)
		if err != nil {
			continue
		}
		if ec, ok := pub.(*ecdsa.PublicKey); ok {
			keys[k.KeyID.String()] = ec
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("verifier keys: none usable")
	}
	return keys, nil
}
