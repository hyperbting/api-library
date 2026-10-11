// Package redeem turns a code the player types into a grant (see package grant). Codes are free promos
// (events, support, creators), not a way to sell content: they live in Firestore, are created by an admin
// through the API, and each player can redeem a given code once.
package redeem

import (
	"crypto/rand"
	"errors"
	"strings"
	"time"

	"api-library/internal/grant"
)

// Firestore layout (server-only; the catch-all security rule denies clients):
//
//	redeemCodes/{CODE}           Code (CODE is normalized: upper case, no dashes or spaces)
//
// A redemption is the grant "redeem:<CODE>" in users/{uid}/private/grants, created in the same transaction
// that increments Code.Redeemed.
const (
	CodesCollection = "redeemCodes"

	// Generated codes: 12 characters from a 32-letter alphabet without I, O, 0, 1 (60 bits).
	generatedLength = 12
	alphabet        = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

	minCodeLength = 4
	maxCodeLength = 32

	MaxRedemptionsLimit = 1_000_000
	maxNoteLength       = 200
)

// Error codes returned in RedeemResponse.ErrorCode (HTTP 200, ok=false).
const (
	CodeInvalid         = "invalid_code"
	CodeExpired         = "expired"
	CodeDisabled        = "disabled"
	CodeFinished        = "finished"
	CodeAlreadyRedeemed = "already_redeemed"
	CodeRateLimited     = "rate_limited"
)

var (
	ErrInvalidRequest = errors.New("invalid redeem request")
	ErrCodeExists     = errors.New("code already exists")
	ErrCodeNotFound   = errors.New("code not found")
)

// Code is stored at redeemCodes/{CODE}.
type Code struct {
	Code    string         `firestore:"code" json:"code"`
	Rewards []grant.Reward `firestore:"rewards" json:"rewards"`
	// 0 = unlimited.
	MaxRedemptions int        `firestore:"maxRedemptions" json:"maxRedemptions"`
	Redeemed       int        `firestore:"redeemed" json:"redeemed"`
	ExpiresAt      *time.Time `firestore:"expiresAt,omitempty" json:"expiresAt,omitempty"`
	Disabled       bool       `firestore:"disabled" json:"disabled"`
	Note           string     `firestore:"note" json:"note"`
	CreatedBy      string     `firestore:"createdBy" json:"createdBy"`
	CreatedAt      time.Time  `firestore:"createdAt" json:"createdAt"`
}

// API DTOs, mirrored in Assets/Scripts/DTO/Redeem/RedeemDTO.cs.

// POST /api/{version}/redeem/use
type UseRequest struct {
	Code string `json:"code" validate:"required,max=64"`
}

type UseResponse struct {
	OK        bool         `json:"ok"`
	ErrorCode string       `json:"errorCode,omitempty"`
	Grant     *grant.Grant `json:"grant,omitempty"`
}

// POST /api/{version}/redeem/admin/codes
type CreateCodeRequest struct {
	// Empty: the server generates one.
	Code           string         `json:"code" validate:"max=64"`
	Rewards        []grant.Reward `json:"rewards"`
	MaxRedemptions int            `json:"maxRedemptions" validate:"min=0"`
	ExpiresAt      *time.Time     `json:"expiresAt"`
	Note           string         `json:"note" validate:"max=200"`
}

// POST /api/{version}/redeem/admin/codes/disable
type DisableCodeRequest struct {
	Code     string `json:"code" validate:"required,max=64"`
	Disabled bool   `json:"disabled"`
}

// Normalize returns the stored form of a typed code: upper case, without spaces and dashes.
// ok is false when what is left cannot be a code.
func Normalize(raw string) (string, bool) {
	var b strings.Builder
	for _, r := range strings.ToUpper(raw) {
		switch {
		case r == '-' || r == '_' || r == ' ' || r == '\t' || r == '\n' || r == '\r':
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			return "", false
		}
	}
	code := b.String()
	if len(code) < minCodeLength || len(code) > maxCodeLength {
		return "", false
	}
	return code, true
}

// Format groups a code in fours for display.
func Format(code string) string {
	var b strings.Builder
	for i, r := range code {
		if i > 0 && i%4 == 0 {
			b.WriteByte('-')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func generate() (string, error) {
	buf := make([]byte, generatedLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, generatedLength)
	for i, v := range buf {
		out[i] = alphabet[int(v)%len(alphabet)] // 256 % 32 == 0: no modulo bias
	}
	return string(out), nil
}
