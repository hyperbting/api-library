package route

import (
	"api-library/internal/app"
	"api-library/internal/config"
	"fmt"
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
	if container.UserHdl == nil {
		return
	}

	// Per-IP limits slow down password guessing and mass sign-ups.
	auth := router.Group("/auth")
	auth.Post("/login", authLimiter(10), container.UserHdl.LoginEmailPassword)

	// Registration is opt-in via app.register_enabled in the config.
	if cfg != nil && cfg.App.RegisterEnabled {
		auth.Post("/register", authLimiter(5), container.UserHdl.RegisterEmailPassword)
	}
}

func setupFriendRoutes(router fiber.Router, container *app.Container) {
	// 	friends := router.Group("/friends")
	//
	// 	friends.Get("/", container.FriendHdl.ListFriends)
	// 	friends.Post("/add", container.FriendHdl.AddFriend)
}

// authLimiter allows max requests per minute from one IP on a credentials route.
func authLimiter(max int) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        max,
		Expiration: time.Minute,
		LimitReached: func(c fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": "too many requests"})
		},
	})
}
