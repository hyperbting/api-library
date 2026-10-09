package user_test

import (
	"context"
	"errors"
	"testing"

	"api-library/internal/user"
	"api-library/pkg/session"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

type fakeRepo struct {
	user.UserRepository
	byEmail   map[string]*user.User
	createErr error
}

func (r *fakeRepo) FindByEmail(_ context.Context, email string) (*user.User, error) {
	if u, ok := r.byEmail[email]; ok {
		return u, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *fakeRepo) Create(_ context.Context, u *user.User) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.byEmail[*u.Email] = u
	return nil
}

type fakeSession struct{ session.Service }

func (fakeSession) IssueAccessToken(context.Context, string, []string) (*session.AccessToken, error) {
	return &session.AccessToken{Token: "t"}, nil
}

func newSvc(repo *fakeRepo) user.Service {
	return user.NewService(repo, fakeSession{})
}

func TestService_EmailIsCaseInsensitive(t *testing.T) {
	repo := &fakeRepo{byEmail: map[string]*user.User{}}
	svc := newSvc(repo)
	ctx := context.Background()

	if _, _, err := svc.RegisterEmailPassword(ctx, " Player@Example.COM ", "password123"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, ok := repo.byEmail["player@example.com"]; !ok {
		t.Fatalf("email not stored normalized: %v", repo.byEmail)
	}
	if _, _, err := svc.RegisterEmailPassword(ctx, "PLAYER@example.com", "password123"); !errors.Is(err, user.ErrEmailAlreadyExists) {
		t.Fatalf("second register with different case: got %v want ErrEmailAlreadyExists", err)
	}
	if _, _, err := svc.LoginEmailPassword(ctx, "Player@Example.com", "password123", ""); err != nil {
		t.Fatalf("login with different case: %v", err)
	}
}

func TestService_RegisterRaceMapsToEmailExists(t *testing.T) {
	for name, createErr := range map[string]error{
		"pg unique violation": &pgconn.PgError{Code: "23505"},
		"gorm duplicated key": gorm.ErrDuplicatedKey,
	} {
		t.Run(name, func(t *testing.T) {
			svc := newSvc(&fakeRepo{byEmail: map[string]*user.User{}, createErr: createErr})
			_, _, err := svc.RegisterEmailPassword(context.Background(), "a@example.com", "password123")
			if !errors.Is(err, user.ErrEmailAlreadyExists) {
				t.Fatalf("got %v want ErrEmailAlreadyExists", err)
			}
		})
	}

	svc := newSvc(&fakeRepo{byEmail: map[string]*user.User{}, createErr: errors.New("db down")})
	if _, _, err := svc.RegisterEmailPassword(context.Background(), "a@example.com", "password123"); !errors.Is(err, user.ErrFailedToCreateUser) {
		t.Fatalf("other create error: got %v want ErrFailedToCreateUser", err)
	}
}

func TestService_LoginBadCredentials(t *testing.T) {
	repo := &fakeRepo{byEmail: map[string]*user.User{}}
	svc := newSvc(repo)
	ctx := context.Background()
	if _, _, err := svc.RegisterEmailPassword(ctx, "a@example.com", "password123"); err != nil {
		t.Fatal(err)
	}
	// Account without a password (e.g. social only) that has an email.
	social, _ := user.NewEmailUser("social@example.com")
	repo.byEmail["social@example.com"] = social

	for name, tc := range map[string][2]string{
		"wrong password": {"a@example.com", "nope-nope"},
		"unknown email":  {"missing@example.com", "password123"},
		"no password":    {"social@example.com", "password123"},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := svc.LoginEmailPassword(ctx, tc[0], tc[1], "")
			if !errors.Is(err, user.ErrInvalidCredentials) {
				t.Fatalf("got %v want ErrInvalidCredentials", err)
			}
		})
	}
}
