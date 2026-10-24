package gacha

import (
	"errors"
	"log"
	"strings"

	"api-library/internal/auth"
	"api-library/pkg/validator"

	"github.com/gofiber/fiber/v3"
)

// Handler serves the gacha API. Routes must sit behind the auth middleware. A refused draw is HTTP 200 with
// ok=false (the client treats non-2xx as a network failure).
type Handler struct {
	srv Service
}

func NewHandler(srv Service) *Handler {
	return &Handler{srv: srv}
}

// GET /gacha
func (h *Handler) State(c fiber.Ctx) error {
	user := auth.MustGetUser(c)
	// ?rows=character,accessory names the rows to open; without it both open (an older client).
	var open []Row
	for _, name := range strings.Split(c.Query("rows"), ",") {
		switch Row(strings.TrimSpace(name)) {
		case RowCharacter:
			open = append(open, RowCharacter)
		case RowAccessory:
			open = append(open, RowAccessory)
		}
	}
	st, err := h.srv.State(c.Context(), user.UserID, open...)
	if err != nil {
		log.Printf("[gacha] state uid=%s: %v", user.UserID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read the gacha"})
	}
	return c.JSON(st)
}

// POST /gacha/draw
func (h *Handler) Draw(c fiber.Ctx) error {
	user := auth.MustGetUser(c)

	var req DrawRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	if err := validator.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}

	res, err := h.srv.Draw(c.Context(), user.UserID, req)
	switch {
	case errors.Is(err, ErrInvalidRequest):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case err != nil:
		log.Printf("[gacha] draw uid=%s row=%s mode=%s x%d: %v", user.UserID, req.Row, req.Mode, req.Count, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to draw"})
	}
	if res.OK && !res.Repeated {
		log.Printf("[gacha] uid=%s drew %s %s x%d", user.UserID, req.Row, req.Mode, req.Count)
	} else if !res.OK {
		log.Printf("[gacha] uid=%s draw %s %s x%d refused: %s", user.UserID, req.Row, req.Mode, req.Count, res.ErrorCode)
	}
	return c.JSON(res)
}
