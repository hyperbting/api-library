package route

import (
	"api-library/internal/app"
	"api-library/internal/auth"
	"api-library/internal/config"
	"fmt"

	"github.com/gofiber/fiber/v3"
)

func RegisterFirebaseCloudFunctionRoutes(app *fiber.App, cfg *config.AppConfig, container *app.FirebaseContainer) {
	// container is nil when Firebase is disabled
	if cfg == nil || container == nil {
		return
	}

	apiPath := fmt.Sprintf("/api/%s", cfg.App.Version)
	api := app.Group(apiPath)

	setupQuestRoutes(api, container)
}

func setupQuestRoutes(router fiber.Router, container *app.FirebaseContainer) {
	if container.QuestHdl == nil || container.Firestore == nil {
		return
	}

	quests := router.Group("/quests", container.FirebaseAuthMw)
	quests.Get("/progress", container.QuestHdl.GetProgress)
	quests.Post("/complete-step", container.QuestHdl.CompleteStep)
	// Admin only (a document admins/{uid} in Firestore): wipes the caller's own quest progress. Used by the client dev menu.
	quests.Post("/admin/reset", auth.RequireAdmin(auth.FirestoreAdminChecker(container.Firestore)), container.QuestHdl.Reset)
}
