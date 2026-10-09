package user

import (
	"testing"
	"time"
)

func TestFirestoreDocRoundTrip(t *testing.T) {
	u, err := NewEmailPasswordUser("player@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	u.EmailVerifiedAt = &now
	u.Status = AccountStatusActive

	doc := toDoc(u)
	// Firestore returns arrays as []any
	doc[fRoles] = []any{"player", "admin"}
	got := fromDoc(doc)

	if got.Email == nil || *got.Email != "player@example.com" || got.Platform != DevicePlatformEMAIL {
		t.Fatalf("identity lost: %+v", got)
	}
	if ok, _ := got.VerifyPassword("password123"); !ok {
		t.Fatal("password hash lost")
	}
	if !got.Roles.IsAdmin() || !got.Roles.IsPlayer() {
		t.Fatalf("roles lost: %v", got.Roles)
	}
	if got.EmailVerifiedAt == nil || got.Status != AccountStatusActive {
		t.Fatalf("status/verified lost: %+v", got)
	}
}

func TestDocIDMatchesAccessTokenIdentity(t *testing.T) {
	if got := docID(DevicePlatformEMAIL, "player@example.com"); got != "e|player@example.com" {
		t.Fatalf("got %q", got)
	}
}
