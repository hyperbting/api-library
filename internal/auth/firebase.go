package auth

import (
	"strings"

	"firebase.google.com/go/v4/auth"
	"github.com/gofiber/fiber/v3"
)

type FirebaseAuthProvider struct {
	client *auth.Client
}

func NewFirebaseAuthProvider(client *auth.Client) (*FirebaseAuthProvider, error) {
	return &FirebaseAuthProvider{client: client}, nil
}

func (p *FirebaseAuthProvider) Middleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": ErrMissingAuthHeader.Error()})
		}

		idToken := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := p.client.VerifyIDToken(c.Context(), idToken)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": ErrInvalidFirebaseToken.Error()})
		}

		// 將 Firebase UID 放入 Context
		c.Locals("userID", token.UID)
		return c.Next()
	}
}
