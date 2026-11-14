package bond

import (
	"errors"
	"log"
	"strings"

	"api-library/internal/auth"

	"github.com/gofiber/fiber/v3"
)

// Handler serves the bond API. Player routes sit behind the auth middleware; admin routes behind the admin gate.
type Handler struct {
	srv     Service
	enabled bool
}

// NewHandler: enabled is config bond.enabled (env BOND_ENABLED). Off, the player routes answer 503 and the admin
// routes still work.
func NewHandler(srv Service, enabled bool) *Handler {
	return &Handler{srv: srv, enabled: enabled}
}

func (h *Handler) disabled(c fiber.Ctx) error {
	return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": ErrDisabled.Error()})
}

// GET /bond
func (h *Handler) State(c fiber.Ctx) error {
	if !h.enabled {
		return h.disabled(c)
	}
	user := auth.MustGetUser(c)
	res, err := h.srv.State(c.Context(), user.UserID)
	if err != nil {
		log.Printf("[bond] state uid=%s: %v", user.UserID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read bond"})
	}
	return c.JSON(res)
}

// POST /bond/report
func (h *Handler) Report(c fiber.Ctx) error {
	if !h.enabled {
		return h.disabled(c)
	}
	user := auth.MustGetUser(c)
	var req ReportRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	res, err := h.srv.Report(c.Context(), user.UserID, req)
	if errors.Is(err, ErrInvalidRequest) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		log.Printf("[bond] report uid=%s batch=%s: %v", user.UserID, req.BatchID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to report"})
	}
	if len(res.Rejected) > 0 {
		log.Printf("[bond] report uid=%s batch=%s rejected %d item(s)", user.UserID, req.BatchID, len(res.Rejected))
	}
	return c.JSON(res)
}

// GET /admin/bond?uid=  (admin only, enforced by the route)
func (h *Handler) AdminGet(c fiber.Ctx) error {
	uid := strings.TrimSpace(c.Query("uid"))
	if uid == "" || len(uid) > 128 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	res, err := h.srv.State(c.Context(), uid)
	if err != nil {
		log.Printf("[bond] admin state uid=%s: %v", uid, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read bond"})
	}
	return c.JSON(res)
}

// PUT /admin/bond  (admin only, enforced by the route)
func (h *Handler) AdminSet(c fiber.Ctx) error {
	admin := auth.MustGetUser(c)
	var req AdminSetRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	res, err := h.srv.AdminSet(c.Context(), req)
	if errors.Is(err, ErrInvalidRequest) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		log.Printf("[bond] admin set admin=%s uid=%s pet=%s: %v", admin.UserID, req.UID, req.Pet, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to update bond"})
	}
	log.Printf("[bond] admin set admin=%s uid=%s pet=%s bondDays=%v clearLastBondAt=%v", admin.UserID, req.UID, req.Pet, req.BondDays, req.ClearLastBondAt)
	return c.JSON(res)
}
