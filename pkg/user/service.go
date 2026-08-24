package user

import (
	"context"
)

type Service interface {
	// 1. User Creation (Used by AuthService upon successful auth/registration)
	// CreateEmailUser(ctx context.Context, email string) (*User, error) //require sms service to send email
	CreateEmailPasswordUser(ctx context.Context, email, rawPassword string) (*User, error)
	CreatePlatformUser(ctx context.Context, platform DevicePlatform, platformUUID string) (*User, error)

	// 2. Data Lookups (Used by AuthService to find users)
	GetByUserID(ctx context.Context, id uint) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByIdentity(ctx context.Context, platform DevicePlatform, platformUUID string) (*User, error)

	// 3. User Mutations (Used by AuthService & HTTP Handlers)
	//UpdatePassword(ctx context.Context, userID, newRawPassword string) error
	//UpdateLastLoginIP(ctx context.Context, userID, ip string) error
	//UpdateDeviceAndAppStatus(ctx context.Context, userID string, app AppStatus, dev DeviceStatus) error

	// // 4. Platform Linking / Unlinking
	// LinkPlatform(ctx context.Context, userID string, platform DevicePlatform, platformUUID string) error
	// UnlinkPlatform(ctx context.Context, userID string, platform DevicePlatform) error
}

type usrServiceImpl struct {
	userRepo UserRepository
}

func NewService(userRepo UserRepository) Service {
	return &usrServiceImpl{
		userRepo: userRepo,
	}
}

func (s *usrServiceImpl) CreateEmailPasswordUser(ctx context.Context, email, rawPassword string) (*User, error) {
	// Check if user already exists
	existing, err := s.userRepo.FindByEmail(ctx, email)
	if err == nil && existing != nil {
		return nil, ErrEmailAlreadyExists
	}

	// Use model constructor to validate input and hash password
	newUser, err := NewEmailPasswordUser(email, rawPassword)
	if err != nil {
		return nil, err
	}

	// Save to database
	if err := s.userRepo.Create(ctx, newUser); err != nil {
		return nil, ErrFailedToCreateUser
	}

	return newUser, nil
}

func (s *usrServiceImpl) CreatePlatformUser(ctx context.Context, platform DevicePlatform, platformUUID string) (*User, error) {
	// Check if user already exists
	existing, err := s.userRepo.FindByIdentity(ctx, platform, platformUUID)
	if err == nil && existing != nil {
		return nil, ErrEmailAlreadyExists
	}

	// Use model constructor to validate input and hash password
	newUser, err := NewPlatformUser(platform, platformUUID)
	if err != nil {
		return nil, err
	}

	// Save to database
	if err := s.userRepo.Create(ctx, newUser); err != nil {
		return nil, ErrFailedToCreateUser
	}

	return newUser, nil
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
