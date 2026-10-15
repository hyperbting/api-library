package gem

import (
	"errors"
	"log"
	"strings"

	"api-library/internal/auth"
	"api-library/pkg/validator"

	"github.com/gofiber/fiber/v3"
)

// Handler serves the gem API. Routes must sit behind the auth middleware. A refused spend is HTTP 200 with
// ok=false (the client treats non-2xx as a network failure).
type Handler struct {
	srv Service
}

func NewHandler(srv Service) *Handler {
	return &Handler{srv: srv}
}

// GET /gems
func (h *Handler) State(c fiber.Ctx) error {
	user := auth.MustGetUser(c)
	st, err := h.srv.State(c.Context(), user.UserID)
	if err != nil {
		log.Printf("[gem] state uid=%s: %v", user.UserID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read gems"})
	}
	return c.JSON(st)
}

// POST /gems/spend
func (h *Handler) Spend(c fiber.Ctx) error {
	user := auth.MustGetUser(c)

	var req SpendRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	if err := validator.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}

	res, err := h.srv.Spend(c.Context(), user.UserID, req)
	if err != nil {
		log.Printf("[gem] spend uid=%s product=%s: %v", user.UserID, req.Product, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to spend gems"})
	}
	if res.OK {
		log.Printf("[gem] uid=%s bought %s, %d gems left", user.UserID, req.Product, res.State.Balance)
	} else {
		log.Printf("[gem] uid=%s refused %s: %s", user.UserID, req.Product, res.ErrorCode)
	}
	return c.JSON(res)
}

// POST /gems/boost/consume
func (h *Handler) ConsumeBoost(c fiber.Ctx) error {
	user := auth.MustGetUser(c)

	var req ConsumeRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	if err := validator.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}

	st, err := h.srv.ConsumeBoost(c.Context(), user.UserID, req.Seconds)
	if err != nil {
		log.Printf("[gem] consume uid=%s: %v", user.UserID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update boost"})
	}
	return c.JSON(st)
}

// GET /admin/gems/user?uid=...  (admin only)
func (h *Handler) AdminUser(c fiber.Ctx) error {
	uid := strings.TrimSpace(c.Query("uid"))
	if uid == "" || len(uid) > 128 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	res, err := h.srv.AdminOverview(c.Context(), uid)
	if err != nil {
		log.Printf("[gem] admin overview uid=%s: %v", uid, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read the player"})
	}
	return c.JSON(res)
}

// POST /admin/gems/gift  (admin only)
func (h *Handler) AdminGift(c fiber.Ctx) error {
	admin := auth.MustGetUser(c)

	var req GiftRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	if err := validator.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error(), "detail": err.Error()})
	}

	res, err := h.srv.Gift(c.Context(), admin.UserID, req)
	switch {
	case errors.Is(err, ErrInvalidRequest):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case err != nil:
		log.Printf("[gem] admin gift admin=%s uid=%s: %v", admin.UserID, req.UID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to gift"})
	}
	if res.OK && !res.Repeated {
		log.Printf("[gem] admin=%s gifted uid=%s %d gems + %d items (%s)", admin.UserID, req.UID, req.Gems, len(req.Items), res.Grant.ID)
	}
	return c.JSON(res)
}
