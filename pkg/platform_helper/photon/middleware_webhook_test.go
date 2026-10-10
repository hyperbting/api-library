package photon

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

type testEvent struct {
	GameId string `json:"GameId" validate:"required"`
}

func newTestApp() *fiber.App {
	app := fiber.New()
	app.Post("/hook", NewPhotonWebhookMiddleware[testEvent](nil), func(c fiber.Ctx) error {
		ev, err := GetEvent[testEvent](c)
		if err != nil {
			return err
		}
		return c.SendString(ev.GameId)
	})
	return app
}

func doPost(t *testing.T, body string) string {
	t.Helper()
	req := httptest.NewRequest("POST", "/hook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := newTestApp().Test(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestWebhookMiddleware(t *testing.T) {
	if got := doPost(t, `{"GameId":"g1"}`); got != "g1" {
		t.Fatalf("valid: got %q", got)
	}
	if got := doPost(t, `{"GameId":""}`); !strings.Contains(got, ErrInvalidPayload.Error()) {
		t.Fatalf("invalid: got %q", got)
	}
	if got := doPost(t, `{bad`); !strings.Contains(got, ErrInvalidJSONPayload.Error()) {
		t.Fatalf("bad json: got %q", got)
	}
}
