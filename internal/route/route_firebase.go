package route

import (
	"api-library/internal/adminui"
	"api-library/internal/app"
	"api-library/internal/auth"
	"api-library/internal/clientver"
	"api-library/internal/config"
	"api-library/internal/gem"
	"api-library/internal/iap"
	"api-library/internal/news"
	"api-library/internal/quest"
	"fmt"

	"github.com/gofiber/fiber/v3"
)

func RegisterFirebaseCloudFunctionRoutes(app *fiber.App, cfg *config.AppConfig, container *app.FirebaseContainer) {
	// Which commit is running, readable over HTTP. Not /healthz: the Cloud Run service only mounts these routes.
	// Also says which client versions are supported (forced update): unauthenticated, read by the client's Splash.
	app.Get("/version", versionHandler(cfg))

	// container is nil when Firebase is disabled
	if cfg == nil || container == nil {
		return
	}

	apiPath := fmt.Sprintf("/api/%s", cfg.App.Version)
	// A client below CLIENT_MIN_VERSION gets 426 on every API call (sessions left open past a raised minimum).
	api := app.Group(apiPath, clientver.Middleware(cfg.Client.MinVersion))

	mountUserRoutes(api, container.UserHdl, cfg)
	setupQuestRoutes(api, container)
	setupNewsRoutes(api, container)
	setupSaveRoutes(api, container)
	setupRedeemRoutes(api, container)
	setupGemRoutes(api, container)
	setupPetRoutes(api, container)

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

// adminSignInProviders is the list of Firebase sign-in methods the admin routes accept: email/password and Google
// follow the panel's switches, and a custom-token session is accepted only with ADMIN_ACCEPT_CUSTOM_TOKEN.
func adminSignInProviders(cfg config.AdminConfig) []string {
	var providers []string
	if cfg.LoginEmail {
		providers = append(providers, auth.ProviderPassword)
	}
	if cfg.LoginGoogle {
		providers = append(providers, auth.ProviderGoogle)
	}
	if cfg.AcceptCustomToken {
		providers = append(providers, auth.ProviderCustom)
	}
	return providers
}

// setupAdminRoutes mounts the admin API. A request needs a Firebase ID token from an enabled sign-in method
// (Google only when ADMIN_LOGIN_GOOGLE, email/password only when ADMIN_LOGIN_EMAIL, a signed custom token only when
// ADMIN_ACCEPT_CUSTOM_TOKEN) and the admin role.
func setupAdminRoutes(router fiber.Router, cfg *config.AppConfig, container *app.FirebaseContainer) {
	if container.Firestore == nil {
		return
	}
	admin := router.Group("/admin",
		container.FirebaseAuthMw,
		auth.RequireSignInProvider(adminSignInProviders(cfg.Admin)...),
		auth.RequireAdmin(auth.FirestoreAdminChecker(container.Firestore)),
	)
	// The panel calls this after sign-in to confirm the account is an admin.
	admin.Get("/me", func(c fiber.Ctx) error {
		u := auth.MustGetUser(c)
		return c.JSON(fiber.Map{"uid": u.UserID, "provider": u.SignInProvider})
	})

	mountAdminQuestRoutes(admin, container.QuestHdl)
	mountAdminNewsRoutes(admin, container.NewsHdl)
	mountAdminGemRoutes(admin, container.GemHdl, container.IapHdl)
}

// mountAdminGemRoutes: the panel's Gems page. A player's balance, spends and grants by uid, a gift of gems or
// items, and the newest store orders of everybody. The router must already demand the admin role.
func mountAdminGemRoutes(admin fiber.Router, gems *gem.Handler, iaps *iap.Handler) {
	if gems != nil {
		admin.Get("/gems/user", gems.AdminUser)
		admin.Post("/gems/gift", userLimiter(30), gems.AdminGift)
	}
	if iaps != nil {
		admin.Get("/iap/orders", iaps.AdminOrders)
	}
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

// setupPetRoutes: the owned-pet list (GET /pets), rebuilt from the grants ledger.
func setupPetRoutes(router fiber.Router, container *app.FirebaseContainer) {
	if container.Firestore == nil || container.PetHdl == nil {
		return
	}
	router.Get("/pets", container.FirebaseAuthMw, userLimiter(60), container.PetHdl.List)
}

// setupGemRoutes: the gem balance and what it buys, and the store purchases that fill it (GDD 5.2.1).
func setupGemRoutes(router fiber.Router, container *app.FirebaseContainer) {
	if container.Firestore == nil {
		return
	}
	if container.GemHdl != nil {
		gems := router.Group("/gems", container.FirebaseAuthMw)
		// Per player and minute. A spend is a tap on a shop button, a purchase one store flow; the boost report
		// comes about once a minute while the player is in a store.
		gems.Get("/", userLimiter(60), container.GemHdl.State)
		gems.Post("/spend", userLimiter(20), container.GemHdl.Spend)
		gems.Post("/boost/consume", userLimiter(30), container.GemHdl.ConsumeBoost)
		// A rewarded ad (fever, offline double, a warp hour): one call per ad shown.
		router.Post("/ads/reward", container.FirebaseAuthMw, userLimiter(30), container.GemHdl.AdReward)
	}
	if container.IapHdl != nil {
		router.Post("/iap/purchase", container.FirebaseAuthMw, userLimiter(10), container.IapHdl.Purchase)
	}
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
