package auth

import (
	"context"
	"errors"
	"fmt"
	"log"

	"api-library/pkg/validator"

	fbauth "firebase.google.com/go/v4/auth"
	"github.com/gofiber/fiber/v3"
)

var (
	// ErrRegistrationEmailTaken: a Firebase Auth user with this email already exists.
	ErrRegistrationEmailTaken = errors.New("email already registered")
	// ErrRegistrationInvalid: Firebase rejected the email or password.
	ErrRegistrationInvalid = errors.New("invalid email or password")
)

// EmailUserCreator creates a Firebase Auth email/password user and returns its uid. It never grants a role:
// admin stays a manual UserRoles entry on users/{uid} (see admin.go).
type EmailUserCreator func(ctx context.Context, email, password string) (uid string, err error)

// NewFirebaseEmailUserCreator creates users through the Firebase Admin SDK. This is the account the admin
// panel signs in with (Firebase Auth email/password), not the player email accounts stored in Firestore.
func NewFirebaseEmailUserCreator(client *fbauth.Client) EmailUserCreator {
	return func(ctx context.Context, email, password string) (string, error) {
		params := (&fbauth.UserToCreate{}).Email(email).Password(password).EmailVerified(false)
		u, err := client.CreateUser(ctx, params)
		switch {
		case err == nil:
			return u.UID, nil
		case fbauth.IsEmailAlreadyExists(err):
			return "", ErrRegistrationEmailTaken
		case fbauth.IsInvalidEmail(err):
			return "", ErrRegistrationInvalid
		case fbauth.IsInsufficientPermission(err):
			// Not the caller's fault: the Cloud Run runtime service account lacks firebaseauth.users.create.
			return "", fmt.Errorf("the runtime service account cannot create Firebase Auth users "+
				"(grant it roles/firebaseauth.admin): %w", err)
		default:
			return "", err
		}
	}
}

// RegisterEmailRequest is the payload of POST /admin/register.
type RegisterEmailRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

// RegisterEmailHandler serves POST /admin/register: it creates the sign-in account only. The caller still
// needs the admin role before any /admin route lets it in, so this grants nothing by itself.
func RegisterEmailHandler(create EmailUserCreator) fiber.Handler {
	return func(c fiber.Ctx) error {
		var req RegisterEmailRequest
		if err := c.Bind().JSON(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
		}
		if err := validator.Struct(req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request", "detail": err.Error()})
		}

		uid, err := create(c.Context(), req.Email, req.Password)
		switch {
		case err == nil:
			// The uid is what the operator puts the admin role on (Firestore users/{uid}).
			log.Printf("[admin] registered sign-in account uid=%s (no role granted)", uid)
			return c.Status(fiber.StatusCreated).JSON(fiber.Map{"uid": uid, "email": req.Email})
		case errors.Is(err, ErrRegistrationEmailTaken):
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": ErrRegistrationEmailTaken.Error()})
		case errors.Is(err, ErrRegistrationInvalid):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrRegistrationInvalid.Error()})
		default:
			log.Printf("[admin] register email=%s: %v", req.Email, err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "registration failed"})
		}
	}
}
