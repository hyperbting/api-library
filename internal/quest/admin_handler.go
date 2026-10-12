package quest

import (
	"errors"
	"log"

	"api-library/internal/auth"
	"api-library/pkg/validator"

	"github.com/gofiber/fiber/v3"
)

// Admin handlers. The route group must demand a Firebase ID token and the admin role
// (setupAdminRoutes). The target is a uid in the query or body, never the caller's own token.

func badRequest(c fiber.Ctx) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
}

// GET /admin/quests/catalog
func (h *Handler) AdminCatalog(c fiber.Ctx) error {
	return c.JSON(h.srv.AdminCatalog())
}

// GET /admin/quests/progress?uid=
func (h *Handler) AdminGet(c fiber.Ctx) error {
	uid := c.Query("uid")
	if !ValidUID(uid) {
		return badRequest(c)
	}

	res, err := h.srv.AdminGet(c.Context(), uid)
	if err != nil {
		log.Printf("[quest] admin get by=%s uid=%s: %v", auth.MustGetUser(c).UserID, uid, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to load quest progress"})
	}
	return c.JSON(res)
}

// PUT /admin/quests/progress
func (h *Handler) AdminSet(c fiber.Ctx) error {
	var req SetProgressRequest
	if err := c.Bind().JSON(&req); err != nil || validator.Struct(req) != nil || !ValidUID(req.UID) {
		return badRequest(c)
	}

	by := auth.MustGetUser(c).UserID
	res, err := h.srv.AdminSetProgress(c.Context(), req.UID, req)
	switch {
	case errors.Is(err, ErrUnknownQuest), errors.Is(err, ErrInvalidStep):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case err != nil:
		log.Printf("[quest] admin set by=%s uid=%s quest=%s: %v", by, req.UID, req.QuestID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to edit quest progress"})
	}
	log.Printf("[quest] admin set by=%s uid=%s quest=%s step=%d completed=%t", by, req.UID, req.QuestID, res.StepIndex, res.Completed)
	return c.JSON(res)
}

// POST /admin/quests/progress/reset
func (h *Handler) AdminResetQuest(c fiber.Ctx) error {
	var req ResetQuestRequest
	if err := c.Bind().JSON(&req); err != nil || validator.Struct(req) != nil || !ValidUID(req.UID) {
		return badRequest(c)
	}

	by := auth.MustGetUser(c).UserID
	removed, err := h.srv.AdminResetQuest(c.Context(), req.UID, req.QuestID)
	if err != nil {
		log.Printf("[quest] admin reset quest by=%s uid=%s quest=%s: %v", by, req.UID, req.QuestID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to reset quest progress"})
	}
	log.Printf("[quest] admin reset quest by=%s uid=%s quest=%s had=%t", by, req.UID, req.QuestID, removed)
	return c.JSON(fiber.Map{"removed": removed})
}

// POST /admin/quests/reset
func (h *Handler) AdminResetUser(c fiber.Ctx) error {
	var req ResetUserRequest
	if err := c.Bind().JSON(&req); err != nil || validator.Struct(req) != nil || !ValidUID(req.UID) {
		return badRequest(c)
	}

	by := auth.MustGetUser(c).UserID
	if err := h.srv.Reset(c.Context(), req.UID); err != nil {
		log.Printf("[quest] admin reset by=%s uid=%s: %v", by, req.UID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to reset quest progress"})
	}
	log.Printf("[quest] admin reset by=%s uid=%s", by, req.UID)
	return c.JSON(fiber.Map{"ok": true})
}
