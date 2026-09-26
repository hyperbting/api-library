package route

import (
	"api-library/internal/app"
	"api-library/internal/config"

	"github.com/gofiber/fiber/v3"
)

func RegisterFirebaseCloudFunctionRoutes(app *fiber.App, cfg *config.AppConfig, container *app.FirebasseContainer) {
	// app.Get("/healthz", func(c fiber.Ctx) error {
	// 	return c.Status(fiber.StatusOK).JSON(fiber.Map{
	// 		"status": "ok",
	// 	})
	// })

	//	apiPath := fmt.Sprintf("/api/%s", cfg.App.Version)
	//	api := app.Group(apiPath)

}
