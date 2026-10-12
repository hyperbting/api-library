package route

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"api-library/internal/app"
	"api-library/internal/config"
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
