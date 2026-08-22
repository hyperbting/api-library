package auth

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

func FiberMiddleware(tm TokenManager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenStr == "" || tokenStr == authHeader {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing or malformed token"})
		}

		claims, err := tm.ValidateAccessToken(tokenStr)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token"})
		}

		// Pass parsed claims down to handlers via context
		c.Locals("user_id", claims.UserID)
		c.Locals("roles", claims.Roles)

		return c.Next()
	}
}
