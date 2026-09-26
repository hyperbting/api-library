package auth

import (
	"api-library/pkg/session"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
)

type CustomSessionProvider struct {
	tm session.TokenManager
}

func NewCustomSessionProvider(tm session.TokenManager) *CustomSessionProvider {
	return &CustomSessionProvider{tm: tm}
}

func (p *CustomSessionProvider) Middleware() fiber.Handler {
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
		claims, err := p.tm.ValidateAccessToken(tokenStr)
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
		roleMap := make(map[string]struct{}, len(claims.Roles))
		for _, r := range claims.Roles {
			roleMap[r] = struct{}{}
		}
		c.Locals(userCtxKey, UserContext{
			UserID:   claims.UserID,
			Roles:    claims.Roles,
			rolesMap: roleMap,
		})

		// 5. Continue execution flow
		return c.Next()
	}
}

type UserContext struct {
	UserID   string
	Roles    []string            // keeps original order / serialization
	rolesMap map[string]struct{} // internal O(1) set
}

func (u UserContext) HasRole(role string) bool {
	if u.rolesMap == nil {
		for _, r := range u.Roles {
			if r == role {
				return true
			}
		}
		return false
	}
	_, exists := u.rolesMap[role]
	return exists
}

const userCtxKey = "auth:user"

// GetUser extracts the user context from fiber.Ctx
func GetUser(c fiber.Ctx) (UserContext, bool) {
	val, ok := c.Locals(userCtxKey).(UserContext)
	return val, ok
}

// MustGetUser extracts the user context or panics/fails if not found
func MustGetUser(c fiber.Ctx) UserContext {
	user, ok := GetUser(c)
	if !ok {
		panic("user context not found in request")
	}
	return user
}
