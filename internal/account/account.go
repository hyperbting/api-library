// Package account lists the players that linked their progress to a game platform (Play Games, Game Center), for
// the admin panel. The source is Firebase Auth itself: a link is a provider on the Firebase account, so nothing has
// to be saved by the client and the list cannot drift from what Firebase knows. Anonymous players stay unlinked.
package account

import (
	"context"
	"log"
	"strings"
	"time"

	"firebase.google.com/go/v4/auth"
	"github.com/gofiber/fiber/v3"
	"google.golang.org/api/iterator"
)

// Provider IDs Firebase reports for the platform identities (and Apple / Google, should they be added later).
var platformProviders = map[string]string{
	"playgames.google.com": "Play Games",
	"gc.apple.com":         "Game Center",
	"apple.com":            "Apple",
	"google.com":           "Google",
}

// Provider is one linked identity of a Firebase account.
type Provider struct {
	ProviderID  string `json:"providerId"`
	Label       string `json:"label"`
	PlatformUID string `json:"platformUid"`
	DisplayName string `json:"displayName,omitempty"`
}

// Row is one linked player.
type Row struct {
	UID         string     `json:"uid"`
	Providers   []Provider `json:"providers"`
	CreatedAt   *time.Time `json:"createdAt,omitempty"`
	LastSignIn  *time.Time `json:"lastSignIn,omitempty"`
	LastRefresh *time.Time `json:"lastRefresh,omitempty"`
}

// Page is one page of the list. NextPageToken is empty on the last page.
type Page struct {
	Accounts      []Row  `json:"accounts"`
	Scanned       int    `json:"scanned"`
	NextPageToken string `json:"nextPageToken,omitempty"`
}

// Source reads one page of Firebase accounts (the Auth client in production).
type Source interface {
	Users(ctx context.Context, pageSize int, token string) (users []*auth.ExportedUserRecord, next string, err error)
}

// FirebaseSource lists accounts with the Firebase Admin Auth client.
type FirebaseSource struct{ Client *auth.Client }

func (s FirebaseSource) Users(ctx context.Context, pageSize int, token string) ([]*auth.ExportedUserRecord, string, error) {
	pager := iterator.NewPager(s.Client.Users(ctx, ""), pageSize, token)
	var users []*auth.ExportedUserRecord
	next, err := pager.NextPage(&users)
	return users, next, err
}

// Service builds the list of linked players.
type Service struct{ src Source }

func NewService(src Source) *Service { return &Service{src: src} }

const (
	defaultPageSize = 500
	maxPageSize     = 1000
)

// Linked scans one page of Firebase accounts and returns the ones with a platform provider. A page of the scan is
// up to pageSize accounts (most are anonymous guests), so a page can hold few or no rows and still have a next one.
func (s *Service) Linked(ctx context.Context, pageSize int, token string) (*Page, error) {
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	users, next, err := s.src.Users(ctx, pageSize, token)
	if err != nil {
		return nil, err
	}
	page := &Page{Accounts: []Row{}, Scanned: len(users), NextPageToken: next}
	for _, u := range users {
		if row, ok := toRow(u); ok {
			page.Accounts = append(page.Accounts, row)
		}
	}
	return page, nil
}

func toRow(u *auth.ExportedUserRecord) (Row, bool) {
	if u == nil || u.UserRecord == nil {
		return Row{}, false
	}
	row := Row{UID: u.UID, Providers: []Provider{}}
	for _, info := range u.ProviderUserInfo {
		if info == nil {
			continue
		}
		label, ok := platformProviders[info.ProviderID]
		if !ok {
			continue
		}
		row.Providers = append(row.Providers, Provider{
			ProviderID:  info.ProviderID,
			Label:       label,
			PlatformUID: info.UID,
			DisplayName: strings.TrimSpace(info.DisplayName),
		})
	}
	if len(row.Providers) == 0 {
		return Row{}, false
	}
	if m := u.UserMetadata; m != nil {
		row.CreatedAt = millis(m.CreationTimestamp)
		row.LastSignIn = millis(m.LastLogInTimestamp)
		row.LastRefresh = millis(m.LastRefreshTimestamp)
	}
	return row, true
}

func millis(ms int64) *time.Time {
	if ms <= 0 {
		return nil
	}
	t := time.UnixMilli(ms).UTC()
	return &t
}

// Handler serves the admin route.
type Handler struct{ srv *Service }

func NewHandler(srv *Service) *Handler { return &Handler{srv: srv} }

// AdminList answers GET /admin/accounts?pageToken=&pageSize=: the linked players among one page of Firebase accounts.
func (h *Handler) AdminList(c fiber.Ctx) error {
	page, err := h.srv.Linked(c.Context(), fiber.Query[int](c, "pageSize", defaultPageSize), c.Query("pageToken"))
	if err != nil {
		log.Printf("[account] admin list: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to list accounts"})
	}
	return c.JSON(page)
}
