package adssv

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

var clock = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

type testKeys struct {
	priv    *ecdsa.PrivateKey
	keyID   string
	server  *httptest.Server
	fetches int
}

func newTestKeys(t *testing.T) *testKeys {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	k := &testKeys{priv: priv, keyID: "3335741209"}
	k.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		k.fetches++
		body := fmt.Sprintf(`{"keys":[{"keyId":%s,"pem":"x","base64":%q}]}`, k.keyID, base64.StdEncoding.EncodeToString(der))
		w.Write([]byte(body))
	}))
	t.Cleanup(k.server.Close)
	return k
}

func (k *testKeys) verifier(units ...string) *Verifier {
	return NewVerifier(units, WithKeysURL(k.server.URL), WithClock(func() time.Time { return clock }))
}

// callback builds a signed query string the way AdMob does: parameters first, then signature and key_id.
func (k *testKeys) callback(t *testing.T, mutate func(url.Values)) string {
	t.Helper()
	q := url.Values{}
	q.Set("ad_network", "5450213213286189855")
	q.Set("ad_unit", "1234567890")
	q.Set("custom_data", "ad-req-0001")
	q.Set("reward_amount", "1")
	q.Set("reward_item", "warp")
	q.Set("timestamp", strconv.FormatInt(clock.Add(-time.Minute).UnixMilli(), 10))
	q.Set("transaction_id", "tx-1")
	q.Set("user_id", "uid-1")
	if mutate != nil {
		mutate(q)
	}
	// AdMob's order is alphabetical by key, which url.Values.Encode also gives.
	signed := q.Encode()
	digest := sha256.Sum256([]byte(signed))
	sig, err := ecdsa.SignASN1(rand.Reader, k.priv, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signed + "&signature=" + base64.RawURLEncoding.EncodeToString(sig) + "&key_id=" + k.keyID
}

func TestVerifyAcceptsAGoodCallback(t *testing.T) {
	k := newTestKeys(t)
	cb, err := k.verifier("1234567890").Verify(context.Background(), k.callback(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if cb.UserID != "uid-1" || cb.CustomData != "ad-req-0001" || cb.TransactionID != "tx-1" || cb.RewardAmount != 1 {
		t.Errorf("callback = %+v", cb)
	}
}

func TestVerifyRejects(t *testing.T) {
	k := newTestKeys(t)
	good := k.callback(t, nil)
	cases := map[string]struct {
		query string
		units []string
		want  error
	}{
		"tampered user":     {query: replace(good, "uid-1", "uid-2"), units: []string{"1234567890"}, want: ErrBadSignature},
		"tampered data":     {query: replace(good, "ad-req-0001", "ad-req-0002"), units: []string{"1234567890"}, want: ErrBadSignature},
		"no signature":      {query: "ad_unit=1&user_id=u", units: []string{"1"}, want: ErrBadRequest},
		"unknown key":       {query: replace(good, "key_id="+k.keyID, "key_id=1"), units: []string{"1234567890"}, want: ErrUnknownKey},
		"other ad unit":     {query: good, units: []string{"999"}, want: ErrAdUnit},
		"no ad units set":   {query: good, units: nil, want: ErrAdUnit},
		"stale":             {query: k.callback(t, func(q url.Values) { q.Set("timestamp", strconv.FormatInt(clock.Add(-2*time.Hour).UnixMilli(), 10)) }), units: []string{"1234567890"}, want: ErrStale},
		"from the future":   {query: k.callback(t, func(q url.Values) { q.Set("timestamp", strconv.FormatInt(clock.Add(time.Hour).UnixMilli(), 10)) }), units: []string{"1234567890"}, want: ErrStale},
		"no custom data":    {query: k.callback(t, func(q url.Values) { q.Del("custom_data") }), units: []string{"1234567890"}, want: ErrBadRequest},
		"no transaction id": {query: k.callback(t, func(q url.Values) { q.Del("transaction_id") }), units: []string{"1234567890"}, want: ErrBadRequest},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := k.verifier(c.units...).Verify(context.Background(), c.query)
			if !errors.Is(err, c.want) {
				t.Errorf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestVerifyAcceptsTheFullUnitNameAndTheBareNumber(t *testing.T) {
	k := newTestKeys(t)
	units := ParseAdUnits("ca-app-pub-111/1234567890, ")
	if _, err := k.verifier(units...).Verify(context.Background(), k.callback(t, nil)); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyCachesTheKeysAndLimitsRefetches(t *testing.T) {
	k := newTestKeys(t)
	v := k.verifier("1234567890")
	for i := 0; i < 3; i++ {
		if _, err := v.Verify(context.Background(), k.callback(t, nil)); err != nil {
			t.Fatal(err)
		}
	}
	if k.fetches != 1 {
		t.Errorf("fetches = %d, want 1", k.fetches)
	}
	// An unknown key ID right after a download does not trigger another download.
	bad := replace(k.callback(t, nil), "key_id="+k.keyID, "key_id=7")
	for i := 0; i < 3; i++ {
		v.Verify(context.Background(), bad)
	}
	if k.fetches != 1 {
		t.Errorf("fetches = %d after unknown keys, want 1", k.fetches)
	}
}

func TestVerifyWithoutKeysFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	k := newTestKeys(t)
	v := NewVerifier([]string{"1234567890"}, WithKeysURL(srv.URL), WithClock(func() time.Time { return clock }))
	if _, err := v.Verify(context.Background(), k.callback(t, nil)); err == nil {
		t.Fatal("verified without any key")
	}
}

// memRepo stands in for Firestore.
type memRepo struct{ recs map[string]Record }

func (m *memRepo) Save(_ context.Context, r Record) error {
	if _, ok := m.recs[r.TransactionID]; !ok {
		m.recs[r.TransactionID] = r
	}
	return nil
}

func (m *memRepo) Has(_ context.Context, uid, requestID string) (bool, error) {
	for _, r := range m.recs {
		if r.UID == uid && r.RequestID == requestID {
			return true, nil
		}
	}
	return false, nil
}

func TestHandlerStoresAVerifiedCallbackOnce(t *testing.T) {
	k := newTestKeys(t)
	repo := &memRepo{recs: map[string]Record{}}
	srv := NewService(k.verifier("1234567890"), repo)
	srv.now = func() time.Time { return clock }
	app := fiber.New()
	app.Get("/ssv", NewHandler(srv).Callback)

	get := func(query string) int {
		res, err := app.Test(httptest.NewRequest("GET", "/ssv?"+query, nil))
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode
	}
	good := k.callback(t, nil)
	if got := get(good); got != 200 {
		t.Fatalf("status = %d", got)
	}
	if got := get(good); got != 200 { // a replay is answered 200, stored once
		t.Fatalf("replay status = %d", got)
	}
	if len(repo.recs) != 1 {
		t.Fatalf("records = %d, want 1", len(repo.recs))
	}
	r := repo.recs["tx-1"]
	if r.UID != "uid-1" || r.RequestID != "ad-req-0001" || !r.ExpireAt.After(clock) {
		t.Errorf("record = %+v", r)
	}
	if ok, _ := srv.Verified(context.Background(), "uid-1", "ad-req-0001"); !ok {
		t.Error("Verified = false for the stored callback")
	}
	if ok, _ := srv.Verified(context.Background(), "uid-2", "ad-req-0001"); ok {
		t.Error("Verified = true for another player")
	}
	if got := get(replace(good, "uid-1", "uid-2")); got != 400 {
		t.Errorf("tampered status = %d, want 400", got)
	}
	if got := get(""); got != 400 {
		t.Errorf("empty status = %d, want 400", got)
	}
}

func TestKeysJSONNumberKeyID(t *testing.T) {
	var list struct {
		Keys []struct {
			KeyID json.Number `json:"keyId"`
		} `json:"keys"`
	}
	if err := json.Unmarshal([]byte(`{"keys":[{"keyId":3335741209}]}`), &list); err != nil || list.Keys[0].KeyID.String() != "3335741209" {
		t.Fatalf("%v %+v", err, list)
	}
}

func replace(s, old, new string) string {
	i := indexOf(s, old)
	if i < 0 {
		panic("replace: " + old + " not in " + s)
	}
	return s[:i] + new + s[i+len(old):]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
