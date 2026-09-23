package user

import (
	"context"
	"api-library/pkg/session"
)

type Service interface {
	// 0. Auth flows (create/find user + generate session tokens)
	RegisterEmailPassword(ctx context.Context, email, rawPassword string) (*User, *session.TokenPair, error)
	LoginEmailPassword(ctx context.Context, email, rawPassword, ip string) (*User, *session.TokenPair, error)
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
	userRepo UserRepository
	sessionSrv session.Service
}

func NewService(userRepo UserRepository, sessionSrv session.Service) Service {
	return &usrServiceImpl{
		userRepo: userRepo,
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

func (s *usrServiceImpl) RegisterEmailPassword(ctx context.Context, email, rawPassword string) (*User, *session.TokenPair, error) {
	existing, err := s.userRepo.FindByEmail(ctx, email)
	if err == nil && existing != nil {
		return nil, nil, ErrEmailAlreadyExists
	}

	newUser, err := NewEmailPasswordUser(email, rawPassword)
	if err != nil {
		return nil, nil, err
	}
	if err := s.userRepo.Create(ctx, newUser); err != nil {
		return nil, nil, ErrFailedToCreateUser
	}

	tp, err := s.sessionSrv.CreateSession(ctx, newUser.UserIdentity.String(), newUser.Roles.Strings())
	if err != nil {
		return nil, nil, err
	}
	return newUser, tp, nil
}

func (s *usrServiceImpl) LoginEmailPassword(ctx context.Context, email, rawPassword, ip string) (*User, *session.TokenPair, error) {
	usr, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil || usr == nil {
		return nil, nil, ErrInvalidCredentials
	}

	ok, err := usr.VerifyPassword(rawPassword)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, ErrInvalidCredentials
	}

	tp, err := s.sessionSrv.CreateSession(ctx, usr.UserIdentity.String(), usr.Roles.Strings())
	if err != nil {
		return nil, nil, err
	}
	return usr, tp, nil
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
