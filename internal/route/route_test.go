package route

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"api-library/internal/app"
	"api-library/internal/auth"
	"api-library/internal/config"
	"api-library/internal/news"
	"api-library/internal/quest"
	"api-library/internal/user"
	"api-library/pkg/session"

	"github.com/gofiber/fiber/v3"
)

// fakeUserSvc embeds the interface so only RegisterEmailPassword needs stubbing.
type fakeUserSvc struct {
	user.Service
}

func (f *fakeUserSvc) RegisterEmailPassword(_ context.Context, _, _ string) (*user.User, *session.AccessToken, error) {
	return &user.User{}, &session.AccessToken{Token: "access-token"}, nil
}

func TestSetupUserRoutes_RegisterToggle(t *testing.T) {
	container := &app.Container{UserHdl: user.NewHandler(&fakeUserSvc{})}

	for _, tc := range []struct {
		name     string
		enabled  bool
		wantCode int
	}{
		{"enabled", true, fiber.StatusCreated},
		{"disabled", false, fiber.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			api := app.Group("/api/v99")
			cfg := &config.AppConfig{App: config.AppDetail{RegisterEnabled: tc.enabled}}
			setupUserRoutes(api, container, cfg)

			req := httptest.NewRequest("POST", "/api/v99/auth/register", strings.NewReader(`{"email":"player@example.com","password":"password123"}`))
			req.Header.Set("Content-Type", "application/json")
			res, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			if res.StatusCode != tc.wantCode {
				t.Fatalf("got %d want %d", res.StatusCode, tc.wantCode)
			}
		})
	}
}

func TestSetupUserRoutes_AuthRateLimited(t *testing.T) {
	container := &app.Container{UserHdl: user.NewHandler(&fakeUserSvc{})}
	app := fiber.New()
	cfg := &config.AppConfig{App: config.AppDetail{RegisterEnabled: true}}
	setupUserRoutes(app.Group("/api/v99"), container, cfg)

	var last int
	for i := 0; i < 6; i++ {
		req := httptest.NewRequest("POST", "/api/v99/auth/register", strings.NewReader(`{"email":"player@example.com","password":"password123"}`))
		req.Header.Set("Content-Type", "application/json")
		res, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		last = res.StatusCode
	}
	if last != fiber.StatusTooManyRequests {
		t.Fatalf("6th register got %d want %d", last, fiber.StatusTooManyRequests)
	}
}

func TestFirebaseRoutesServeUserAuth(t *testing.T) {
	cfg := &config.AppConfig{App: config.AppDetail{Version: "v99", RegisterEnabled: true}}
	fc := &app.FirebaseContainer{UserHdl: user.NewHandler(&fakeUserSvc{})}
	fiberApp := fiber.New()
	RegisterFirebaseCloudFunctionRoutes(fiberApp, cfg, fc)

	req := httptest.NewRequest("POST", "/api/v99/auth/register", strings.NewReader(`{"email":"player@example.com","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	res, err := fiberApp.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if res.StatusCode != fiber.StatusCreated {
		t.Fatalf("got %d want %d", res.StatusCode, fiber.StatusCreated)
	}
}

// The panel (/admin) follows ADMIN_PANEL_ENABLED and only exists on top of the admin API (ADMIN_ENABLED):
// neither = game API only, admin = + /api/<v>/admin routes, admin + panel = + the embedded panel.
func TestAdminPanelFollowsItsOwnSwitch(t *testing.T) {
	for _, tc := range []struct {
		name         string
		enabled      bool
		panelEnabled bool
		wantPanel    int
	}{
		{"neither", false, false, fiber.StatusNotFound},
		{"admin api only", true, false, fiber.StatusNotFound},
		{"panel without the admin api", false, true, fiber.StatusNotFound},
		{"admin api and panel", true, true, fiber.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.AppConfig{
				App:   config.AppDetail{Version: "v99", RegisterEnabled: true},
				Admin: config.AdminConfig{Enabled: tc.enabled, PanelEnabled: tc.panelEnabled},
			}
			fc := &app.FirebaseContainer{UserHdl: user.NewHandler(&fakeUserSvc{})}
			fiberApp := fiber.New()
			RegisterFirebaseCloudFunctionRoutes(fiberApp, cfg, fc)

			res, err := fiberApp.Test(httptest.NewRequest("GET", "/admin/config.json", nil))
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			if res.StatusCode != tc.wantPanel {
				t.Fatalf("/admin/config.json got %d want %d", res.StatusCode, tc.wantPanel)
			}

			// The game API is there in every combination.
			req := httptest.NewRequest("POST", "/api/v99/auth/register", strings.NewReader(`{"email":"player@example.com","password":"password123"}`))
			req.Header.Set("Content-Type", "application/json")
			res, err = fiberApp.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			if res.StatusCode != fiber.StatusCreated {
				t.Fatalf("register got %d want %d", res.StatusCode, fiber.StatusCreated)
			}
		})
	}
}

// POST /admin/register exists only with ADMIN_REGISTER_EMAIL (and email sign-in), and is reachable without a
// token even though the /admin group registered after it demands one. When it is not mounted the request falls
// through to that group's token gate (401), so it is not reachable anonymously either way.
func TestAdminRegisterFollowsItsSwitch(t *testing.T) {
	create := func(context.Context, string, string) (string, error) { return "uid-1", nil }

	for _, tc := range []struct {
		name     string
		admin    config.AdminConfig
		creator  auth.EmailUserCreator
		wantCode int
	}{
		{"off by default", config.AdminConfig{Enabled: true, LoginEmail: true}, create, fiber.StatusUnauthorized},
		{"on", config.AdminConfig{Enabled: true, LoginEmail: true, RegisterEmail: true}, create, fiber.StatusCreated},
		{"email sign-in is off", config.AdminConfig{Enabled: true, RegisterEmail: true}, create, fiber.StatusUnauthorized},
		{"no creator", config.AdminConfig{Enabled: true, LoginEmail: true, RegisterEmail: true}, nil, fiber.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.AppConfig{App: config.AppDetail{Version: "v99"}, Admin: tc.admin}
			fiberApp := fiber.New()
			api := fiberApp.Group("/api/v99")
			mountAdminRegister(api, cfg, tc.creator)
			// What setupAdminRoutes does next: a token gate on everything else under /admin.
			gate := api.Group("/admin", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusUnauthorized) })
			gate.Get("/me", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

			req := httptest.NewRequest("POST", "/api/v99/admin/register", strings.NewReader(`{"email":"ops@example.com","password":"password123"}`))
			req.Header.Set("Content-Type", "application/json")
			res, err := fiberApp.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			if res.StatusCode != tc.wantCode {
				t.Fatalf("register got %d want %d", res.StatusCode, tc.wantCode)
			}

			me, err := fiberApp.Test(httptest.NewRequest("GET", "/api/v99/admin/me", nil))
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			if me.StatusCode != fiber.StatusUnauthorized {
				t.Fatalf("/admin/me got %d want %d (the gate must still apply)", me.StatusCode, fiber.StatusUnauthorized)
			}
		})
	}
}

// The quest admin routes sit under the /admin group's gate, so without a token they answer 401, and a missing
// handler (quests off) mounts nothing.
func TestAdminQuestRoutesStayBehindTheGate(t *testing.T) {
	fiberApp := fiber.New()
	gate := fiberApp.Group("/api/v99/admin", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusUnauthorized) })
	mountAdminQuestRoutes(gate, quest.NewHandler(nil))
	mountAdminQuestRoutes(gate, nil) // no quest handler: must not panic

	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v99/admin/quests/catalog"},
		{"GET", "/api/v99/admin/quests/progress?uid=u"},
		{"PUT", "/api/v99/admin/quests/progress"},
		{"POST", "/api/v99/admin/quests/progress/reset"},
		{"POST", "/api/v99/admin/quests/reset"},
	} {
		res, err := fiberApp.Test(httptest.NewRequest(tc.method, tc.path, nil))
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		if res.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("%s %s got %d want 401", tc.method, tc.path, res.StatusCode)
		}
	}
}

// The news admin routes sit under the /admin gate (401 without a token) and GET /news is mounted only with a handler.
func TestAdminNewsRoutesStayBehindTheGate(t *testing.T) {
	fiberApp := fiber.New()
	gate := fiberApp.Group("/api/v99/admin", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusUnauthorized) })
	mountAdminNewsRoutes(gate, news.NewHandler(nil, nil))
	mountAdminNewsRoutes(gate, nil) // no handler: must not panic

	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v99/admin/news"},
		{"POST", "/api/v99/admin/news"},
		{"POST", "/api/v99/admin/news/delete"},
	} {
		res, err := fiberApp.Test(httptest.NewRequest(tc.method, tc.path, nil))
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		if res.StatusCode != fiber.StatusUnauthorized {
			t.Fatalf("%s %s got %d want 401", tc.method, tc.path, res.StatusCode)
		}
	}
}
