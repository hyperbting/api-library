// Package bond is what each pet remembers about the player: its hearts (GDD bond days), and the pet stories and
// heart-level celebrations the player has seen. The server decides when a heart is earned; the client only measures
// play time in a store and reports it. Design: the Unity repo's docs/designs/pets-remember-the-player.md (M1).
package bond

import (
	"errors"
	"regexp"
	"time"
)

// Firestore layout (server-only; security rules deny all client access to users/{uid}/private/*):
//
//	users/{uid}/private/bond   Doc, one document per user
//
// Bond has its own document rather than living in private/quests: it is written about once per heart, story read
// and app pause, and its fields must not be versioned with the quest catalog.
const (
	UsersCollection   = "users"
	PrivateCollection = "private"
	BondDocID         = "bond"

	SchemaVersion = 1

	// Product rules (GDD 2.0.2): a heart fills after HeartSeconds of play once the Cooldown since the last one passed.
	HeartSeconds = 300
	Cooldown     = 20 * time.Hour

	// Plausibility: a pet's reported seconds may not exceed the server time since its last accepted report plus
	// ClampSlack. A pet's first report may cover at most FirstReportWindow (and no time before its grant).
	ClampSlack        = 10 * time.Second
	FirstReportWindow = 10 * time.Minute

	// Request limits.
	MaxEntries         = 40
	MinBatchIDLength   = 8
	MaxBatchIDLength   = 64
	MaxIDLength        = 64
	MaxSeenPerEntry    = 16
	MaxSecondsPerEntry = 24 * 60 * 60
	MaxAdminBondDays   = 10_000

	// Ids of the last accepted batches, so a batch the client resends after a lost reply is applied once.
	MaxBatchIDs = 20
)

// Levels are the heart counts at which a heart level is reached (and a Bond<N> story unlocks).
var Levels = []int{1, 3, 7, 14, 30}

// seenPattern: the story and celebration ids a player may mark as seen. A pattern, not a list, so a new pet needs no
// change here.
var seenPattern = regexp.MustCompile(`^(FirstDay|Bond[1-5]|lvl[1-5])$`)

// Reasons in Rejected.
const (
	ReasonUnknownPet  = "unknown_pet"
	ReasonUnknownSeen = "unknown_seen"
)

var (
	ErrInvalidRequest = errors.New("invalid request")
	ErrDisabled       = errors.New("bond is disabled")
)

// PetBond is one pet's record. Times are server times.
type PetBond struct {
	BondDays int `firestore:"bondDays"`
	// Time of the last heart; nil before the first.
	LastBondAt *time.Time `firestore:"lastBondAt,omitempty"`
	// Play seconds toward the next heart, counted only once the cooldown since LastBondAt passed.
	SecondsSinceLastBond int `firestore:"secondsSinceLastBond"`
	// Time of the last accepted report that carried seconds for this pet; nil before the first.
	LastReportAt *time.Time `firestore:"lastReportAt,omitempty"`
	// Story and celebration ids seen (FirstDay, Bond1..Bond5, lvl1..lvl5). Only grows.
	Seen []string `firestore:"seen"`
}

// Doc is stored at users/{uid}/private/bond.
type Doc struct {
	SchemaVersion int `firestore:"schemaVersion"`
	// When the document was first written. A starter pet (no grant) was "first met" then.
	CreatedAt time.Time           `firestore:"createdAt"`
	UpdatedAt time.Time           `firestore:"updatedAt"`
	Pets      map[string]*PetBond `firestore:"pets"`
	// Ids of the last MaxBatchIDs accepted batches, oldest first.
	Batches []string `firestore:"batches"`
}

func newDoc(now time.Time) *Doc {
	return &Doc{SchemaVersion: SchemaVersion, CreatedAt: now, Pets: map[string]*PetBond{}}
}

// Entry is one pet's play in one store since the client's last batch. Seen-only entries may leave StoreID empty.
type Entry struct {
	Pet     string   `json:"pet"`
	StoreID string   `json:"storeId"`
	Seconds int      `json:"seconds"`
	Seen    []string `json:"seen"`
}

// ReportRequest is POST /bond/report. BatchID stays the same when the client resends a batch whose reply it lost.
type ReportRequest struct {
	BatchID string  `json:"batchId"`
	Entries []Entry `json:"entries"`
}

// Rejected is an item the server did not accept. The client logs it and drops it.
type Rejected struct {
	Pet    string `json:"pet"`
	Item   string `json:"item"`
	Reason string `json:"reason"`
}

// PetView is one pet in a reply. Mirrors DTO.Bond.BondPet in the Unity client.
type PetView struct {
	BondDays             int        `json:"bondDays"`
	Level                int        `json:"level"`
	SecondsSinceLastBond int        `json:"secondsSinceLastBond"`
	NextHeartInSeconds   int        `json:"nextHeartInSeconds"`
	LastBondAt           *time.Time `json:"lastBondAt,omitempty"`
	Seen                 []string   `json:"seen"`
	FirstMetAt           *time.Time `json:"firstMetAt,omitempty"`
}

// StateResponse is GET /bond and the reply of every report: every owned pet, with or without a record.
type StateResponse struct {
	Pets map[string]PetView `json:"pets"`
	// Items of this report that were not accepted (report replies only).
	Rejected []Rejected `json:"rejected,omitempty"`
	// The batch was applied before (a resend); nothing changed.
	Duplicate bool `json:"duplicate,omitempty"`
}

// AdminSetRequest is PUT /admin/bond: the repair path for a wrong record.
type AdminSetRequest struct {
	UID             string `json:"uid"`
	Pet             string `json:"pet"`
	BondDays        *int   `json:"bondDays,omitempty"`
	ClearLastBondAt bool   `json:"clearLastBondAt,omitempty"`
}
