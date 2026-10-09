package user_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"api-library/internal/user"
	"api-library/pkg/session"

	"github.com/gofiber/fiber/v3"
)

// fakeService embeds the Service interface so only the methods under test need
// stubbing; any other call would panic on the nil embedded interface.
type fakeService struct {
	user.Service
	register func(ctx context.Context, email, password string) (*user.User, *session.AccessToken, error)
	login    func(ctx context.Context, email, password, ip string) (*user.User, *session.AccessToken, error)
}

func (f *fakeService) RegisterEmailPassword(ctx context.Context, email, password string) (*user.User, *session.AccessToken, error) {
	return f.register(ctx, email, password)
}

func (f *fakeService) LoginEmailPassword(ctx context.Context, email, password, ip string) (*user.User, *session.AccessToken, error) {
	return f.login(ctx, email, password, ip)
}

func postJSON(t *testing.T, app *fiber.App, path, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return res
}

func TestRegisterEmailPassword_Success(t *testing.T) {
	svc := &fakeService{
		register: func(_ context.Context, email, password string) (*user.User, *session.AccessToken, error) {
			return &user.User{}, &session.AccessToken{Token: "access-token"}, nil
		},
	}
	app := fiber.New()
	app.Post("/auth/register", user.NewHandler(svc).RegisterEmailPassword)

	res := postJSON(t, app, "/auth/register", `{"email":"player@example.com","password":"password123"}`)
	if res.StatusCode != fiber.StatusCreated {
		t.Fatalf("got %d want %d", res.StatusCode, fiber.StatusCreated)
	}
}

func TestRegisterEmailPassword_DuplicateEmail(t *testing.T) {
	svc := &fakeService{
		register: func(_ context.Context, _, _ string) (*user.User, *session.AccessToken, error) {
			return nil, nil, user.ErrEmailAlreadyExists
		},
	}
	app := fiber.New()
	app.Post("/auth/register", user.NewHandler(svc).RegisterEmailPassword)

	res := postJSON(t, app, "/auth/register", `{"email":"player@example.com","password":"password123"}`)
	if res.StatusCode != fiber.StatusConflict {
		t.Fatalf("got %d want %d", res.StatusCode, fiber.StatusConflict)
	}
}

// Invalid input must be rejected before the service is called: the stub returns
// an error that would surface as 500, so a 400 proves validation short-circuited.
func TestRegisterEmailPassword_Validation(t *testing.T) {
	svc := &fakeService{
		register: func(_ context.Context, _, _ string) (*user.User, *session.AccessToken, error) {
			return nil, nil, errors.New("service should not be called")
		},
	}
	app := fiber.New()
	app.Post("/auth/register", user.NewHandler(svc).RegisterEmailPassword)

	for name, body := range map[string]string{
		"short password": `{"email":"player@example.com","password":"short"}`,
		"invalid email":  `{"email":"not-an-email","password":"password123"}`,
		"missing fields": `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			res := postJSON(t, app, "/auth/register", body)
			if res.StatusCode != fiber.StatusBadRequest {
				t.Fatalf("got %d want %d", res.StatusCode, fiber.StatusBadRequest)
			}
		})
	}
}

func TestLoginEmailPassword_Success(t *testing.T) {
	svc := &fakeService{
		login: func(_ context.Context, _, _, _ string) (*user.User, *session.AccessToken, error) {
			return &user.User{}, &session.AccessToken{Token: "access-token"}, nil
		},
	}
	app := fiber.New()
	app.Post("/auth/login", user.NewHandler(svc).LoginEmailPassword)

	res := postJSON(t, app, "/auth/login", `{"email":"player@example.com","password":"password123"}`)
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("got %d want %d", res.StatusCode, fiber.StatusOK)
	}
}

func TestLoginEmailPassword_InvalidCredentials(t *testing.T) {
	svc := &fakeService{
		login: func(_ context.Context, _, _, _ string) (*user.User, *session.AccessToken, error) {
			return nil, nil, user.ErrInvalidCredentials
		},
	}
	app := fiber.New()
	app.Post("/auth/login", user.NewHandler(svc).LoginEmailPassword)

	res := postJSON(t, app, "/auth/login", `{"email":"player@example.com","password":"password123"}`)
	if res.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("got %d want %d", res.StatusCode, fiber.StatusUnauthorized)
	}
}
