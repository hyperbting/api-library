package quest

import (
	"log"

	"api-library/internal/auth"
	"api-library/pkg/validator"

	"github.com/gofiber/fiber/v3"
)

// Handler serves the quest API. Routes must sit behind the auth middleware.
// Rejected steps are HTTP 200 with accepted=false: the client treats non-2xx as a network failure and retries.
type Handler struct {
	srv Service
}

func NewHandler(srv Service) *Handler {
	return &Handler{srv: srv}
}

// GET /quests/progress
func (h *Handler) GetProgress(c fiber.Ctx) error {
	user := auth.MustGetUser(c)

	res, err := h.srv.GetProgress(c.Context(), user.UserID)
	if err != nil {
		log.Printf("[quest] get progress uid=%s: %v", user.UserID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load quest progress"})
	}
	return c.JSON(res)
}

// POST /quests/complete-step
func (h *Handler) CompleteStep(c fiber.Ctx) error {
	user := auth.MustGetUser(c)

	var req CompleteStepRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	if err := validator.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error(), "detail": err.Error()})
	}

	res, err := h.srv.CompleteStep(c.Context(), user.UserID, req)
	if err != nil {
		log.Printf("[quest] complete step uid=%s quest=%s step=%s: %v", user.UserID, req.QuestID, req.StepID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to complete quest step"})
	}
	if !res.Accepted {
		log.Printf("[quest] rejected uid=%s quest=%s step=%s index=%d: %s", user.UserID, req.QuestID, req.StepID, req.StepIndex, res.ErrorCode)
	}
	return c.JSON(res)
}
