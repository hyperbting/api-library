package auth

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func registerApp(create EmailUserCreator) *fiber.App {
	app := fiber.New()
	app.Post("/admin/register", RegisterEmailHandler(create))
	return app
}

func postRegister(t *testing.T, app *fiber.App, body string) int {
	t.Helper()
	req := httptest.NewRequest("POST", "/admin/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return res.StatusCode
}

func TestRegisterEmailHandler(t *testing.T) {
	for _, tc := range []struct {
		name     string
		body     string
		create   EmailUserCreator
		wantCode int
	}{
		{"created", `{"email":"ops@example.com","password":"password123"}`,
			func(context.Context, string, string) (string, error) { return "uid-1", nil }, fiber.StatusCreated},
		{"email taken", `{"email":"ops@example.com","password":"password123"}`,
			func(context.Context, string, string) (string, error) { return "", ErrRegistrationEmailTaken }, fiber.StatusConflict},
		{"firebase rejects it", `{"email":"ops@example.com","password":"password123"}`,
			func(context.Context, string, string) (string, error) { return "", ErrRegistrationInvalid }, fiber.StatusBadRequest},
		{"backend failure", `{"email":"ops@example.com","password":"password123"}`,
			func(context.Context, string, string) (string, error) { return "", errors.New("boom") }, fiber.StatusInternalServerError},
		{"not an email", `{"email":"nope","password":"password123"}`, nil, fiber.StatusBadRequest},
		{"short password", `{"email":"ops@example.com","password":"short"}`, nil, fiber.StatusBadRequest},
		{"not json", `email=ops`, nil, fiber.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			create := tc.create
			if create == nil {
				create = func(context.Context, string, string) (string, error) {
					t.Fatal("create must not be called for an invalid request")
					return "", nil
				}
			}
			if got := postRegister(t, registerApp(create), tc.body); got != tc.wantCode {
				t.Fatalf("got %d want %d", got, tc.wantCode)
			}
		})
	}
}
