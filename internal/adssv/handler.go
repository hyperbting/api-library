package adssv

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v3"
)

type Handler struct {
	srv *Service
}

func NewHandler(srv *Service) *Handler { return &Handler{srv: srv} }

// Callback is GET /api/{version}/webhooks/admob/ssv. Public: Google calls it, the signature is the credential.
// Anything but a 200 makes AdMob treat the delivery as failed (and it may retry), so a callback that can never be
// valid gets a 400 and only a server-side failure gets a 500.
func (h *Handler) Callback(c fiber.Ctx) error {
	// The signature covers the query string byte for byte, so it is read raw, not through c.Queries().
	raw := string(c.Request().URI().QueryString())
	cb, err := h.srv.Handle(c.Context(), raw)
	switch {
	case errors.Is(err, ErrNoReward):
		// AdMob's own test of the URL (saving it in the console): signed, so it is Google's, but it rewards nothing.
		log.Printf("[adssv] test call from AdMob, nothing stored")
		return c.SendStatus(fiber.StatusOK)
	case err == nil:
		log.Printf("[adssv] verified uid=%s ad_unit=%s request=%s", cb.UserID, cb.AdUnit, cb.CustomData)
		return c.SendStatus(fiber.StatusOK)
	case errors.Is(err, ErrBadRequest), errors.Is(err, ErrBadSignature), errors.Is(err, ErrStale),
		errors.Is(err, ErrAdUnit), errors.Is(err, ErrUnknownKey):
		// Never log the query: it carries the player's ID and the signature.
		log.Printf("[adssv] rejected: %v", err)
		return c.SendStatus(fiber.StatusBadRequest)
	default:
		log.Printf("[adssv] failed: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}
}
