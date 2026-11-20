package pet

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"api-library/internal/auth"
	"api-library/internal/grant"
	"api-library/pkg/validator"

	"cloud.google.com/go/firestore"
	"github.com/gofiber/fiber/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Admin side of pet ownership (panel page "Pets", and the Dev tab on the admin's own account). Ownership is the set
// of Pet rewards in the grants ledger, so every change goes through that ledger and is written to its PetLog:
// an add is an "admin" grant like a gem gift, a remove or a reset takes the pet's grants out and archives them in
// the log entry (see grant.PetLogEntry). Starter pets are owned without a grant and cannot be removed.

var ErrInvalidRequest = errors.New("invalid request")

// Limits of one change.
const (
	maxChangePets = 64
	maxChangeNote = 120
)

// Error codes of a change (HTTP 200, ok=false).
const (
	CodeNoPets       = "no_pets"         // add or remove with an empty list
	CodeUnknownPet   = "unknown_pet"     // a name that is not one of grant.KnownPets
	CodeStarterPet   = "starter_pet"     // a starter pet cannot be removed
	CodeNothingToDo  = "nothing_to_do"   // every pet to add is owned already, or none to remove is owned
	CodeTooManyGrant = "too_many_grants" // the ledger is full (grant.MaxGrants)
)

// ChangeRequest is POST /admin/pets/change (panel, any uid) and POST /pets/admin/change (Dev tab, the caller's uid).
type ChangeRequest struct {
	UID string `json:"uid" validate:"max=128"`
	// Chosen by the caller for each submit (a UUID): a double click or a retry does not change anything twice.
	RequestID string          `json:"requestId" validate:"required,min=8,max=64"`
	Action    grant.PetAction `json:"action" validate:"required,oneof=add remove reset"`
	// The pets to add or remove (Addressable addresses, "Pet_Parrot"); ignored by reset, which removes every pet
	// that is not a starter.
	Pets []string `json:"pets" validate:"max=64"`
	Note string   `json:"note" validate:"max=120"`
}

type ChangeResponse struct {
	OK        bool               `json:"ok"`
	ErrorCode string             `json:"errorCode,omitempty"`
	Repeated  bool               `json:"repeated,omitempty"`
	Entry     *grant.PetLogEntry `json:"entry,omitempty"`
	Owned     []string           `json:"owned"`
}

// AdminPetSource is one grant that gives an owned pet.
type AdminPetSource struct {
	GrantID   string       `json:"grantId"`
	Source    grant.Source `json:"source"`
	SourceRef string       `json:"sourceRef"`
	CreatedAt time.Time    `json:"createdAt"`
	Claimed   bool         `json:"claimed"`
}

type AdminOwnedPet struct {
	Pet     string           `json:"pet"`
	Starter bool             `json:"starter"`
	Sources []AdminPetSource `json:"sources"`
}

// AdminOverview is GET /admin/pets/user?uid=...
type AdminOverview struct {
	UID   string          `json:"uid"`
	Owned []AdminOwnedPet `json:"owned"`
	// Every pet a Pet reward may name, sorted: what the panel offers to add.
	Known []string `json:"known"`
	// Newest first.
	Log []grant.PetLogEntry `json:"log"`
	// False when the player has no grants document: the uid may be mistyped.
	HasRecords bool `json:"hasRecords"`
}

// BuildOverview describes the pets of a grants document.
func BuildOverview(uid string, d *grant.Doc, exists bool) *AdminOverview {
	out := &AdminOverview{UID: uid, Owned: []AdminOwnedPet{}, Known: knownPets(), Log: []grant.PetLogEntry{}, HasRecords: exists}
	sources := map[string][]AdminPetSource{}
	for _, g := range sortedGrants(d) {
		for _, r := range g.Rewards {
			if r.Type == grant.RewardPet && grant.KnownPets[r.Key] {
				sources[r.Key] = append(sources[r.Key], AdminPetSource{
					GrantID: g.ID, Source: g.Source, SourceRef: g.SourceRef, CreatedAt: g.CreatedAt, Claimed: g.ClaimedAt != nil,
				})
			}
		}
	}
	for _, p := range Owned(d) {
		out.Owned = append(out.Owned, AdminOwnedPet{Pet: p, Starter: isStarter(p), Sources: append([]AdminPetSource{}, sources[p]...)})
	}
	for i := len(d.PetLog) - 1; i >= 0; i-- {
		out.Log = append(out.Log, d.PetLog[i])
	}
	return out
}

// ApplyChange applies one admin change to d at time now. write reports whether d changed (false for a repeated
// request or a refused change). The same request ID never applies twice.
func ApplyChange(d *grant.Doc, by string, req ChangeRequest, now time.Time) (res *ChangeResponse, write bool, err error) {
	res = &ChangeResponse{}
	if e := d.PetLogEntryFor(req.RequestID); e != nil {
		stored := *e
		res.OK, res.Repeated, res.Entry, res.Owned = true, true, &stored, Owned(d)
		return res, false, nil
	}

	note := strings.TrimSpace(req.Note)
	if len(note) > maxChangeNote {
		note = note[:maxChangeNote]
	}
	entry := grant.PetLogEntry{RequestID: req.RequestID, Action: req.Action, By: by, Note: note, At: now, Pets: []string{}}

	switch req.Action {
	case grant.PetAdd:
		code, err := addPets(d, req, &entry)
		if err != nil || code != "" {
			res.ErrorCode, res.Owned = code, Owned(d)
			return res, false, err
		}
	case grant.PetRemove, grant.PetReset:
		pets := req.Pets
		if req.Action == grant.PetReset {
			pets = nil
			for _, p := range Owned(d) {
				if !isStarter(p) {
					pets = append(pets, p)
				}
			}
		} else if code := checkPets(pets, true); code != "" {
			res.ErrorCode, res.Owned = code, Owned(d)
			return res, false, nil
		}
		removePets(d, pets, &entry)
		if len(entry.Pets) == 0 {
			res.ErrorCode, res.Owned = CodeNothingToDo, Owned(d)
			return res, false, nil
		}
	default:
		return nil, false, fmt.Errorf("%w: action %q", ErrInvalidRequest, req.Action)
	}

	d.AppendPetLog(entry)
	d.UpdatedAt = now
	stored := entry
	res.OK, res.Entry, res.Owned = true, &stored, Owned(d)
	return res, true, nil
}

// addPets gives the pets the player does not own yet as "admin" grants, MaxRewardsPerGrant pets per grant
// (IDs "admin:<requestId>", then "admin:<requestId>:2", ...), created unclaimed so the client applies them.
func addPets(d *grant.Doc, req ChangeRequest, entry *grant.PetLogEntry) (string, error) {
	if code := checkPets(req.Pets, false); code != "" {
		return code, nil
	}
	owned := map[string]bool{}
	for _, p := range Owned(d) {
		owned[p] = true
	}
	var add []string
	for _, p := range req.Pets {
		if !owned[p] {
			owned[p] = true
			add = append(add, p)
		}
	}
	if len(add) == 0 {
		return CodeNothingToDo, nil
	}
	chunks := (len(add) + grant.MaxRewardsPerGrant - 1) / grant.MaxRewardsPerGrant
	if len(d.Grants)+chunks > grant.MaxGrants {
		return CodeTooManyGrant, nil
	}
	for i := 0; i < chunks; i++ {
		part := add[i*grant.MaxRewardsPerGrant : min((i+1)*grant.MaxRewardsPerGrant, len(add))]
		rewards := make([]grant.Reward, len(part))
		for j, p := range part {
			rewards[j] = grant.Reward{Type: grant.RewardPet, Key: p, Amount: 1}
		}
		id := "admin:" + req.RequestID
		if i > 0 {
			id = fmt.Sprintf("%s:%d", id, i+1)
		}
		g := grant.Grant{ID: id, Source: grant.SourceAdmin, SourceRef: strings.TrimSpace(entry.By + " pets " + entry.Note), Rewards: rewards, CreatedAt: entry.At}
		if _, err := d.Add(g); err != nil {
			if errors.Is(err, grant.ErrTooManyGrants) {
				return CodeTooManyGrant, nil
			}
			return "", err
		}
		entry.GrantIDs = append(entry.GrantIDs, id)
	}
	entry.Pets = add
	return "", nil
}

// removePets takes every Pet line naming one of pets out of the ledger and archives it in entry: a grant left
// with no rewards is removed (archived whole, its ID free again), a grant with other rewards keeps them.
// entry.Pets lists the pets that were owned through a grant.
func removePets(d *grant.Doc, pets []string, entry *grant.PetLogEntry) {
	target := map[string]bool{}
	for _, p := range pets {
		if !isStarter(p) {
			target[p] = true
		}
	}
	removed := map[string]bool{}
	for _, g := range sortedGrants(d) {
		var keep, taken []grant.Reward
		for _, r := range g.Rewards {
			if r.Type == grant.RewardPet && target[r.Key] {
				taken = append(taken, r)
				removed[r.Key] = true
			} else {
				keep = append(keep, r)
			}
		}
		if len(taken) == 0 {
			continue
		}
		archived := *g
		archived.Rewards = taken
		entry.Archived = append(entry.Archived, archived)
		if len(keep) == 0 {
			delete(d.Grants, g.ID)
		} else {
			d.Grants[g.ID].Rewards = keep
		}
	}
	for _, p := range pets {
		if removed[p] {
			entry.Pets = append(entry.Pets, p)
			delete(removed, p) // a name given twice is listed once
		}
	}
}

func checkPets(pets []string, removing bool) string {
	if len(pets) == 0 {
		return CodeNoPets
	}
	for _, p := range pets {
		if !grant.KnownPets[p] {
			return CodeUnknownPet
		}
		if removing && isStarter(p) {
			return CodeStarterPet
		}
	}
	return ""
}

func isStarter(p string) bool {
	for _, s := range grant.StarterPets {
		if s == p {
			return true
		}
	}
	return false
}

func knownPets() []string {
	out := make([]string, 0, len(grant.KnownPets))
	for p := range grant.KnownPets {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// sortedGrants lists the grants oldest first (ID breaks ties), so archives and listings are stable.
func sortedGrants(d *grant.Doc) []*grant.Grant {
	if d == nil {
		return nil
	}
	out := make([]*grant.Grant, 0, len(d.Grants))
	for _, g := range d.Grants {
		if g != nil {
			out = append(out, g)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// AdminRepository reads and changes the grants ledger in a transaction.
type AdminRepository interface {
	// Update runs fn on the user's grants document inside a transaction and writes it when fn returns true.
	// exists tells whether the document was there before.
	Update(ctx context.Context, uid string, fn func(d *grant.Doc, exists bool) (bool, error)) error
}

func (r *firestoreRepo) Update(ctx context.Context, uid string, fn func(d *grant.Doc, exists bool) (bool, error)) error {
	ref := grant.DocRef(r.client, uid)
	return r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if err != nil && status.Code(err) != codes.NotFound {
			return err
		}
		d, err := grant.Decode(snap)
		if err != nil {
			return err
		}
		write, err := fn(d, snap != nil && snap.Exists())
		if err != nil || !write {
			return err
		}
		return tx.Set(ref, d)
	})
}

// NewAdminRepository is the Firestore store of the admin service.
func NewAdminRepository(client *firestore.Client) AdminRepository {
	return &firestoreRepo{client: client}
}

type AdminService struct {
	repo AdminRepository
	now  func() time.Time
}

func NewAdminService(repo AdminRepository) *AdminService {
	return &AdminService{repo: repo, now: time.Now}
}

func (s *AdminService) Overview(ctx context.Context, uid string) (*AdminOverview, error) {
	if uid == "" || len(uid) > 128 {
		return nil, fmt.Errorf("%w: uid", ErrInvalidRequest)
	}
	var out *AdminOverview
	err := s.repo.Update(ctx, uid, func(d *grant.Doc, exists bool) (bool, error) {
		out = BuildOverview(uid, d, exists)
		return false, nil
	})
	return out, err
}

func (s *AdminService) Change(ctx context.Context, by string, req ChangeRequest) (*ChangeResponse, error) {
	if req.UID == "" || len(req.UID) > 128 || req.RequestID == "" {
		return nil, fmt.Errorf("%w: uid and requestId", ErrInvalidRequest)
	}
	if len(req.Pets) > maxChangePets {
		return nil, fmt.Errorf("%w: too many pets", ErrInvalidRequest)
	}
	var res *ChangeResponse
	err := s.repo.Update(ctx, req.UID, func(d *grant.Doc, _ bool) (bool, error) {
		// fn may be retried by the transaction: ApplyChange builds the answer from scratch each time.
		r, write, err := ApplyChange(d, by, req, s.now().UTC())
		res = r
		return write, err
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// AdminHandler serves the admin routes; the router demands the admin role.
type AdminHandler struct{ srv *AdminService }

func NewAdminHandler(srv *AdminService) *AdminHandler { return &AdminHandler{srv: srv} }

// GET /admin/pets/user?uid=...  (panel)
func (h *AdminHandler) AdminUser(c fiber.Ctx) error {
	uid := strings.TrimSpace(c.Query("uid"))
	res, err := h.srv.Overview(c.Context(), uid)
	switch {
	case errors.Is(err, ErrInvalidRequest):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case err != nil:
		log.Printf("[pet] admin overview uid=%s: %v", uid, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read the player"})
	}
	return c.JSON(res)
}

// POST /admin/pets/change  (panel: any player by uid)
func (h *AdminHandler) AdminChange(c fiber.Ctx) error { return h.change(c, false) }

// POST /pets/admin/change  (Dev tab: the caller's own account; the uid in the body is ignored)
func (h *AdminHandler) SelfChange(c fiber.Ctx) error { return h.change(c, true) }

func (h *AdminHandler) change(c fiber.Ctx, self bool) error {
	admin := auth.MustGetUser(c)
	var req ChangeRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error()})
	}
	if self {
		req.UID = admin.UserID
	}
	if err := validator.Struct(req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": ErrInvalidRequest.Error(), "detail": err.Error()})
	}
	res, err := h.srv.Change(c.Context(), admin.UserID, req)
	switch {
	case errors.Is(err, ErrInvalidRequest):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	case err != nil:
		log.Printf("[pet] admin change admin=%s uid=%s %s: %v", admin.UserID, req.UID, req.Action, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to change pets"})
	}
	if res.OK && !res.Repeated {
		log.Printf("[pet] admin=%s %s uid=%s pets=%v", admin.UserID, req.Action, req.UID, res.Entry.Pets)
	}
	return c.JSON(res)
}
