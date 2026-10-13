package route

import (
	"api-library/internal/adminui"
	"api-library/internal/app"
	"api-library/internal/auth"
	"api-library/internal/config"
	"api-library/internal/news"
	"api-library/internal/quest"
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
	setupNewsRoutes(api, container)
	setupSaveRoutes(api, container)
	setupRedeemRoutes(api, container)

	// Admin API (ADMIN_ENABLED) and, on top of it, the embedded panel (ADMIN_PANEL_ENABLED). Both are off on the
	// public service, which runs the same image.
	if cfg.Admin.Enabled {
		// Registered before the /admin group below: it is public (no token yet), the group's middleware is not.
		mountAdminRegister(api, cfg, container.EmailUserCreator)
		setupAdminRoutes(api, cfg, container)
		if cfg.Admin.PanelEnabled {
			adminui.Register(app, cfg)
		}
	}
}

// mountAdminRegister serves POST /admin/register when ADMIN_REGISTER_EMAIL is on (and email sign-in is, since the
// account it creates can only sign in with email/password). It creates the sign-in account and nothing else:
// the admin role is still set by hand on users/{uid}. Per-IP rate limited like the player register route.
func mountAdminRegister(router fiber.Router, cfg *config.AppConfig, create auth.EmailUserCreator) {
	if cfg == nil || create == nil || !cfg.Admin.RegisterEmail || !cfg.Admin.LoginEmail {
		return
	}
	router.Post("/admin/register", authLimiter(5, false), auth.RegisterEmailHandler(create))
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

	mountAdminQuestRoutes(admin, container.QuestHdl)
	mountAdminNewsRoutes(admin, container.NewsHdl)
}

// setupNewsRoutes: GET /news for any signed-in player (public items; the dev items too for an admin account).
func setupNewsRoutes(router fiber.Router, container *app.FirebaseContainer) {
	if container.NewsHdl == nil || container.Firestore == nil {
		return
	}
	router.Get("/news", container.FirebaseAuthMw, container.NewsHdl.List)
}

// mountAdminNewsRoutes: the panel's News page. The router must already demand the admin role.
func mountAdminNewsRoutes(admin fiber.Router, h *news.Handler) {
	if h == nil {
		return
	}
	admin.Get("/news", h.AdminList)
	admin.Post("/news", h.AdminSave)
	admin.Post("/news/delete", h.AdminDelete)
}

// mountAdminQuestRoutes: view, edit and reset any user's quest progress by uid (panel page "Quests").
// The router must already demand the admin role.
func mountAdminQuestRoutes(admin fiber.Router, h *quest.Handler) {
	if h == nil {
		return
	}
	q := admin.Group("/quests")
	q.Get("/catalog", h.AdminCatalog)
	q.Get("/progress", h.AdminGet)
	q.Put("/progress", h.AdminSet)
	q.Post("/progress/reset", h.AdminResetQuest)
	q.Post("/reset", h.AdminResetUser)
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
