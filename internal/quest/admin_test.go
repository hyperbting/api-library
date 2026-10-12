package quest

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"api-library/internal/auth"

	"github.com/gofiber/fiber/v3"
)

func adminApp(t *testing.T) (*fiber.App, Service) {
	t.Helper()
	s, _ := newTestService(t)
	h := NewHandler(s)
	app := fiber.New()
	// Stands in for the auth middleware (the same context key auth.MustGetUser reads).
	app.Use(func(c fiber.Ctx) error {
		c.Locals("auth:user", auth.UserContext{UserID: "admin-uid"})
		return c.Next()
	})
	app.Get("/q/catalog", h.AdminCatalog)
	app.Get("/q/progress", h.AdminGet)
	app.Put("/q/progress", h.AdminSet)
	app.Post("/q/progress/reset", h.AdminResetQuest)
	app.Post("/q/reset", h.AdminResetUser)
	return app, s
}

func do(t *testing.T, app *fiber.App, method, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestAdminSetProgressPutsQuestAtStep(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	// A user with no progress: the quest is created at step 2, the steps before it completed.
	p, err := s.AdminSetProgress(ctx, "target", SetProgressRequest{UID: "target", QuestID: "tut_prologue", StepIndex: 2})
	if err != nil {
		t.Fatal(err)
	}
	if p.StepIndex != 2 || p.Completed || len(p.CompletedSteps) != 2 || p.CompletedSteps[0] != "intro" || p.CompletedSteps[1] != "jar" {
		t.Fatalf("set step 2: %+v", p)
	}
	if p.CheckpointIndex != 2 {
		t.Fatalf("step 2 is the staff checkpoint, got checkpoint %d", p.CheckpointIndex)
	}

	view, err := s.AdminGet(ctx, "target")
	if err != nil || !view.Exists || len(view.Quests) != 1 || view.Quests[0].StepIndex != 2 {
		t.Fatalf("view: %+v err=%v", view, err)
	}

	// Back to step 1: only intro stays completed, and the checkpoint falls back to 0.
	p, err = s.AdminSetProgress(ctx, "target", SetProgressRequest{UID: "target", QuestID: "tut_prologue", StepIndex: 1})
	if err != nil {
		t.Fatal(err)
	}
	if p.StepIndex != 1 || len(p.CompletedSteps) != 1 || p.CheckpointIndex != 0 {
		t.Fatalf("back to step 1: %+v", p)
	}
}

func TestAdminCompleteStartsFollowUpOnce(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	p, err := s.AdminSetProgress(ctx, "target", SetProgressRequest{UID: "target", QuestID: "tut_prologue", Completed: true, StepIndex: 99})
	if err != nil {
		t.Fatal(err)
	}
	if !p.Completed || p.CompletedAt == nil || p.StepIndex != 3 || len(p.CompletedSteps) != 4 {
		t.Fatalf("completed: %+v", p)
	}

	view, _ := s.AdminGet(ctx, "target")
	if len(view.Quests) != 2 || view.Quests[0].QuestID != "ch1" || view.Quests[0].Completed {
		t.Fatalf("ch1 should have started: %+v", view.Quests)
	}

	// Move ch1 on, then complete the tutorial again: the follow-up is not restarted.
	if _, err := s.AdminSetProgress(ctx, "target", SetProgressRequest{UID: "target", QuestID: "ch1", Completed: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdminSetProgress(ctx, "target", SetProgressRequest{UID: "target", QuestID: "tut_prologue", Completed: true}); err != nil {
		t.Fatal(err)
	}
	view, _ = s.AdminGet(ctx, "target")
	if !view.Quests[0].Completed {
		t.Fatalf("ch1 was restarted: %+v", view.Quests[0])
	}

	// Re-opening a completed quest clears the completion.
	p, _ = s.AdminSetProgress(ctx, "target", SetProgressRequest{UID: "target", QuestID: "tut_prologue", StepIndex: 0})
	if p.Completed || p.CompletedAt != nil || p.StepIndex != 0 || len(p.CompletedSteps) != 0 {
		t.Fatalf("reopened: %+v", p)
	}
}

func TestAdminSetProgressRejectsBadInput(t *testing.T) {
	s, repo := newTestService(t)
	ctx := context.Background()

	if _, err := s.AdminSetProgress(ctx, "t", SetProgressRequest{QuestID: "nope"}); err != ErrUnknownQuest {
		t.Fatalf("unknown quest: %v", err)
	}
	if _, err := s.AdminSetProgress(ctx, "t", SetProgressRequest{QuestID: "tut_prologue", StepIndex: 4}); err != ErrInvalidStep {
		t.Fatalf("index past the end: %v", err)
	}
	if _, err := s.AdminSetProgress(ctx, "t", SetProgressRequest{QuestID: "tut_prologue", StepIndex: -1}); err != ErrInvalidStep {
		t.Fatalf("negative index: %v", err)
	}
	if repo.writes != 0 {
		t.Fatalf("rejections must not write, got %d", repo.writes)
	}
}

func TestAdminResetQuestAndUser(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	for _, id := range []string{"tut_prologue", "ch1"} {
		if _, err := s.AdminSetProgress(ctx, "t", SetProgressRequest{QuestID: id, Completed: true}); err != nil {
			t.Fatal(err)
		}
	}

	if removed, err := s.AdminResetQuest(ctx, "t", "ch1"); err != nil || !removed {
		t.Fatalf("reset ch1: removed=%v err=%v", removed, err)
	}
	if removed, _ := s.AdminResetQuest(ctx, "t", "ch1"); removed {
		t.Fatal("second reset of ch1 must report nothing removed")
	}
	view, _ := s.AdminGet(ctx, "t")
	if len(view.Quests) != 1 || view.Quests[0].QuestID != "tut_prologue" {
		t.Fatalf("after reset ch1: %+v", view.Quests)
	}

	// Another user is untouched by the edits above, and a user with no document is a no-op.
	if removed, err := s.AdminResetQuest(ctx, "nobody", "ch1"); err != nil || removed {
		t.Fatalf("reset for a user with no document: removed=%v err=%v", removed, err)
	}
	if err := s.Reset(ctx, "t"); err != nil {
		t.Fatal(err)
	}
	if view, _ := s.AdminGet(ctx, "t"); view.Exists || len(view.Quests) != 0 {
		t.Fatalf("after reset all: %+v", view)
	}
}

func TestAdminResetQuestRemovesOrphanedProgress(t *testing.T) {
	s, repo := newTestService(t)
	repo.docs["t"] = &QuestsDoc{Quests: map[string]*Progress{"old_quest": {QuestID: "old_quest"}}}

	view, _ := s.AdminGet(context.Background(), "t")
	if len(view.Quests) != 1 || view.Quests[0].InCatalog {
		t.Fatalf("an orphan must be listed and flagged: %+v", view.Quests)
	}
	if removed, _ := s.AdminResetQuest(context.Background(), "t", "old_quest"); !removed {
		t.Fatal("an orphan can be reset")
	}
}

func TestAdminHandlers(t *testing.T) {
	app, _ := adminApp(t)

	code, body := do(t, app, "GET", "/q/catalog", "")
	var cat struct {
		Version int `json:"version"`
		Quests  []struct {
			ID    string `json:"id"`
			Steps []struct {
				ID string `json:"id"`
			} `json:"steps"`
		} `json:"quests"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &cat) != nil || cat.Version != 7 || len(cat.Quests) != 2 || len(cat.Quests[0].Steps) != 4 {
		t.Fatalf("catalog: %d %s", code, body)
	}

	if code, _ := do(t, app, "GET", "/q/progress", ""); code != 400 {
		t.Fatalf("no uid: %d", code)
	}
	if code, _ := do(t, app, "GET", "/q/progress?uid=a%2Fb", ""); code != 400 {
		t.Fatalf("uid with a slash: %d", code)
	}
	if code, body := do(t, app, "GET", "/q/progress?uid=someone", ""); code != 200 || !strings.Contains(body, `"quests":[]`) {
		t.Fatalf("empty user: %d %s", code, body)
	}

	if code, body := do(t, app, "PUT", "/q/progress", `{"uid":"someone","questId":"tut_prologue","stepIndex":2}`); code != 200 || !strings.Contains(body, `"stepIndex":2`) {
		t.Fatalf("set: %d %s", code, body)
	}
	if code, _ := do(t, app, "PUT", "/q/progress", `{"uid":"someone","questId":"nope","stepIndex":0}`); code != 400 {
		t.Fatalf("unknown quest: %d", code)
	}
	if code, _ := do(t, app, "PUT", "/q/progress", `{"uid":"someone","questId":"tut_prologue","stepIndex":9}`); code != 400 {
		t.Fatalf("step out of range: %d", code)
	}
	if code, _ := do(t, app, "PUT", "/q/progress", `{"questId":"tut_prologue"}`); code != 400 {
		t.Fatalf("missing uid: %d", code)
	}

	if code, body := do(t, app, "POST", "/q/progress/reset", `{"uid":"someone","questId":"tut_prologue"}`); code != 200 || !strings.Contains(body, `"removed":true`) {
		t.Fatalf("reset quest: %d %s", code, body)
	}
	if code, _ := do(t, app, "POST", "/q/progress/reset", `{"uid":"someone"}`); code != 400 {
		t.Fatalf("reset quest without a quest id: %d", code)
	}
	if code, _ := do(t, app, "POST", "/q/reset", `{"uid":"someone"}`); code != 200 {
		t.Fatalf("reset user: %d", code)
	}
	if code, _ := do(t, app, "POST", "/q/reset", `{}`); code != 400 {
		t.Fatalf("reset user without a uid: %d", code)
	}
}
