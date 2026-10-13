package news

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"api-library/internal/auth"

	"github.com/gofiber/fiber/v3"
)

type memRepo struct {
	items map[string]Item
	next  int
}

func newMemRepo() *memRepo { return &memRepo{items: map[string]Item{}} }

func (r *memRepo) List(context.Context) ([]Item, error) {
	out := []Item{}
	for _, i := range r.items {
		out = append(out, i)
	}
	return out, nil
}

func (r *memRepo) Get(_ context.Context, id string) (*Item, error) {
	i, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &i, nil
}

func (r *memRepo) Create(_ context.Context, item *Item) (string, error) {
	r.next++
	id := "n" + string(rune('0'+r.next))
	item.ID = id
	r.items[id] = *item
	return id, nil
}

func (r *memRepo) Update(_ context.Context, item *Item) error {
	if _, ok := r.items[item.ID]; !ok {
		return ErrNotFound
	}
	r.items[item.ID] = *item
	return nil
}

func (r *memRepo) Delete(_ context.Context, id string) error {
	delete(r.items, id)
	return nil
}

var t0 = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func newTestService() (*service, *memRepo) {
	repo := newMemRepo()
	return &service{repo: repo, now: func() time.Time { return t0 }}, repo
}

func req(audience, title string) SaveRequest {
	return SaveRequest{Audience: audience, Title: map[string]string{"en": title}, Body: map[string]string{"en": "body of " + title}}
}

func TestSaveValidates(t *testing.T) {
	s, _ := newTestService()
	ctx := context.Background()

	bad := []SaveRequest{
		{Audience: "everyone", Title: map[string]string{"en": "x"}, Body: map[string]string{"en": "x"}},
		{Audience: AudiencePublic, Title: map[string]string{"zh-Hant": "x"}, Body: map[string]string{"en": "x"}},
		{Audience: AudiencePublic, Title: map[string]string{"en": "x"}, Body: map[string]string{}},
		{Audience: AudiencePublic, Title: map[string]string{"en": "x", "fr": "x"}, Body: map[string]string{"en": "x"}},
		{Audience: AudiencePublic, Title: map[string]string{"en": strings.Repeat("a", 81)}, Body: map[string]string{"en": "x"}},
		{Audience: AudiencePublic, Title: map[string]string{"en": "x"}, Body: map[string]string{"en": strings.Repeat("a", 1001)}},
	}
	for i, r := range bad {
		_, err := s.Save(ctx, r)
		var re *RequestError
		if !errors.As(err, &re) || !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("case %d: want a request error, got %v", i, err)
		}
	}

	// A blank translation is dropped, text is trimmed
	item, err := s.Save(ctx, SaveRequest{
		Audience: AudiencePublic,
		Title:    map[string]string{"en": "  Hello  ", "zh-Hant": "   "},
		Body:     map[string]string{"en": "Body"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Title["en"] != "Hello" || len(item.Title) != 1 {
		t.Fatalf("title: %+v", item.Title)
	}
}

func TestPlayerListHidesDevExpiredAndOrders(t *testing.T) {
	s, repo := newTestService()
	ctx := context.Background()

	put := func(id, audience string, at time.Time, pinned bool, expires *time.Time) {
		repo.items[id] = Item{ID: id, Audience: audience, PublishedAt: at, Pinned: pinned, ExpiresAt: expires,
			Title: map[string]string{"en": id}, Body: map[string]string{"en": id}}
	}
	past := t0.Add(-time.Hour)
	future := t0.Add(time.Hour)
	put("old", AudiencePublic, t0.Add(-48*time.Hour), false, nil)
	put("new", AudiencePublic, t0.Add(-time.Hour), false, nil)
	put("pinned", AudiencePublic, t0.Add(-72*time.Hour), true, nil)
	put("gone", AudiencePublic, t0.Add(-2*time.Hour), false, &past)
	put("soon", AudiencePublic, t0.Add(-3*time.Hour), false, &future)
	put("dev", AudienceDev, t0.Add(-30*time.Minute), false, nil)

	ids := func(items []Item) string {
		var out []string
		for _, i := range items {
			out = append(out, i.ID)
		}
		return strings.Join(out, ",")
	}

	player, _ := s.ForPlayer(ctx, false)
	if got := ids(player); got != "pinned,new,soon,old" {
		t.Fatalf("player list: %s", got)
	}
	admin, _ := s.ForPlayer(ctx, true)
	if got := ids(admin); got != "pinned,dev,new,soon,old" {
		t.Fatalf("admin list: %s", got)
	}
	all, _ := s.All(ctx)
	if len(all) != 6 {
		t.Fatalf("panel list holds expired items too, got %d", len(all))
	}
}

func TestSaveKeepsPublishTimeOnUpdate(t *testing.T) {
	s, _ := newTestService()
	ctx := context.Background()

	created, err := s.Save(ctx, req(AudiencePublic, "First"))
	if err != nil {
		t.Fatal(err)
	}
	if !created.PublishedAt.Equal(t0) {
		t.Fatalf("published at %v", created.PublishedAt)
	}

	s.now = func() time.Time { return t0.Add(24 * time.Hour) }
	update := req(AudienceDev, "Edited")
	update.ID = created.ID
	updated, err := s.Save(ctx, update)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.PublishedAt.Equal(t0) || updated.Audience != AudienceDev || updated.Title["en"] != "Edited" {
		t.Fatalf("updated: %+v", updated)
	}

	update.ID = "missing"
	if _, err := s.Save(ctx, update); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
}

func appFor(t *testing.T, admin bool) (*fiber.App, *memRepo) {
	t.Helper()
	s, repo := newTestService()
	h := NewHandler(s, func(context.Context, string) (bool, error) { return admin, nil })
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("auth:user", auth.UserContext{UserID: "uid"})
		return c.Next()
	})
	app.Get("/news", h.List)
	app.Get("/admin/news", h.AdminList)
	app.Post("/admin/news", h.AdminSave)
	app.Post("/admin/news/delete", h.AdminDelete)
	return app, repo
}

func call(t *testing.T, app *fiber.App, method, path, body string) (int, string) {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	res, err := app.Test(r)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestHandlersFilterDevByAdminRole(t *testing.T) {
	for _, tc := range []struct {
		admin   bool
		wantDev bool
	}{{false, false}, {true, true}} {
		app, repo := appFor(t, tc.admin)
		repo.items["a"] = Item{ID: "a", Audience: AudiencePublic, PublishedAt: t0, Title: map[string]string{"en": "pub"}, Body: map[string]string{"en": "b"}}
		repo.items["b"] = Item{ID: "b", Audience: AudienceDev, PublishedAt: t0, Title: map[string]string{"en": "devonly"}, Body: map[string]string{"en": "b"}}

		code, body := call(t, app, "GET", "/news", "")
		if code != 200 || !strings.Contains(body, "pub") {
			t.Fatalf("admin=%v: %d %s", tc.admin, code, body)
		}
		if got := strings.Contains(body, "devonly"); got != tc.wantDev {
			t.Fatalf("admin=%v: dev item present=%v", tc.admin, got)
		}
	}
}

func TestAdminHandlers(t *testing.T) {
	app, _ := appFor(t, true)

	if code, body := call(t, app, "POST", "/admin/news", `{"audience":"nope","title":{"en":"x"},"body":{"en":"x"}}`); code != 400 || !strings.Contains(body, "audience") {
		t.Fatalf("bad audience: %d %s", code, body)
	}
	code, body := call(t, app, "POST", "/admin/news", `{"audience":"dev","title":{"en":"Known issues"},"body":{"en":"Snooze Zzz may overlap"},"version":"1.0","pinned":true}`)
	if code != 200 || !strings.Contains(body, `"id":"n1"`) || !strings.Contains(body, `"pinned":true`) {
		t.Fatalf("create: %d %s", code, body)
	}
	if code, body := call(t, app, "GET", "/admin/news", ""); code != 200 || !strings.Contains(body, "Known issues") {
		t.Fatalf("list: %d %s", code, body)
	}
	if code, _ := call(t, app, "POST", "/admin/news", `{"id":"zzz","audience":"dev","title":{"en":"x"},"body":{"en":"x"}}`); code != 404 {
		t.Fatalf("update unknown: %d", code)
	}
	if code, _ := call(t, app, "POST", "/admin/news/delete", `{}`); code != 400 {
		t.Fatalf("delete without id: %d", code)
	}
	if code, _ := call(t, app, "POST", "/admin/news/delete", `{"id":"n1"}`); code != 200 {
		t.Fatalf("delete: %d", code)
	}
	if _, body := call(t, app, "GET", "/admin/news", ""); strings.Contains(body, "Known issues") {
		t.Fatalf("still listed: %s", body)
	}
}
