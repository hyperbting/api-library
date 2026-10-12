package route

import (
	"api-library/internal/adminui"
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

	// Admin panel and admin API: only when ADMIN_ENABLED, so the public service can run the same image without them
	if cfg.Admin.Enabled {
		adminui.Register(app, cfg)
		setupAdminRoutes(api, cfg, container)
	}
}

// setupAdminRoutes mounts the admin API. A request needs a Firebase ID token from an enabled sign-in method
// (Google only when ADMIN_LOGIN_GOOGLE, email/password only when ADMIN_LOGIN_EMAIL) and the admin role.
func setupAdminRoutes(router fiber.Router, cfg *config.AppConfig, container *app.FirebaseContainer) {
	if container.Firestore == nil {
		return
	}
	var providers []string
	if cfg.Admin.LoginEmail {
		providers = append(providers, auth.ProviderPassword)
	}
	if cfg.Admin.LoginGoogle {
		providers = append(providers, auth.ProviderGoogle)
	}

	admin := router.Group("/admin",
		container.FirebaseAuthMw,
		auth.RequireSignInProvider(providers...),
		auth.RequireAdmin(auth.FirestoreAdminChecker(container.Firestore)),
	)
	// The panel calls this after sign-in to confirm the account is an admin.
	admin.Get("/me", func(c fiber.Ctx) error {
		u := auth.MustGetUser(c)
		return c.JSON(fiber.Map{"uid": u.UserID, "provider": u.SignInProvider})
	})
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
