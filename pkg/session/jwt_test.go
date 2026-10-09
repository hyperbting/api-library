package session

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func newTestManager(t *testing.T) TokenManager {
	t.Helper()
	tm, err := NewTokenManager(&Config{
		SecretKey:  "test-secret-key",
		Issuer:     "test-issuer",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewTokenManager: %v", err)
	}
	return tm
}

func TestNewTokenManager_Validation(t *testing.T) {
	cases := map[string]*Config{
		"nil config":       nil,
		"empty secret":     {Issuer: "i", AccessTTL: time.Minute, RefreshTTL: time.Hour},
		"empty issuer":     {SecretKey: "s", AccessTTL: time.Minute, RefreshTTL: time.Hour},
		"zero access ttl":  {SecretKey: "s", Issuer: "i", RefreshTTL: time.Hour},
		"zero refresh ttl": {SecretKey: "s", Issuer: "i", AccessTTL: time.Minute},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewTokenManager(cfg); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestGenerateAccessToken(t *testing.T) {
	tm := newTestManager(t)

	at, err := tm.GenerateAccessToken("e|player@example.com", []string{"player", "admin"})
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	if at.Token == "" {
		t.Fatal("expected non-empty token")
	}
	if d := time.Until(at.ExpiresAt); d < 14*time.Minute || d > 16*time.Minute {
		t.Errorf("unexpected expiry window: %v", d)
	}

	claims, err := tm.ValidateAccessToken(at.Token)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if claims.UserID != "e|player@example.com" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "e|player@example.com")
	}
	if !reflect.DeepEqual(claims.Roles, []string{"player", "admin"}) {
		t.Errorf("Roles = %v, want [player admin]", claims.Roles)
	}
}

func TestValidateAccessToken_RejectsGarbage(t *testing.T) {
	tm := newTestManager(t)
	if _, err := tm.ValidateAccessToken("not-a-jwt"); err == nil {
		t.Fatal("expected error for malformed token, got nil")
	}
}

func TestValidateAccessToken_RejectsWrongSecret(t *testing.T) {
	tm := newTestManager(t)
	other, err := NewTokenManager(&Config{
		SecretKey:  "a-different-secret",
		Issuer:     "test-issuer",
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewTokenManager: %v", err)
	}

	at, err := tm.GenerateAccessToken("u1", nil)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}
	if _, err := other.ValidateAccessToken(at.Token); err == nil {
		t.Fatal("expected error for token signed with a different secret, got nil")
	}
}

func TestGenerateTokenPair(t *testing.T) {
	tm := newTestManager(t)

	tp, err := tm.GenerateTokenPair("u1", []string{"player"})
	if err != nil {
		t.Fatalf("GenerateTokenPair: %v", err)
	}
	if tp.AccessToken == "" || tp.RefreshToken == "" || tp.JTI == "" {
		t.Fatalf("expected non-empty tokens and JTI, got %+v", tp)
	}

	ac, err := tm.ValidateAccessToken(tp.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if ac.UserID != "u1" {
		t.Errorf("access UserID = %q, want u1", ac.UserID)
	}

	rc, err := tm.ValidateRefreshToken(tp.RefreshToken)
	if err != nil {
		t.Fatalf("ValidateRefreshToken: %v", err)
	}
	if rc.ID != tp.JTI {
		t.Errorf("refresh JTI = %q, want %q", rc.ID, tp.JTI)
	}
}

// failingRepo errors on every call: IssueAccessToken must not touch the store.
type failingRepo struct{}

func (failingRepo) SaveSession(context.Context, *Session) error {
	return errors.New("repo must not be used")
}
func (failingRepo) Save(context.Context, *Session, time.Duration) error {
	return errors.New("repo must not be used")
}
func (failingRepo) Get(context.Context, string, string) (*Session, error) {
	return nil, errors.New("repo must not be used")
}
func (failingRepo) Delete(context.Context, string, string) error {
	return errors.New("repo must not be used")
}
func (failingRepo) DeleteAll(context.Context, string) error {
	return errors.New("repo must not be used")
}

func TestIssueAccessToken_NoSessionStore(t *testing.T) {
	tm := newTestManager(t)
	srv := NewService(tm, failingRepo{})

	at, err := srv.IssueAccessToken(context.Background(), "e|player@example.com", []string{"player"})
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	claims, err := tm.ValidateAccessToken(at.Token)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if claims.UserID != "e|player@example.com" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "e|player@example.com")
	}
	if !reflect.DeepEqual(claims.Roles, []string{"player"}) {
		t.Errorf("Roles = %v, want [player]", claims.Roles)
	}
}
