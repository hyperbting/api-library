package auth

import "github.com/gofiber/fiber/v3"

// RequireRole rejects (403) users whose roles (from the access token claims,
// attached to the context by the auth middleware) do not include any of the
// allowed roles. It performs no DB access.
//
// It is a single parameterized factory rather than one middleware per role, so
// new roles need no new middleware: RequireRole("moderator", "admin").
//
// Mount it after an auth middleware that populates the user context, e.g.:
//
//	group := api.Group("/admin", container.SessionAuthMw, container.SessionAdminAuthMw)
func RequireRole(allowed ...string) fiber.Handler {
	return func(c fiber.Ctx) error {
		user, ok := GetUser(c)
		if !ok {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}

		for _, r := range allowed {
			if user.HasRole(r) {
				return c.Next()
			}
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
	}
}
