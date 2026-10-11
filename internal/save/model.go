// Package save stores the player's game state snapshot (cash, venues, unlocks, ...). The client's economy
// runs locally, so the snapshot is client-determined: the server keeps it, versions it, refuses a stale or
// regressing write, and returns the grants the player has not applied yet. Server-owned data (quests,
// grants, later gacha results) never goes through this document.
package save

import (
	"time"

	"api-library/internal/grant"
)

// Firestore layout (server-only; security rules deny all client access to users/{uid}/private/*):
//
//	users/{uid}/private/save     Doc, one document per user
//
// The snapshot and the grants document are written in one transaction when a save claims grants.
const (
	UsersCollection   = "users"
	PrivateCollection = "private"
	SaveDocID         = "save"

	SchemaVersion = 1

	// Upper bound for the opaque client blob. A snapshot is a few hundred bytes today.
	MaxDataBytes = 64 * 1024

	MaxVenues        = 64
	MaxVenueLevel    = 1000
	MaxClaimedGrants = 100
	MaxIDLength      = 128
)

// Error codes returned in PutResponse.ErrorCode (HTTP 200, ok=false).
const (
	// The client's baseRev is not the stored rev: another device saved since. Server holds the state.
	CodeRevConflict = "rev_conflict"
	// The snapshot's lifetime earnings are below the stored ones. Progress only grows, so this is a bug
	// (an empty default save uploaded after a failed load) or a reset; the client adopts the server state.
	CodeRegressed = "regressed"
)

// Summary is what the server reads from a snapshot (conflict display, regression check, later bounds).
// Mirrors DTO.Save.SaveSummary in the Unity client.
type Summary struct {
	Cash           float64 `firestore:"cash" json:"cash"`
	LifetimeEarned float64 `firestore:"lifetimeEarned" json:"lifetimeEarned"`
	// venueId -> cashier level
	Venues map[string]int `firestore:"venues" json:"venues"`
}

// Doc is stored at users/{uid}/private/save.
type Doc struct {
	SchemaVersion int `firestore:"schemaVersion"`
	// Incremented by the server on each accepted write; the client sends the rev it based its change on.
	Rev     int64     `firestore:"rev"`
	SavedAt time.Time `firestore:"savedAt"`
	// Device clock at upload (Unix ms), informational.
	ClientTimeMs int64   `firestore:"clientTimeMs"`
	Summary      Summary `firestore:"summary"`
	// The client's own JSON (opaque to the server, so new client fields need no server change).
	Data string `firestore:"data"`
}

// API DTOs, mirrored in Assets/Scripts/DTO/Save/SaveDTO.cs.

// ServerState is the stored snapshot, sent on load and on a rejected write.
type ServerState struct {
	Rev     int64     `json:"rev"`
	SavedAt time.Time `json:"savedAt"`
	Summary Summary   `json:"summary"`
	Data    string    `json:"data"`
}

// GET /api/{version}/save/load
type LoadResponse struct {
	// Server clock (Unix ms): the client keeps the offset to its own clock and measures time away with it.
	ServerNowMs int64 `json:"serverNowMs"`
	// Nil for an account that never saved.
	State  *ServerState  `json:"state"`
	Grants []grant.Grant `json:"grants"`
}

// POST /api/{version}/save/sync
type SyncRequest struct {
	BaseRev       int64    `json:"baseRev"`
	Data          string   `json:"data"`
	Summary       Summary  `json:"summary"`
	ClientTimeMs  int64    `json:"clientTimeMs"`
	ClaimedGrants []string `json:"claimedGrants"`
}

type SyncResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   string `json:"errorCode,omitempty"`
	Rev         int64  `json:"rev"`
	ServerNowMs int64  `json:"serverNowMs"`
	// Set when the write was rejected: the state the client should resolve against.
	Server *ServerState `json:"server,omitempty"`
	// Grants still waiting after this call (new ones arrive here between boots).
	Grants []grant.Grant `json:"grants"`
}
