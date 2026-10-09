package user

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// UsersCollection holds one document per player. Firebase players use their Firebase UID as the document ID
// (written by the quest service); email players use their identity string ("e|<email>"), which can never equal
// a Firebase UID, so the Firestore rule "a client reads users/{its own uid}" never exposes an email player.
const UsersCollection = "users"

// Field names. The roles field is shared with the quest admin check (auth.FirestoreAdminChecker).
const (
	fPlatform     = "platform"
	fPlatformUUID = "platformUUID"
	fName         = "name"
	fRoles        = "UserRoles"
	fEmail        = "email"
	fVerifiedAt   = "emailVerifiedAt"
	fPasswordHash = "passwordHash"
	fStatus       = "status"
	fCreatedAt    = "createdAt"
)

type firestoreUserRepo struct {
	col *firestore.CollectionRef
}

// NewFirestoreRepository stores email players in Firestore, for the Cloud Run service that has no SQL database.
// Only the lookups the email login and register flows use are supported; lookups by numeric ID are not.
func NewFirestoreRepository(client *firestore.Client) UserRepository {
	return &firestoreUserRepo{col: client.Collection(UsersCollection)}
}

func docID(platform DevicePlatform, platformUUID string) string {
	id := UserIdentity{Platform: platform, PlatformUUID: platformUUID}
	return id.String()
}

// Create fails with gorm.ErrDuplicatedKey when the identity exists, so the service reports "email already registered".
func (r *firestoreUserRepo) Create(ctx context.Context, u *User) error {
	if u.Status == "" {
		u.Status = AccountStatusActive
	}
	_, err := r.col.Doc(docID(u.Platform, u.PlatformUUID)).Create(ctx, toDoc(u))
	if status.Code(err) == codes.AlreadyExists {
		return gorm.ErrDuplicatedKey
	}
	return err
}

func (r *firestoreUserRepo) FindByID(context.Context, uint) (*User, error) {
	return nil, errors.New("user: lookup by numeric ID is not supported with Firestore")
}

func (r *firestoreUserRepo) FindByEmail(ctx context.Context, email string) (*User, error) {
	return r.get(ctx, docID(DevicePlatformEMAIL, email))
}

func (r *firestoreUserRepo) FindByIdentity(ctx context.Context, platform DevicePlatform, platformUUID string) (*User, error) {
	return r.get(ctx, docID(platform, platformUUID))
}

func (r *firestoreUserRepo) get(ctx context.Context, id string) (*User, error) {
	snap, err := r.col.Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, gorm.ErrRecordNotFound
	}
	if err != nil {
		return nil, err
	}
	return fromDoc(snap.Data()), nil
}

func (r *firestoreUserRepo) Update(ctx context.Context, u *User) error {
	_, err := r.col.Doc(docID(u.Platform, u.PlatformUUID)).Set(ctx, toDoc(u))
	return err
}

// UpdatePassword takes the document ID (the identity string, as in the access token).
func (r *firestoreUserRepo) UpdatePassword(ctx context.Context, userID string, passwordHash string) error {
	_, err := r.col.Doc(userID).Update(ctx, []firestore.Update{{Path: fPasswordHash, Value: passwordHash}})
	return err
}

func (r *firestoreUserRepo) Delete(ctx context.Context, id string) error {
	_, err := r.col.Doc(id).Delete(ctx)
	return err
}

func toDoc(u *User) map[string]any {
	doc := map[string]any{
		fPlatform:     string(u.Platform),
		fPlatformUUID: u.PlatformUUID,
		fName:         u.Name,
		fRoles:        u.Roles.Strings(),
		fStatus:       string(u.Status),
		fCreatedAt:    time.Now().UTC(),
	}
	if u.Email != nil {
		doc[fEmail] = *u.Email
	}
	if u.EmailVerifiedAt != nil {
		doc[fVerifiedAt] = *u.EmailVerifiedAt
	}
	if u.PasswordHash != nil {
		doc[fPasswordHash] = *u.PasswordHash
	}
	return doc
}

func fromDoc(data map[string]any) *User {
	str := func(key string) string {
		s, _ := data[key].(string)
		return s
	}
	u := &User{
		Name:         str(fName),
		Status:       AccountStatus(str(fStatus)),
		UserIdentity: UserIdentity{Platform: DevicePlatform(str(fPlatform)), PlatformUUID: str(fPlatformUUID)},
	}
	if roles, ok := data[fRoles].([]any); ok {
		for _, r := range roles {
			if name, ok := r.(string); ok {
				u.Roles = append(u.Roles, UserRoleType(name))
			}
		}
	}
	if email := str(fEmail); email != "" {
		u.Email = &email
	}
	if hash := str(fPasswordHash); hash != "" {
		u.PasswordHash = &hash
	}
	if t, ok := data[fVerifiedAt].(time.Time); ok {
		u.EmailVerifiedAt = &t
	}
	return u
}
