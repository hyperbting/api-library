package news

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// News items live in the Firestore collection "news" (server-only: the security rules deny clients everything
// outside users/*, so the game reads them through GET /news). One collection holds both audiences:
//
//	public  every player (menu News tab, "what's new" after an update)
//	dev     admin accounts only; the server leaves them out for everyone else
//
// Text is per language (the game's locale codes); the client picks its language and falls back to "en".
const (
	Collection = "news"

	AudiencePublic = "public"
	AudienceDev    = "dev"

	// Limits of what the panel may save.
	maxTitle   = 80
	maxBody    = 1000
	maxVersion = 32
	maxItems   = 200
)

// Locales are the language keys an item may carry.
var Locales = []string{"en", "zh-Hant", "ja"}

var (
	ErrInvalidRequest = errors.New("invalid news request")
	ErrNotFound       = errors.New("news item not found")
)

type Item struct {
	ID       string            `firestore:"-" json:"id"`
	Audience string            `firestore:"audience" json:"audience"`
	Title    map[string]string `firestore:"title" json:"title"`
	Body     map[string]string `firestore:"body" json:"body"`
	// The app version the item is about (display only), e.g. "1.0".
	Version     string     `firestore:"version,omitempty" json:"version,omitempty"`
	Pinned      bool       `firestore:"pinned" json:"pinned"`
	PublishedAt time.Time  `firestore:"publishedAt" json:"publishedAt"`
	ExpiresAt   *time.Time `firestore:"expiresAt,omitempty" json:"expiresAt,omitempty"`
}

// Expired reports whether the item no longer shows to players.
func (i *Item) Expired(now time.Time) bool {
	return i.ExpiresAt != nil && !i.ExpiresAt.After(now)
}

// SaveRequest is POST /admin/news: creates an item, or updates the one named by ID.
type SaveRequest struct {
	ID        string            `json:"id"`
	Audience  string            `json:"audience"`
	Title     map[string]string `json:"title"`
	Body      map[string]string `json:"body"`
	Version   string            `json:"version"`
	Pinned    bool              `json:"pinned"`
	ExpiresAt *time.Time        `json:"expiresAt"`
}

// DeleteRequest is POST /admin/news/delete.
type DeleteRequest struct {
	ID string `json:"id"`
}

// ListResponse is GET /news and GET /admin/news.
type ListResponse struct {
	Items []Item `json:"items"`
}

func validLocale(code string) bool {
	for _, l := range Locales {
		if l == code {
			return true
		}
	}
	return false
}

// Validate trims the text in place and reports why a request cannot be saved ("" when it can).
func (r *SaveRequest) Validate() string {
	if r.Audience != AudiencePublic && r.Audience != AudienceDev {
		return "audience must be public or dev"
	}
	r.Version = strings.TrimSpace(r.Version)
	if len(r.Version) > maxVersion {
		return "version is too long"
	}
	if msg := cleanText(r.Title, "title", maxTitle); msg != "" {
		return msg
	}
	if msg := cleanText(r.Body, "body", maxBody); msg != "" {
		return msg
	}
	return ""
}

// cleanText needs an English text and only known languages, each within the limit.
func cleanText(text map[string]string, field string, max int) string {
	for code, value := range text {
		if !validLocale(code) {
			return field + ": unknown language " + code
		}
		value = strings.TrimSpace(value)
		if value == "" {
			delete(text, code)
			continue
		}
		if len([]rune(value)) > max {
			return field + " (" + code + ") is longer than " + strconv.Itoa(max) + " characters"
		}
		text[code] = value
	}
	if text["en"] == "" {
		return field + ": an English text is required"
	}
	return ""
}
