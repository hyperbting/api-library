package middleware

import (
	"api-library/pkg/session"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// AuthMiddleware returns a GoFiber v3 middleware handler
func AuthMiddleware(tm session.TokenManager) fiber.Handler {
	// Notice: fiber.Ctx is passed by VALUE in v3
	return func(c fiber.Ctx) error {
		// 1. Extract Authorization Header
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": ErrMissingAuthHeader.Error(),
			})
		}

		// 2. Parse Bearer prefix
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenStr == authHeader {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": ErrInvalidTokenFormat.Error(),
			})
		}

		// 3. Validate Token via domain service
		claims, err := tm.ValidateAccessToken(tokenStr)
		if err != nil {
			msg := "Invalid or expired token"
			if errors.Is(err, session.ErrTokenRevoked) {
				msg = "Token has been revoked"
			}
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": msg,
			})
		}

		// 4. Attach claims to context for downstream route handlers
		c.Locals("user_id", claims.UserID)
		c.Locals("roles", claims.Roles)

		// 5. Continue execution flow
		return c.Next()
	}
}
