package auth

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestRequireAdmin(t *testing.T) {
	checker := func(_ context.Context, uid string) (bool, error) {
		switch uid {
		case "admin":
			return true, nil
		case "broken":
			return false, errors.New("firestore down")
		}
		return false, nil
	}
	withUser := func(uid string) fiber.Handler {
		return func(c fiber.Ctx) error {
			c.Locals(userCtxKey, UserContext{UserID: uid})
			return c.Next()
		}
	}
	ok := func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }

	app := fiber.New()
	app.Get("/admin", withUser("admin"), RequireAdmin(checker), ok)
	app.Get("/player", withUser("player"), RequireAdmin(checker), ok)
	app.Get("/broken", withUser("broken"), RequireAdmin(checker), ok)
	app.Get("/anon", RequireAdmin(checker), ok)

	for path, want := range map[string]int{"/admin": 200, "/player": 403, "/broken": 500, "/anon": 403} {
		res, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != want {
			t.Fatalf("%s: got %d want %d", path, res.StatusCode, want)
		}
	}
}
