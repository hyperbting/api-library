// Package pet is the player's owned-pet list. Ownership is not a document of its own: it is the starter pets plus
// every Pet reward in the user's grants ledger (claimed or not), so the list can always be rebuilt from the server
// (a new device, a lost save). The client keeps its own copy in the save snapshot and applies Pet grants to it;
// GET /pets is where it checks against the server.
package pet

import (
	"context"
	"log"
	"sort"

	"api-library/internal/auth"
	"api-library/internal/grant"

	"cloud.google.com/go/firestore"
	"github.com/gofiber/fiber/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// OwnedResponse is GET /pets. Owned holds Addressable addresses ("Pet_Tortoise"), starters first, the rest sorted.
// Mirrors DTO.Pets.OwnedPetsResponse in the Unity client.
type OwnedResponse struct {
	Owned []string `json:"owned"`
}

// Owned builds the list from a grants document (nil = a user without grants).
func Owned(grants *grant.Doc) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(grant.StarterPets)+2)
	for _, id := range grant.StarterPets {
		seen[id] = true
		out = append(out, id)
	}
	var extra []string
	if grants != nil {
		for _, g := range grants.Grants {
			if g == nil {
				continue
			}
			for _, r := range g.Rewards {
				if r.Type == grant.RewardPet && grant.KnownPets[r.Key] && !seen[r.Key] {
					seen[r.Key] = true
					extra = append(extra, r.Key)
				}
			}
		}
	}
	sort.Strings(extra)
	return append(out, extra...)
}

// Repository reads the user's grants ledger.
type Repository interface {
	Grants(ctx context.Context, uid string) (*grant.Doc, error)
}

type firestoreRepo struct{ client *firestore.Client }

func NewFirestoreRepository(client *firestore.Client) Repository {
	return &firestoreRepo{client: client}
}

func (r *firestoreRepo) Grants(ctx context.Context, uid string) (*grant.Doc, error) {
	snap, err := grant.DocRef(r.client, uid).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return grant.NewDoc(), nil
	}
	if err != nil {
		return nil, err
	}
	return grant.Decode(snap)
}

type Service interface {
	Owned(ctx context.Context, uid string) (OwnedResponse, error)
}

type service struct{ repo Repository }

func NewService(repo Repository) Service { return &service{repo: repo} }

func (s *service) Owned(ctx context.Context, uid string) (OwnedResponse, error) {
	grants, err := s.repo.Grants(ctx, uid)
	if err != nil {
		return OwnedResponse{}, err
	}
	return OwnedResponse{Owned: Owned(grants)}, nil
}

// Handler serves GET /pets behind the auth middleware.
type Handler struct{ srv Service }

func NewHandler(srv Service) *Handler { return &Handler{srv: srv} }

func (h *Handler) List(c fiber.Ctx) error {
	user := auth.MustGetUser(c)
	res, err := h.srv.Owned(c.Context(), user.UserID)
	if err != nil {
		log.Printf("[pet] owned uid=%s: %v", user.UserID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read pets"})
	}
	return c.JSON(res)
}
