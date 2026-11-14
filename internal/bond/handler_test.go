package bond

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"api-library/internal/auth"

	"github.com/gofiber/fiber/v3"
)

type stubService struct{ calls int }

func (s *stubService) State(context.Context, string) (StateResponse, error) {
	s.calls++
	return StateResponse{Pets: map[string]PetView{}}, nil
}
func (s *stubService) Report(_ context.Context, _ string, req ReportRequest) (StateResponse, error) {
	s.calls++
	if err := validateReport(req); err != nil {
		return StateResponse{}, err
	}
	return StateResponse{Pets: map[string]PetView{}}, nil
}
func (s *stubService) AdminSet(context.Context, AdminSetRequest) (StateResponse, error) {
	s.calls++
	return StateResponse{Pets: map[string]PetView{}}, nil
}

// asUser stands in for the auth middleware (the same context key auth.GetUser reads).
func asUser(c fiber.Ctx) error {
	c.Locals("auth:user", auth.UserContext{UserID: "u"})
	return c.Next()
}

func newApp(h *Handler) *fiber.App {
	app := fiber.New()
	app.Get("/bond", asUser, h.State)
	app.Post("/bond/report", asUser, h.Report)
	app.Get("/admin/bond", asUser, h.AdminGet)
	app.Put("/admin/bond", asUser, h.AdminSet)
	return app
}

func do(t *testing.T, app *fiber.App, method, path, body string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return res.StatusCode
}

func TestPlayerRoutesAnswer503WhenDisabled(t *testing.T) {
	srv := &stubService{}
	app := newApp(NewHandler(srv, false))
	if got := do(t, app, "GET", "/bond", ""); got != fiber.StatusServiceUnavailable {
		t.Fatalf("GET /bond off: %d, want 503", got)
	}
	if got := do(t, app, "POST", "/bond/report", `{"batchId":"batch-0001","entries":[{"pet":"Pet_Tortoise","seconds":1}]}`); got != fiber.StatusServiceUnavailable {
		t.Fatalf("POST /bond/report off: %d, want 503", got)
	}
	if got := do(t, app, "GET", "/admin/bond?uid=u", ""); got != fiber.StatusOK {
		t.Fatalf("admin GET while off: %d, want 200", got)
	}
	if srv.calls != 1 {
		t.Fatalf("service calls %d, want 1 (only the admin read)", srv.calls)
	}
}

func TestReportRequestChecks(t *testing.T) {
	app := newApp(NewHandler(&stubService{}, true))
	if got := do(t, app, "POST", "/bond/report", `not json`); got != fiber.StatusBadRequest {
		t.Fatalf("bad json: %d, want 400", got)
	}
	if got := do(t, app, "POST", "/bond/report", `{"batchId":"x","entries":[]}`); got != fiber.StatusBadRequest {
		t.Fatalf("invalid batch: %d, want 400", got)
	}
	if got := do(t, app, "POST", "/bond/report", `{"batchId":"batch-0001","entries":[{"pet":"Pet_Tortoise","seconds":30}]}`); got != fiber.StatusOK {
		t.Fatalf("valid report: %d, want 200", got)
	}
	if got := do(t, app, "GET", "/admin/bond", ""); got != fiber.StatusBadRequest {
		t.Fatalf("admin GET without uid: %d, want 400", got)
	}
}
