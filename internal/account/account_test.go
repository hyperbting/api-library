package account

import (
	"context"
	"net/http/httptest"
	"testing"

	"firebase.google.com/go/v4/auth"
	"github.com/gofiber/fiber/v3"
)

type fakeSource struct {
	users []*auth.ExportedUserRecord
	next  string
	size  int
}

func (f *fakeSource) Users(_ context.Context, pageSize int, _ string) ([]*auth.ExportedUserRecord, string, error) {
	f.size = pageSize
	return f.users, f.next, nil
}

func rec(uid string, providers ...*auth.UserInfo) *auth.ExportedUserRecord {
	return &auth.ExportedUserRecord{UserRecord: &auth.UserRecord{
		UserInfo:         &auth.UserInfo{UID: uid},
		ProviderUserInfo: providers,
		UserMetadata:     &auth.UserMetadata{CreationTimestamp: 1760000000000, LastLogInTimestamp: 1760000100000},
	}}
}

func TestLinkedKeepsOnlyPlatformAccounts(t *testing.T) {
	src := &fakeSource{next: "tok", users: []*auth.ExportedUserRecord{
		rec("guest"), // anonymous: no providers
		rec("pw", &auth.UserInfo{ProviderID: "password", UID: "a@b.c"}),                                                     // email (an admin): not a player link
		rec("pgs", &auth.UserInfo{ProviderID: "playgames.google.com", UID: "g123", DisplayName: " Tortoise "}),              // linked
		rec("gc", &auth.UserInfo{ProviderID: "gc.apple.com", UID: "G:9"}, &auth.UserInfo{ProviderID: "password", UID: "x"}), // linked, extra provider ignored
	}}
	page, err := NewService(src).Linked(context.Background(), 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Scanned != 4 || page.NextPageToken != "tok" || len(page.Accounts) != 2 {
		t.Fatalf("page: %+v", page)
	}
	pgs := page.Accounts[0]
	if pgs.UID != "pgs" || pgs.Providers[0].Label != "Play Games" || pgs.Providers[0].DisplayName != "Tortoise" || pgs.CreatedAt == nil || pgs.LastRefresh != nil {
		t.Fatalf("pgs row: %+v", pgs)
	}
	if page.Accounts[1].UID != "gc" || len(page.Accounts[1].Providers) != 1 {
		t.Fatalf("gc row: %+v", page.Accounts[1])
	}
	if src.size != defaultPageSize {
		t.Fatalf("default page size: %d", src.size)
	}
}

func TestPageSizeIsCapped(t *testing.T) {
	src := &fakeSource{}
	if _, err := NewService(src).Linked(context.Background(), 99999, ""); err != nil {
		t.Fatal(err)
	}
	if src.size != maxPageSize {
		t.Fatalf("page size: %d", src.size)
	}
}

func TestAdminListJSON(t *testing.T) {
	app := fiber.New()
	app.Get("/admin/accounts", NewHandler(NewService(&fakeSource{})).AdminList)
	res, err := app.Test(httptest.NewRequest("GET", "/admin/accounts", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
}
