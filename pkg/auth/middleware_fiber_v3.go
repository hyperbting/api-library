package auth

import (
	"strings"

	"github.com/gofiber/fiber/v3"
)

// AuthV3Middleware returns a GoFiber v3 middleware handler
func AuthV3Middleware(tm TokenManager) fiber.Handler {
	// Notice: fiber.Ctx is passed by VALUE in v3
	return func(c fiber.Ctx) error {
		// 1. Extract Authorization Header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Missing authorization header",
			})
		}

		// 2. Parse Bearer prefix
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenStr == authHeader {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid token format, expected 'Bearer <token>'",
			})
		}

		// 3. Validate Token via domain service
		claims, err := tm.ValidateAccessToken(tokenStr)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or expired token",
			})
		}

		// 4. Attach claims to context for downstream route handlers
		c.Locals("user_id", claims.UserID)
		c.Locals("roles", claims.Roles)

		// 5. Continue execution flow
		return c.Next()
	}
}
