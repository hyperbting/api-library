package save

import (
	"errors"
	"log"

	"api-library/internal/auth"

	"github.com/gofiber/fiber/v3"
)

// Handler serves the save API. Routes must sit behind the auth middleware.
// Rejected syncs are HTTP 200 with ok=false: the client treats non-2xx as a network failure.
type Handler struct {
	srv Service
}

func NewHandler(srv Service) *Handler {
	return &Handler{srv: srv}
}

// GET /save/load
func (h *Handler) Load(c fiber.Ctx) error {
	user := auth.MustGetUser(c)

	res, err := h.srv.Load(c.Context(), user.UserID)
	if err != nil {
		log.Printf("[save] load uid=%s: %v", user.UserID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load save"})
	}
	return c.JSON(res)
}

// POST /save/sync
func (h *Handler) Sync(c fiber.Ctx) error {
	user := auth.MustGetUser(c)

	var req SyncRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}

	res, err := h.srv.Sync(c.Context(), user.UserID, req)
	if errors.Is(err, ErrInvalidRequest) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		log.Printf("[save] sync uid=%s: %v", user.UserID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save"})
	}
	if !res.OK {
		log.Printf("[save] rejected uid=%s baseRev=%d: %s (server rev %d)", user.UserID, req.BaseRev, res.ErrorCode, res.Rev)
	}
	return c.JSON(res)
}

// POST /save/admin/reset (admin role required, enforced by the route)
func (h *Handler) Reset(c fiber.Ctx) error {
	user := auth.MustGetUser(c)

	if err := h.srv.Reset(c.Context(), user.UserID); err != nil {
		log.Printf("[save] reset uid=%s: %v", user.UserID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to reset save"})
	}
	log.Printf("[save] reset uid=%s", user.UserID)
	return c.JSON(fiber.Map{"ok": true})
}
