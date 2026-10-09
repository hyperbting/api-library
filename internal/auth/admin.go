package auth

import (
	"context"
	"log"

	"cloud.google.com/go/firestore"
	"github.com/gofiber/fiber/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Admin lives on the player's profile document users/{uid}, field UserRoles (an array that includes "admin").
// Grant or revoke it by hand in the Firebase console (Firestore > users > <uid>); no deploy and no token refresh.
// Firestore rules make users/{uid} read-only for clients, so a player cannot give themselves the role.
const (
	usersCollection = "users"
	userRolesField  = "UserRoles"
	adminRole       = "admin"
)

// AdminChecker reports whether a user is an admin.
type AdminChecker func(ctx context.Context, uid string) (bool, error)

// FirestoreAdminChecker reads users/{uid}.UserRoles and looks for "admin".
func FirestoreAdminChecker(client *firestore.Client) AdminChecker {
	return func(ctx context.Context, uid string) (bool, error) {
		snap, err := client.Collection(usersCollection).Doc(uid).Get(ctx)
		if status.Code(err) == codes.NotFound {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return hasAdminRole(snap.Data()), nil
	}
}

func hasAdminRole(data map[string]any) bool {
	roles, _ := data[userRolesField].([]any)
	for _, r := range roles {
		if name, ok := r.(string); ok && name == adminRole {
			return true
		}
	}
	return false
}

// RequireAdmin rejects (403) users who are not admins. Mount it after the auth middleware.
// The check runs per request (admin routes are rare), so a revoke takes effect at once.
func RequireAdmin(isAdmin AdminChecker) fiber.Handler {
	return func(c fiber.Ctx) error {
		user, ok := GetUser(c)
		if !ok {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}
		admin, err := isAdmin(c.Context(), user.UserID)
		if err != nil {
			log.Printf("[auth] admin check uid=%s: %v", user.UserID, err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "admin check failed"})
		}
		if !admin {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}
		return c.Next()
	}
}
