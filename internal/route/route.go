package route

import (
	"api-library/internal/app"
	"api-library/internal/config"
	"fmt"

	"github.com/gofiber/fiber/v3"
)

func RegisterRoutes(app *fiber.App, cfg *config.AppConfig, container *app.Container) {
	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": "ok",
		})
	})

	apiPath := fmt.Sprintf("/api/%s", cfg.App.Version)
	api := app.Group(apiPath)
	setupWebhookRoutes(api, container)

	setupUserRoutes(api, container)
	setupFriendRoutes(api, container)

	if cfg.Firebase.Enabled {
		RegisterFirebaseCloudFunctionRoutes(app, cfg, container.FirebasseContainer)
	}
}

func setupUserRoutes(router fiber.Router, container *app.Container) {
	// users := router.Group("/users")

	// users.Get("/:id", container.UserHdl.GetUser)
	// users.Post("/", container.UserHdl.CreateUser)
}

func setupFriendRoutes(router fiber.Router, container *app.Container) {
	// 	friends := router.Group("/friends")
	//
	// 	friends.Get("/", container.FriendHdl.ListFriends)
	// 	friends.Post("/add", container.FriendHdl.AddFriend)
}
