package auth

import (
	"context"
	"log"

	"cloud.google.com/go/firestore"
	"github.com/gofiber/fiber/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AdminsCollection holds one empty-ish document per admin, keyed by Firebase UID. Add or delete a document
// in the Firebase console (Firestore > admins) to grant or revoke admin; no deploy and no token refresh needed.
// Firestore rules deny every client access to it; only this server reads it.
const AdminsCollection = "admins"

// AdminChecker reports whether a user is an admin.
type AdminChecker func(ctx context.Context, uid string) (bool, error)

// FirestoreAdminChecker treats the existence of admins/{uid} as admin.
func FirestoreAdminChecker(client *firestore.Client) AdminChecker {
	return func(ctx context.Context, uid string) (bool, error) {
		_, err := client.Collection(AdminsCollection).Doc(uid).Get(ctx)
		if status.Code(err) == codes.NotFound {
			return false, nil
		}
		return err == nil, err
	}
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
