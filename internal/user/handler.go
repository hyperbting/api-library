package user

import (
	"errors"
	"log"
	"time"

	"api-library/pkg/validator"

	"github.com/gofiber/fiber/v3"
)

// Handler serves the user/auth HTTP API.
type Handler struct {
	srv Service
}

func NewHandler(srv Service) *Handler {
	return &Handler{srv: srv}
}

// LoginRequest is the payload for email/password login.
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// RegisterRequest is the payload for email/password registration.
type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

// AuthResponse returns the authenticated user and a standalone access token.
// Email players are stateless: no refresh token is issued.
type AuthResponse struct {
	User      *User     `json:"user"`
	Token     string    `json:"access_token"`
	TokenType string    `json:"token_type"`
	ExpiresAt time.Time `json:"expires_at"`
}

// POST /auth/login
func (h *Handler) LoginEmailPassword(c fiber.Ctx) error {
	var req LoginRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	if err := validator.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error(), "detail": err.Error()})
	}

	usr, at, err := h.srv.LoginEmailPassword(c.Context(), req.Email, req.Password, c.IP())
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": ErrInvalidCredentials.Error()})
		}
		log.Printf("[user] login email=%s: %v", req.Email, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "login failed"})
	}

	return c.JSON(AuthResponse{
		User:      usr,
		Token:     at.Token,
		TokenType: "Bearer",
		ExpiresAt: at.ExpiresAt,
	})
}

// POST /auth/register
func (h *Handler) RegisterEmailPassword(c fiber.Ctx) error {
	var req RegisterRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	if err := validator.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error(), "detail": err.Error()})
	}

	usr, at, err := h.srv.RegisterEmailPassword(c.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrEmailAlreadyExists) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": ErrEmailAlreadyExists.Error()})
		}
		log.Printf("[user] register email=%s: %v", req.Email, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "registration failed"})
	}

	return c.Status(fiber.StatusCreated).JSON(AuthResponse{
		User:      usr,
		Token:     at.Token,
		TokenType: "Bearer",
		ExpiresAt: at.ExpiresAt,
	})
}
