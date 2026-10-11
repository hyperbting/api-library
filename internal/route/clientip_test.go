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

func TestLastForwardedIP(t *testing.T) {
	cases := map[string]string{
		"":                               "",
		"203.0.113.9":                    "203.0.113.9",
		"1.2.3.4, 203.0.113.9":           "203.0.113.9",
		"1.2.3.4,203.0.113.9":            "203.0.113.9",
		" 2001:db8::1 ":                  "2001:db8::1",
		"203.0.113.9, not-an-ip":         "", // the last entry decides, earlier ones are never trusted
		"203.0.113.9, ":                  "",
		"spoofed, 198.51.100.7":          "198.51.100.7",
		"10.0.0.1, 10.0.0.2, 192.0.2.44": "192.0.2.44",
	}
	for in, want := range cases {
		if got := lastForwardedIP(in); got != want {
			t.Errorf("lastForwardedIP(%q) = %q, want %q", in, got, want)
		}
	}
}

// loginSvc succeeds for the password "good" and fails for anything else.
type loginSvc struct {
	user.Service
}

func (loginSvc) LoginEmailPassword(_ context.Context, _, password, _ string) (*user.User, *session.AccessToken, error) {
	if password != "good" {
		return nil, nil, user.ErrInvalidCredentials
	}
	return &user.User{}, &session.AccessToken{Token: "t"}, nil
}

func loginApp() *fiber.App {
	fiberApp := fiber.New()
	cfg := &config.AppConfig{App: config.AppDetail{}}
	setupUserRoutes(fiberApp.Group("/api/v99"), &app.Container{UserHdl: user.NewHandler(loginSvc{})}, cfg)
	return fiberApp
}

func login(t *testing.T, a *fiber.App, password, forwardedFor string) int {
	t.Helper()
	body := `{"email":"player@example.com","password":"` + password + `"}`
	req := httptest.NewRequest("POST", "/api/v99/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	res, err := a.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return res.StatusCode
}

func TestLoginLimiterCountsOnlyFailures(t *testing.T) {
	a := loginApp()
	for i := 0; i < 30; i++ {
		if got := login(t, a, "good", "198.51.100.1"); got != fiber.StatusOK {
			t.Fatalf("successful login %d got %d", i, got)
		}
	}

	var last int
	for i := 0; i < 11; i++ {
		last = login(t, a, "bad", "198.51.100.1")
	}
	if last != fiber.StatusTooManyRequests {
		t.Fatalf("11th failed login got %d, want 429", last)
	}
}

func TestLoginLimiterIsPerForwardedClient(t *testing.T) {
	a := loginApp()
	for i := 0; i < 11; i++ {
		login(t, a, "bad", "198.51.100.1")
	}
	if got := login(t, a, "bad", "198.51.100.1"); got != fiber.StatusTooManyRequests {
		t.Fatalf("blocked client got %d, want 429", got)
	}

	// Another client behind the same front end is not affected.
	if got := login(t, a, "bad", "198.51.100.2"); got != fiber.StatusUnauthorized {
		t.Fatalf("other client got %d, want 401", got)
	}
	// A client cannot reset its budget by writing its own entries in front of the real one.
	if got := login(t, a, "bad", "203.0.113.77, 198.51.100.1"); got != fiber.StatusTooManyRequests {
		t.Fatalf("spoofed prefix got %d, want 429", got)
	}
}
