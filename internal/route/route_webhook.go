package route

import (
	"api-library/internal/app"

	"github.com/gofiber/fiber/v3"
)

func setupWebhookRoutes(router fiber.Router, container *app.Container) {
	webhooks := router.Group("/webhooks")
	// webhooks.Post("/", container.UserHdl.CreateUser)

	metaWH := webhooks.Group("/meta")
	metaWH.Get("/payments", container.MetaVerifyHdl)
	//metaWH.Post("/payments", container.MetaPaymentMW)
}
