package user

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type DevicePlatform string

const (
	DevicePlatformWeb   DevicePlatform = "web"   // account/password
	DevicePlatformEMAIL DevicePlatform = "email" // account with email TOTP

	DevicePlatformGOOGLE DevicePlatform = "google"
	DevicePlatformAPPLE  DevicePlatform = "apple"
	DevicePlatformOCULUS DevicePlatform = "oculus"
	DevicePlatformSTEAM  DevicePlatform = "steam"
	DevicePlatformPICO   DevicePlatform = "pico"

	DevicePlatformWORKER DevicePlatform = "worker" // reserved, can only be created by ADMIN

	spacer = "|"
)

var (
	DevicePlatformList = []DevicePlatform{
		DevicePlatformWeb,
		DevicePlatformEMAIL,
		DevicePlatformGOOGLE,
		DevicePlatformAPPLE,
		DevicePlatformOCULUS,
		DevicePlatformSTEAM,
		DevicePlatformPICO,
	}

	DevicePlatformOAuthList = []DevicePlatform{
		DevicePlatformGOOGLE,
		DevicePlatformAPPLE,
		DevicePlatformOCULUS,
		DevicePlatformSTEAM,
		DevicePlatformPICO,
	}

	DevicePlatformAcronym = map[DevicePlatform]string{
		DevicePlatformWeb:    "w",
		DevicePlatformEMAIL:  "e",
		DevicePlatformGOOGLE: "g",
		DevicePlatformAPPLE:  "a",
		DevicePlatformOCULUS: "o",
		DevicePlatformSTEAM:  "s",
		DevicePlatformPICO:   "p",
		DevicePlatformWORKER: "w",
	}

	AcronymToDevicePlatform = map[string]DevicePlatform{
		DevicePlatformAcronym[DevicePlatformWeb]:    DevicePlatformWeb,
		DevicePlatformAcronym[DevicePlatformEMAIL]:  DevicePlatformEMAIL,
		DevicePlatformAcronym[DevicePlatformGOOGLE]: DevicePlatformGOOGLE,
		DevicePlatformAcronym[DevicePlatformAPPLE]:  DevicePlatformAPPLE,
		DevicePlatformAcronym[DevicePlatformOCULUS]: DevicePlatformOCULUS,
		DevicePlatformAcronym[DevicePlatformSTEAM]:  DevicePlatformSTEAM,
		DevicePlatformAcronym[DevicePlatformPICO]:   DevicePlatformPICO,
		DevicePlatformAcronym[DevicePlatformWORKER]: DevicePlatformWORKER,
	}
)

type UserIdentity struct {
	Platform     DevicePlatform `gorm:"type:varchar(32);not null;index:idx_platform_id,unique" validate:"required,max=32"`
	PlatformUUID string         `gorm:"type:varchar(128);not null;index:idx_platform_id,unique" validate:"required,min=1,max=128"`
}

func (u *UserIdentity) String() string {
	if u == nil {
		return ""
	}
	acronym := DevicePlatformAcronym[u.Platform]
	return fmt.Sprintf("%s%s%s", acronym, spacer, u.PlatformUUID)
}

func NewUserIdentity(platform, platformUUID string) (*UserIdentity, error) {
	dp := DevicePlatform(platform)
	if _, ok := DevicePlatformAcronym[dp]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPlatform, platform)
	}
	return &UserIdentity{Platform: dp, PlatformUUID: platformUUID}, nil
}

func ParseUserIdentity(uuid string) (*UserIdentity, error) {
	parts := strings.SplitN(uuid, spacer, 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("%w: %s", ErrInvalidIdentity, uuid)
	}

	platform, ok := AcronymToDevicePlatform[parts[0]]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownPlatformAcronym, parts[0])
	}

	return &UserIdentity{Platform: platform, PlatformUUID: parts[1]}, nil
}

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
	UserIdentity `gorm:"embedded"`
	AppStatus    `gorm:"embedded"`
	DeviceStatus `gorm:"embedded"`

	Name         string  `gorm:"type:varchar(256)" validate:"omitempty,max=256"`
	Email        *string `gorm:"type:varchar(255);uniqueIndex:idx_users_email" validate:"omitempty,email,max=255"`
	PasswordHash *string `gorm:"type:varchar(512)" validate:"omitempty,max=512"`
	LastIPs      string  `gorm:"type:varchar(128)" validate:"omitempty,ip|max=128"`
}

func (u *User) HasPassword() bool {
	return u.PasswordHash != nil
}

func (u *User) HasEmail() bool {
	return u.Email != nil && *u.Email != ""
}

// VerifyPassword checks if the provided plain text password matches the hash
// It assumes a bcrypt implementation where the hash includes the salt and algorithm
func (u *User) VerifyPassword(plainPassword string) (bool, error) {
	if u.PasswordHash == nil || *u.PasswordHash == "" {
		return false, errors.New("user has no password set")
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
		return nil, fmt.Errorf("password cannot be empty")
	}

	usrPtr, err := NewEmailUser(email)
	if err != nil {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	hashedPassword := string(hash)
	usrPtr.PasswordHash = &hashedPassword

	return usrPtr, nil
}
