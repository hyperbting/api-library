package meta

import "github.com/gofiber/fiber/v3"

func NewMetaWebhookVerifyHandler(verifyToken string) fiber.Handler {
	return func(c fiber.Ctx) error {

		if c.Method() != fiber.MethodGet {
			return c.Status(fiber.StatusMethodNotAllowed).SendString("Method Not Allowed")
		}

		mode := c.Query("hub.mode")
		token := c.Query("hub.verify_token")
		challenge := c.Query("hub.challenge")

		// empty verifyToken must never match an empty hub.verify_token
		if mode == "subscribe" && verifyToken != "" && token == verifyToken {
			// 成功：回傳 200 並直接輸出 challenge 純文字
			return c.Status(fiber.StatusOK).SendString(challenge)
		}

		// 驗證失敗
		return c.Status(fiber.StatusForbidden).SendString(ErrVerifyTokenMismatched.Error())
	}
}
