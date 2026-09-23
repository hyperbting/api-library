package friend

import "api-library/internal/user"

// FriendHandler or Orchestration Service combining both packages
type FriendHandler struct {
	userSvc user.Service
	// relationshipSvc friend.Service
}

// func (h *FriendHandler) GetFollowingProfiles(c *fiber.Ctx) error {
// 	userID := c.Locals("userID").(string)

// 	// 1. Fetch raw relationship facts (IDs) from Relationship Package
// 	followingIDs, err := h.relationshipSvc.GetFollowingIDs(c.Context(), userID)
// 	if err != nil {
// 		return err
// 	}

// 	// 2. Fetch User profiles from User Package
// 	users, err := h.userSvc.GetByIDs(c.Context(), followingIDs)
// 	if err != nil {
// 		return err
// 	}

// 	return c.JSON(users)
// }
