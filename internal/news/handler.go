package news

import (
	"errors"
	"log"
	"strings"

	"api-library/internal/auth"

	"github.com/gofiber/fiber/v3"
)

// Handler serves the news API. GET /news sits behind the auth middleware only; the admin routes behind the admin
// gate as well (setupAdminRoutes).
type Handler struct {
	srv     Service
	isAdmin auth.AdminChecker
}

// isAdmin decides whether a GET /news caller also gets the dev items; nil means nobody does.
func NewHandler(srv Service, isAdmin auth.AdminChecker) *Handler {
	return &Handler{srv: srv, isAdmin: isAdmin}
}

// GET /news
func (h *Handler) List(c fiber.Ctx) error {
	user := auth.MustGetUser(c)

	includeDev := false
	if h.isAdmin != nil {
		admin, err := h.isAdmin(c.Context(), user.UserID)
		if err != nil {
			// News must not break because the role check did: show the public items.
			log.Printf("[news] admin check uid=%s: %v", user.UserID, err)
		}
		includeDev = admin
	}

	items, err := h.srv.ForPlayer(c.Context(), includeDev)
	if err != nil {
		log.Printf("[news] list uid=%s: %v", user.UserID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load news"})
	}
	return c.JSON(ListResponse{Items: items})
}

// GET /admin/news
func (h *Handler) AdminList(c fiber.Ctx) error {
	items, err := h.srv.All(c.Context())
	if err != nil {
		log.Printf("[news] admin list: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load news"})
	}
	return c.JSON(ListResponse{Items: items})
}

// POST /admin/news
func (h *Handler) AdminSave(c fiber.Ctx) error {
	by := auth.MustGetUser(c).UserID

	var req SaveRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}

	item, err := h.srv.Save(c.Context(), req)
	var bad *RequestError
	switch {
	case errors.As(err, &bad):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": bad.Message})
	case errors.Is(err, ErrNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": ErrNotFound.Error()})
	case err != nil:
		log.Printf("[news] admin save by=%s: %v", by, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save news"})
	}
	log.Printf("[news] admin save by=%s id=%s audience=%s", by, item.ID, item.Audience)
	return c.JSON(item)
}

// POST /admin/news/delete
func (h *Handler) AdminDelete(c fiber.Ctx) error {
	by := auth.MustGetUser(c).UserID

	var req DeleteRequest
	if err := c.Bind().JSON(&req); err != nil || strings.TrimSpace(req.ID) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	if err := h.srv.Delete(c.Context(), req.ID); err != nil {
		log.Printf("[news] admin delete by=%s id=%s: %v", by, req.ID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to delete news"})
	}
	log.Printf("[news] admin delete by=%s id=%s", by, req.ID)
	return c.JSON(fiber.Map{"ok": true})
}
