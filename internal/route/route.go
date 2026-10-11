package route

import (
	"api-library/internal/app"
	"api-library/internal/config"
	"api-library/internal/user"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
)

func RegisterRoutes(app *fiber.App, cfg *config.AppConfig, container *app.Container) {
	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": "ok",
		})
	})

	apiPath := fmt.Sprintf("/api/%s", cfg.App.Version)
	api := app.Group(apiPath)
	setupWebhookRoutes(api, container, cfg.Webhook)

	setupUserRoutes(api, container, cfg)
	setupFriendRoutes(api, container)

	if cfg.FirebaseEnabled() && container.FirebaseContainer != nil {
		RegisterFirebaseCloudFunctionRoutes(app, cfg, container.FirebaseContainer)
	}
}

func setupUserRoutes(router fiber.Router, container *app.Container, cfg *config.AppConfig) {
	mountUserRoutes(router, container.UserHdl, cfg)
}

// mountUserRoutes serves email/password login and register; shared by the full server and the Firebase-only one.
func mountUserRoutes(router fiber.Router, userHdl *user.Handler, cfg *config.AppConfig) {
	if userHdl == nil {
		return
	}

	// Per-IP limits slow down password guessing and mass sign-ups.
	auth := router.Group("/auth")
	// Only failed logins use the budget: a player signing in a few times is not a guesser.
	auth.Post("/login", authLimiter(10, true), userHdl.LoginEmailPassword)

	// Registration is opt-in via app.register_enabled in the config.
	if cfg != nil && cfg.App.RegisterEnabled {
		auth.Post("/register", authLimiter(5, false), userHdl.RegisterEmailPassword)
	}
}

func setupFriendRoutes(router fiber.Router, container *app.Container) {
	// 	friends := router.Group("/friends")
	//
	// 	friends.Get("/", container.FriendHdl.ListFriends)
	// 	friends.Post("/add", container.FriendHdl.AddFriend)
}

// authLimiter allows max requests per minute from one client (see clientKey) on a credentials route.
// With onlyFailures, a response below 400 is not counted. The state is in memory, so with several
// instances the real cap is max per instance.
func authLimiter(max int, onlyFailures bool) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:                    max,
		Expiration:             time.Minute,
		KeyGenerator:           clientKey,
		SkipSuccessfulRequests: onlyFailures,
		LimitReached: func(c fiber.Ctx) error {
			// What the server sees, to check the client address on Cloud Run (no emails or tokens).
			log.Printf("[limit] %s key=%s c.IP()=%s xff=%q", c.Path(), clientKey(c), c.IP(), c.Get(fiber.HeaderXForwardedFor))
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "too many requests"})
		},
	})
}
