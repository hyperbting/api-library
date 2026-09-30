package route

import (
	"api-library/internal/app"
	"api-library/internal/config"

	"github.com/gofiber/fiber/v3"
)

func setupWebhookRoutes(router fiber.Router, container *app.Container, cfg *config.WebhookConfig) {
	webhooks := router.Group("/webhooks")
	// webhooks.Post("/", container.UserHdl.CreateUser)

	metaWH := webhooks.Group("/meta")

	if cfg.Meta.Payment.Enabled {
		metaWH.Get("/payments", container.MetaVerifyHdl)
	}

	//metaWH.Post("/payments", container.MetaPaymentMW)
}
