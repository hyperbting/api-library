package user

import (
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AccountStatus string

const (
	AccountStatusActive    AccountStatus = "active"
	AccountStatusSuspended AccountStatus = "suspended"
	AccountStatusBanned    AccountStatus = "banned"
)

type AppStatus struct {
	Region  string `gorm:"type:varchar(32);not null" validate:"required,max=32"`
	Version string `gorm:"type:varchar(32);not null" validate:"required,max=32"`
}

type DeviceStatus struct {
	DeviceName string `gorm:"type:varchar(128)" validate:"omitempty,max=128"`
	DeviceUUID string `gorm:"type:varchar(128)" validate:"omitempty,max=128"`
}

type User struct {
	gorm.Model
	Name         string    `gorm:"type:varchar(256)"`
	Roles        UserRoles `gorm:"serializer:json;type:json"`
	LastIPs      []string  `gorm:"serializer:json;type:json"`
	UserIdentity `gorm:"embedded"`
	AppStatus    `gorm:"embedded"`
	DeviceStatus `gorm:"embedded"`
	Status       AccountStatus `gorm:"type:varchar(32);default:'active'"`

	// optional account/password auth info
	Email           *string    `gorm:"type:varchar(255);uniqueIndex:idx_users_email"`
	EmailVerifiedAt *time.Time `gorm:"type:timestamp"` // NULL means unverified
	PasswordHash    *string    `gorm:"type:varchar(512)"`
}

func (u *User) HasPassword() bool {
	return u.PasswordHash != nil && *u.PasswordHash != ""
}

func (u *User) HasEmail() bool {
	return u.Email != nil && *u.Email != ""
}

func (u *User) IsEmailVerified() bool {
	return u.Email != nil && u.EmailVerifiedAt != nil
}

// VerifyPassword checks if the provided plain text password matches the hash
// It assumes a bcrypt implementation where the hash includes the salt and algorithm
func (u *User) VerifyPassword(plainPassword string) (bool, error) {
	if !u.HasPassword() {
		return false, ErrNoPassword
	}

	// Compare the plain text password with the stored hash
	// This uses bcrypt.CompareHashAndPassword which returns true if they match
	// and the error is nil. The error is non-nil if the hash is invalid or the
	// plain text password does not match the hash.
	err := bcrypt.CompareHashAndPassword([]byte(*u.PasswordHash), []byte(plainPassword))
	if err != nil {
		return false, err
	}

	return true, nil
}

// NewPlatformUser constructs a basic user identity for third-party platforms (OAuth, Steam, etc.)
func NewPlatformUser(platform DevicePlatform, platformUUID string) (*User, error) {
	if _, ok := DevicePlatformAcronym[platform]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPlatform, platform)
	}

	return &User{
		UserIdentity: UserIdentity{
			Platform:     platform,
			PlatformUUID: platformUUID,
		},
		Roles: UserRoles{UserRolePlayer},
	}, nil
}

// NewEmailUser creates a passwordless email user (e.g., magic links or OTP logins)
func NewEmailUser(email string) (*User, error) {
	usrPtr, err := NewPlatformUser(DevicePlatformEMAIL, email)
	if err != nil {
		return nil, err
	}

	usrPtr.Email = &email
	return usrPtr, nil
}

// NewEmailPasswordUser constructs an email user with a hashed password
func NewEmailPasswordUser(email, rawPassword string) (*User, error) {
	if rawPassword == "" {
		return nil, ErrPasswordEmpty
	}

	usrPtr, err := NewEmailUser(email)
	if err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrPasswordHashFailed, err.Error())
	}

	hashedPassword := string(hash)
	usrPtr.PasswordHash = &hashedPassword

	return usrPtr, nil
}
