package iap

import (
	"errors"
	"log"

	"api-library/internal/auth"
	"api-library/pkg/validator"

	"github.com/gofiber/fiber/v3"
)

// Handler serves POST /iap/purchase behind the auth middleware. A refused purchase is HTTP 200 with ok=false;
// a store that could not be asked is HTTP 503, so the client keeps the purchase unconsumed and retries.
type Handler struct {
	srv Service
}

func NewHandler(srv Service) *Handler {
	return &Handler{srv: srv}
}

// POST /iap/purchase
func (h *Handler) Purchase(c fiber.Ctx) error {
	user := auth.MustGetUser(c)

	var req PurchaseRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	if err := validator.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}

	res, err := h.srv.Purchase(c.Context(), user.UserID, req)
	switch {
	case errors.Is(err, ErrStoreUnavailable):
		log.Printf("[iap] uid=%s sku=%s: %v", user.UserID, req.SKU, err)
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "store verification unavailable"})
	case err != nil:
		log.Printf("[iap] uid=%s sku=%s: %v", user.UserID, req.SKU, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to deliver purchase"})
	}
	if res.OK {
		log.Printf("[iap] uid=%s delivered %s (grant %s, first-purchase bonus %v)", user.UserID, req.SKU, res.Grant.ID, res.FirstPurchaseBonus)
	} else {
		log.Printf("[iap] uid=%s sku=%s refused: %s", user.UserID, req.SKU, res.ErrorCode)
	}
	return c.JSON(res)
}

// GET /admin/iap/orders?limit=50  (admin only)
func (h *Handler) AdminOrders(c fiber.Ctx) error {
	limit := fiber.Query[int](c, "limit", 50)
	orders, err := h.srv.Orders(c.Context(), limit)
	if err != nil {
		log.Printf("[iap] admin orders: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list orders"})
	}
	return c.JSON(fiber.Map{"orders": orders})
}
