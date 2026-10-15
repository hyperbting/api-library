package route

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"api-library/internal/app"
	"api-library/internal/auth"
	"api-library/internal/buildinfo"
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

// The admin routes accept only the sign-in methods the switches name; a signed custom token needs its own switch.
func TestAdminSignInProvidersFollowTheSwitches(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.AdminConfig
		want []string
	}{
		{"nothing on", config.AdminConfig{}, nil},
		{"email only", config.AdminConfig{LoginEmail: true}, []string{auth.ProviderPassword}},
		{"email and google", config.AdminConfig{LoginEmail: true, LoginGoogle: true}, []string{auth.ProviderPassword, auth.ProviderGoogle}},
		{"custom off by default", config.AdminConfig{LoginEmail: true}, []string{auth.ProviderPassword}},
		{"custom on", config.AdminConfig{LoginEmail: true, AcceptCustomToken: true}, []string{auth.ProviderPassword, auth.ProviderCustom}},
		{"custom alone", config.AdminConfig{AcceptCustomToken: true}, []string{auth.ProviderCustom}},
	}
	for _, c := range cases {
		got := adminSignInProviders(c.cfg)
		if len(got) != len(c.want) {
			t.Fatalf("%s: got %v, want %v", c.name, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%s: got %v, want %v", c.name, got, c.want)
			}
		}
	}
}

// /healthz says which commit the image was built from (stamped with -ldflags by the Cloud Run build).
func TestHealthzReportsTheBuildCommit(t *testing.T) {
	old := buildinfo.Commit
	buildinfo.Commit = "ab12cd3-dirty"
	defer func() { buildinfo.Commit = old }()

	app := fiber.New()
	app.Get("/healthz", healthz)
	res, err := app.Test(httptest.NewRequest("GET", "/healthz", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct{ Status, Commit string }
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != fiber.StatusOK || body.Status != "ok" || body.Commit != "ab12cd3-dirty" {
		t.Fatalf("healthz: %d %+v", res.StatusCode, body)
	}
}

// The Cloud Run entrypoint mounts only the Firebase routes, so /version has to exist there, even with Firebase off.
func TestVersionRouteOnTheCloudRunEntrypoint(t *testing.T) {
	old := buildinfo.Commit
	buildinfo.Commit = "ef56ab7"
	defer func() { buildinfo.Commit = old }()

	app := fiber.New()
	RegisterFirebaseCloudFunctionRoutes(app, &config.AppConfig{App: config.AppDetail{Version: "v99"}}, nil)
	res, err := app.Test(httptest.NewRequest("GET", "/version", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body struct{ Status, Commit string }
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != fiber.StatusOK || body.Commit != "ef56ab7" {
		t.Fatalf("version: %d %+v", res.StatusCode, body)
	}
}

// userLimiter counts per client when there is no signed-in user: the fourth request in a minute is refused, and a
// different client is not affected.
func TestUserLimiterFallsBackToClient(t *testing.T) {
	app := fiber.New()
	app.Get("/x", userLimiter(3), func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	do := func(ip string) int {
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("X-Forwarded-For", ip)
		res, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		return res.StatusCode
	}

	for i := 1; i <= 3; i++ {
		if code := do("203.0.113.5"); code != fiber.StatusOK {
			t.Fatalf("request %d: %d", i, code)
		}
	}
	if code := do("203.0.113.5"); code != fiber.StatusTooManyRequests {
		t.Fatalf("fourth request: %d, want 429", code)
	}
	if code := do("203.0.113.6"); code != fiber.StatusOK {
		t.Fatalf("another client: %d, want 200", code)
	}
}
