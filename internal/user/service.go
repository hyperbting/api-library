package user

import (
	"api-library/pkg/session"
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// pgUniqueViolation is the Postgres error code for a unique constraint violation.
const pgUniqueViolation = "23505"

// normalizeEmail makes email lookups case-insensitive and ignores stray spaces.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// isDuplicateKey reports whether err came from violating a unique index.
func isDuplicateKey(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}

var (
	dummyHashOnce sync.Once
	dummyHash     []byte
)

// burnPasswordCheck spends the same time as a real bcrypt check so that login
// for an unknown email takes as long as one for a known email.
func burnPasswordCheck(rawPassword string) {
	dummyHashOnce.Do(func() {
		dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-password"), bcrypt.DefaultCost)
	})
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(rawPassword))
}

type Service interface {
	// 0. Auth flows (create/find user + generate session tokens)
	RegisterEmailPassword(ctx context.Context, email, rawPassword string) (*User, *session.AccessToken, error)
	LoginEmailPassword(ctx context.Context, email, rawPassword, ip string) (*User, *session.AccessToken, error)
	LoginSocial(ctx context.Context, platform DevicePlatform, platformUUID, ip string) (*User, *session.TokenPair, error)

	// 1. Data Lookups (Used by AuthService to find users)
	GetByUserID(ctx context.Context, id uint) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByIdentity(ctx context.Context, platform DevicePlatform, platformUUID string) (*User, error)

	// 2. User Mutations (Used by AuthService & HTTP Handlers)
	//UpdatePassword(ctx context.Context, userID, newRawPassword string) error
	//UpdateLastLoginIP(ctx context.Context, userID, ip string) error
	//UpdateDeviceAndAppStatus(ctx context.Context, userID string, app AppStatus, dev DeviceStatus) error

	// // 3. Platform Linking / Unlinking
	// LinkPlatform(ctx context.Context, userID string, platform DevicePlatform, platformUUID string) error
	// UnlinkPlatform(ctx context.Context, userID string, platform DevicePlatform) error
}

type usrServiceImpl struct {
	userRepo   UserRepository
	sessionSrv session.Service
}

func NewService(userRepo UserRepository, sessionSrv session.Service) Service {
	return &usrServiceImpl{
		userRepo:   userRepo,
		sessionSrv: sessionSrv,
	}
}

func (s *usrServiceImpl) GetByUserID(ctx context.Context, id uint) (*User, error) {
	usr, err := s.userRepo.FindByID(ctx, id)
	if err != nil || usr == nil {
		return nil, ErrUserNotFound
	}
	return usr, nil
}

func (s *usrServiceImpl) GetByEmail(ctx context.Context, email string) (*User, error) {
	usr, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil || usr == nil {
		return nil, ErrUserNotFound
	}
	return usr, nil
}

func (s *usrServiceImpl) GetByIdentity(ctx context.Context, platform DevicePlatform, platformUUID string) (*User, error) {
	usr, err := s.userRepo.FindByIdentity(ctx, platform, platformUUID)
	if err != nil || usr == nil {
		return nil, ErrUserNotFound
	}
	return usr, nil
}

// RegisterEmailPassword creates an email/password user and issues a stateless
// access token only: no refresh token and no session persisted in Redis.
func (s *usrServiceImpl) RegisterEmailPassword(ctx context.Context, email, rawPassword string) (*User, *session.AccessToken, error) {
	email = normalizeEmail(email)
	existing, err := s.userRepo.FindByEmail(ctx, email)
	if err == nil && existing != nil {
		return nil, nil, ErrEmailAlreadyExists
	}

	newUser, err := NewEmailPasswordUser(email, rawPassword)
	if err != nil {
		return nil, nil, err
	}
	if err := s.userRepo.Create(ctx, newUser); err != nil {
		if isDuplicateKey(err) {
			return nil, nil, ErrEmailAlreadyExists
		}
		return nil, nil, ErrFailedToCreateUser
	}

	at, err := s.sessionSrv.IssueAccessToken(ctx, newUser.UserIdentity.String(), newUser.Roles.Strings())
	if err != nil {
		return nil, nil, err
	}
	return newUser, at, nil
}

// LoginEmailPassword verifies email/password credentials and issues a stateless
// access token only: no refresh token and no session persisted in Redis.
func (s *usrServiceImpl) LoginEmailPassword(ctx context.Context, email, rawPassword, ip string) (*User, *session.AccessToken, error) {
	email = normalizeEmail(email)
	usr, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil || usr == nil {
		burnPasswordCheck(rawPassword)
		return nil, nil, ErrInvalidCredentials
	}

	ok, err := usr.VerifyPassword(rawPassword)
	if errors.Is(err, ErrNoPassword) {
		// Account has no password (e.g. social only): same answer as a wrong password.
		burnPasswordCheck(rawPassword)
		return nil, nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, ErrInvalidCredentials
	}

	at, err := s.sessionSrv.IssueAccessToken(ctx, usr.UserIdentity.String(), usr.Roles.Strings())
	if err != nil {
		return nil, nil, err
	}
	return usr, at, nil
}

func (s *usrServiceImpl) LoginSocial(ctx context.Context, platform DevicePlatform, platformUUID, ip string) (*User, *session.TokenPair, error) {
	usr, err := s.userRepo.FindByIdentity(ctx, platform, platformUUID)
	if err != nil || usr == nil {
		// Auto-register social account
		usr, err = NewPlatformUser(platform, platformUUID)
		if err != nil {
			return nil, nil, err
		}
		if err := s.userRepo.Create(ctx, usr); err != nil {
			return nil, nil, ErrFailedToCreateUser
		}
	}

	tp, err := s.sessionSrv.CreateSession(ctx, usr.UserIdentity.String(), usr.Roles.Strings())
	if err != nil {
		return nil, nil, err
	}
	return usr, tp, nil
}
