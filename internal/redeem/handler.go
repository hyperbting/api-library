package redeem

import (
	"errors"
	"log"

	"api-library/internal/auth"
	"api-library/pkg/validator"

	"github.com/gofiber/fiber/v3"
)

// Handler serves the redeem API. Routes must sit behind the auth middleware; the admin ones also behind
// auth.RequireAdmin. A refused code is HTTP 200 with ok=false (the client treats non-2xx as a network failure).
type Handler struct {
	srv Service
}

func NewHandler(srv Service) *Handler {
	return &Handler{srv: srv}
}

// POST /redeem/use
func (h *Handler) Use(c fiber.Ctx) error {
	user := auth.MustGetUser(c)

	var req UseRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	if err := validator.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}

	res, err := h.srv.Use(c.Context(), user.UserID, req.Code)
	if err != nil {
		log.Printf("[redeem] use uid=%s: %v", user.UserID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to redeem"})
	}
	if res.OK {
		log.Printf("[redeem] uid=%s redeemed %s", user.UserID, res.Grant.SourceRef)
	} else {
		log.Printf("[redeem] uid=%s refused: %s", user.UserID, res.ErrorCode)
	}
	return c.JSON(res)
}

// POST /redeem/admin/codes
func (h *Handler) CreateCode(c fiber.Ctx) error {
	user := auth.MustGetUser(c)

	var req CreateCodeRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	if err := validator.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error(), "detail": err.Error()})
	}

	code, err := h.srv.CreateCode(c.Context(), user.UserID, req)
	switch {
	case errors.Is(err, ErrInvalidRequest):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, ErrCodeExists):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	case err != nil:
		log.Printf("[redeem] create uid=%s: %v", user.UserID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create code"})
	}
	log.Printf("[redeem] admin uid=%s created %s", user.UserID, code.Code)
	return c.JSON(fiber.Map{"code": code, "display": Format(code.Code)})
}

// POST /redeem/admin/codes/disable
func (h *Handler) SetDisabled(c fiber.Ctx) error {
	user := auth.MustGetUser(c)

	var req DisableCodeRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	if err := validator.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}

	err := h.srv.SetDisabled(c.Context(), req.Code, req.Disabled)
	switch {
	case errors.Is(err, ErrInvalidRequest):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case errors.Is(err, ErrCodeNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	case err != nil:
		log.Printf("[redeem] disable uid=%s: %v", user.UserID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update code"})
	}
	log.Printf("[redeem] admin uid=%s set disabled=%v on %s", user.UserID, req.Disabled, req.Code)
	return c.JSON(fiber.Map{"ok": true})
}

// GET /redeem/admin/codes
func (h *Handler) ListCodes(c fiber.Ctx) error {
	codes, err := h.srv.ListCodes(c.Context())
	if err != nil {
		log.Printf("[redeem] list: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list codes"})
	}
	return c.JSON(fiber.Map{"codes": codes})
}
