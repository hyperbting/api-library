package user

import (
	"fmt"
	"strings"
)

const (
	spacer = "|"
)

type UserIdentity struct {
	Platform     DevicePlatform `gorm:"type:varchar(32);not null;index:idx_platform_id,unique" validate:"required,max=32"`
	PlatformUUID string         `gorm:"type:varchar(128);not null;index:idx_platform_id,unique" validate:"required,min=1,max=128"`
}

// String() pair with ParseUserIdentity()
func (u *UserIdentity) String() string {
	if u == nil {
		return ""
	}
	acronym := DevicePlatformAcronym[u.Platform]
	return fmt.Sprintf("%s%s%s", acronym, spacer, u.PlatformUUID)
}

// ParseUserIdentity() pair with String()
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

func NewUserIdentity(platform, platformUUID string) (*UserIdentity, error) {
	dp := DevicePlatform(platform)
	if _, ok := DevicePlatformAcronym[dp]; !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPlatform, platform)
	}
	return &UserIdentity{Platform: dp, PlatformUUID: platformUUID}, nil
}
