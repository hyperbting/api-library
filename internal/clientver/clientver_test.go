package clientver

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.0", "1.1", true},
		{"1.1", "1.0", false},
		{"1.0", "1.0", false},
		{"1.0", "1.0.0", false},
		{"1.9", "1.10", true}, // number by number, not text
		{"0.9.9", "1.0", true},
		{"v1.0-beta", "1.0", false},
		{"", "1.0", false},
		{"abc", "1.0", false},
		{"1.0", "", false},
	}
	for _, c := range cases {
		if got := Less(c.a, c.b); got != c.want {
			t.Errorf("Less(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestMiddleware(t *testing.T) {
	status := func(min, header string) int {
		app := fiber.New()
		app.Use(Middleware(min))
		app.Get("/x", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
		req := httptest.NewRequest("GET", "/x", nil)
		if header != "" {
			req.Header.Set(Header, header)
		}
		res, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode
	}

	if got := status("1.1", "1.0"); got != fiber.StatusUpgradeRequired {
		t.Errorf("old client: %d", got)
	}
	if got := status("1.1", "1.1"); got != fiber.StatusOK {
		t.Errorf("equal client: %d", got)
	}
	if got := status("1.1", "1.2"); got != fiber.StatusOK {
		t.Errorf("newer client: %d", got)
	}
	// Fail open: no header, a header that does not parse, no minimum, a minimum that does not parse.
	for name, got := range map[string]int{
		"no header":   status("1.1", ""),
		"bad header":  status("1.1", "banana"),
		"no minimum":  status("", "0.1"),
		"bad minimum": status("soon", "0.1"),
	} {
		if got != fiber.StatusOK {
			t.Errorf("%s: %d", name, got)
		}
	}
}
