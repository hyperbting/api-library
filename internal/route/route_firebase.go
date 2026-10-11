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

	mountUserRoutes(api, container.UserHdl, cfg)
	setupQuestRoutes(api, container)
	setupSaveRoutes(api, container)
	setupRedeemRoutes(api, container)
}

func setupSaveRoutes(router fiber.Router, container *app.FirebaseContainer) {
	if container.SaveHdl == nil || container.Firestore == nil {
		return
	}

	saves := router.Group("/save", container.FirebaseAuthMw)
	saves.Get("/load", container.SaveHdl.Load)
	saves.Post("/sync", container.SaveHdl.Sync)
	// Admin only: drops the caller's own save snapshot. Used by the client dev menu.
	saves.Post("/admin/reset", auth.RequireAdmin(auth.FirestoreAdminChecker(container.Firestore)), container.SaveHdl.Reset)
}

func setupRedeemRoutes(router fiber.Router, container *app.FirebaseContainer) {
	if container.RedeemHdl == nil || container.Firestore == nil {
		return
	}

	redeem := router.Group("/redeem", container.FirebaseAuthMw)
	redeem.Post("/use", container.RedeemHdl.Use)

	// Admin only (Firestore users/{uid}.UserRoles includes "admin"): manage codes.
	isAdmin := auth.RequireAdmin(auth.FirestoreAdminChecker(container.Firestore))
	redeem.Get("/admin/codes", isAdmin, container.RedeemHdl.ListCodes)
	redeem.Post("/admin/codes", isAdmin, container.RedeemHdl.CreateCode)
	redeem.Post("/admin/codes/disable", isAdmin, container.RedeemHdl.SetDisabled)
}

func setupQuestRoutes(router fiber.Router, container *app.FirebaseContainer) {
	if container.QuestHdl == nil || container.Firestore == nil {
		return
	}

	quests := router.Group("/quests", container.FirebaseAuthMw)
	quests.Get("/progress", container.QuestHdl.GetProgress)
	quests.Post("/complete-step", container.QuestHdl.CompleteStep)
	// Admin only (Firestore users/{uid}.UserRoles includes "admin"): wipes the caller's own quest progress. Used by the client dev menu.
	quests.Post("/admin/reset", auth.RequireAdmin(auth.FirestoreAdminChecker(container.Firestore)), container.QuestHdl.Reset)
}
