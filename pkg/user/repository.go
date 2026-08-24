package user

import (
	"context"

	"gorm.io/gorm"
)

type UserRepository interface {
	// Creation
	Create(ctx context.Context, user *User) error

	// Lookups (Used by Social & Email logins)
	FindByID(ctx context.Context, id uint) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	FindByIdentity(ctx context.Context, platform DevicePlatform, platformUUID string) (*User, error)

	// Updates
	Update(ctx context.Context, user *User) error
	UpdatePassword(ctx context.Context, userID string, passwordHash string) error
	// UpdateLastIP(ctx context.Context, userID string, ip string) error

	// Deletion / Unlinking
	Delete(ctx context.Context, id string) error
}

type userRepoImpl struct {
	readOnlyDB *gorm.DB
	writeDB    *gorm.DB
}

func NewUserRepository(readOnlyDB, writeDB *gorm.DB) UserRepository {
	return &userRepoImpl{readOnlyDB: readOnlyDB, writeDB: writeDB}
}

func (r *userRepoImpl) Create(ctx context.Context, user *User) error {
	return r.writeDB.WithContext(ctx).Create(user).Error
}

func (r *userRepoImpl) FindByID(ctx context.Context, id uint) (*User, error) {
	var usr User
	if err := r.readOnlyDB.WithContext(ctx).First(&usr, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &usr, nil
}

func (r *userRepoImpl) FindByEmail(ctx context.Context, email string) (*User, error) {
	var usr User
	if err := r.readOnlyDB.WithContext(ctx).Where("email = ?", email).First(&usr).Error; err != nil {
		return nil, err
	}
	return &usr, nil
}

func (r *userRepoImpl) FindByIdentity(ctx context.Context, platform DevicePlatform, platformUUID string) (*User, error) {
	var usr User
	if err := r.readOnlyDB.WithContext(ctx).Where("platform = ? AND platform_uuid = ?", platform, platformUUID).First(&usr).Error; err != nil {
		return nil, err
	}
	return &usr, nil
}

func (r *userRepoImpl) Update(ctx context.Context, user *User) error {
	return r.writeDB.WithContext(ctx).Save(user).Error
}

func (r *userRepoImpl) UpdatePassword(ctx context.Context, userID string, passwordHash string) error {
	return r.writeDB.WithContext(ctx).Model(&User{}).Where("id = ?", userID).Update("password_hash", passwordHash).Error
}

// func (r *userRepoImpl) UpdateLastIP(ctx context.Context, userID string, ip string) error {
// 	return r.writeDB.WithContext(ctx).Model(&User{}).Where("id = ?", userID).Update("last_ips", ip).Error
// }

func (r *userRepoImpl) Delete(ctx context.Context, id string) error {
	return r.writeDB.WithContext(ctx).Delete(&User{}, "id = ?", id).Error
}
