package auth

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestRequireSignInProvider(t *testing.T) {
	withProvider := func(p string) fiber.Handler {
		return func(c fiber.Ctx) error {
			c.Locals(userCtxKey, UserContext{UserID: "u", SignInProvider: p})
			return c.Next()
		}
	}
	ok := func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }

	app := fiber.New()
	// Email only: Google tokens are refused even if the account is an admin
	app.Get("/email-pw", withProvider(ProviderPassword), RequireSignInProvider(ProviderPassword), ok)
	app.Get("/email-google", withProvider(ProviderGoogle), RequireSignInProvider(ProviderPassword), ok)
	app.Get("/both-google", withProvider(ProviderGoogle), RequireSignInProvider(ProviderPassword, ProviderGoogle), ok)
	app.Get("/anon", withProvider("anonymous"), RequireSignInProvider(ProviderPassword, ProviderGoogle), ok)
	app.Get("/none", withProvider(ProviderPassword), RequireSignInProvider(), ok)
	app.Get("/no-user", RequireSignInProvider(ProviderPassword), ok)

	want := map[string]int{
		"/email-pw": 200, "/email-google": 403, "/both-google": 200,
		"/anon": 403, "/none": 403, "/no-user": 403,
	}
	for path, code := range want {
		res, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != code {
			t.Fatalf("%s: got %d want %d", path, res.StatusCode, code)
		}
	}
}
