package auth

import (
	"api-library/pkg/user"
	"context"
	"time"
)

type Service interface {
	// C - Create Account / Initial Identity
	// RegisterEmail(ctx context.Context, email string) (usr *user.User, accessToken, refreshToken string, err error)
	RegisterEmailPassword(ctx context.Context, email, rawPassword string) (usr *user.User, accessToken, refreshToken string, err error)

	// R - Read Identity / Authenticate
	LoginEmailPassword(ctx context.Context, email, rawPassword, ip string) (usr *user.User, accessToken, refreshToken string, err error)
	LoginSocial(ctx context.Context, platform user.DevicePlatform, platformUUID, ip string) (usr *user.User, accessToken, refreshToken string, err error)

	// U - Update Session State
	RefreshToken(ctx context.Context, refreshTokenStr string) (newAccessToken string, newRefreshToken string, err error)

	// D - Destroy Active Session
	//Logout(ctx context.Context, sessionID string) error
}

type authServiceImpl struct {
	tokenMgr TokenManager      // Interface from jwt.go
	repo     SessionRepository // Interface from repo
	usrSvc   user.Service      // Interface from user package
}

func NewService(tm TokenManager, repo SessionRepository, usrSvc user.Service) Service {
	return &authServiceImpl{
		tokenMgr: tm,
		repo:     repo,
		usrSvc:   usrSvc,
	}
}

// func (s *authServiceImpl) RegisterEmail(ctx context.Context, email string) (usr *user.User, accessToken, refreshToken string, err error) {
// 	if usr, err = s.usrSvc.GetByEmail(ctx, email); err != nil {
// 		return nil, "", "", err
// 	} else if usr != nil {
// 		return nil, "", "", ErrUserAlreadyExists
// 	}

// 	//TODO: email user for ?

// 	if usr, err = s.usrSvc.CreateEmailUser(ctx, email); err != nil {
// 		return nil, "", "", err
// 	}

// 	var jti string
// 	if accessToken, refreshToken, jti, err = s.tokenMgr.GenerateTokenPair(usr.UserIdentity.String(), []string{"email", "player"}); err != nil {
// 		return nil, "", "", err
// 	}

// 	session := &Session{UserID: usr.UserIdentity.String(), JTI: jti, ExpiresAt: time.Now().Add(24 * time.Hour)}

// 	if err = s.repo.SaveSession(ctx, session); err != nil {
// 		return nil, "", "", err
// 	}

// 	return
// }

func (s *authServiceImpl) RegisterEmailPassword(ctx context.Context, email, rawPassword string) (usr *user.User, accessToken, refreshToken string, err error) {
	var retrievedUsr *user.User
	if retrievedUsr, err = s.usrSvc.GetByEmail(ctx, email); err != nil {
		return
	} else if retrievedUsr != nil {
		err = ErrUserAlreadyExists
		return
	}

	//TODO: email user for email verification before create one
	var createdUsr *user.User
	if createdUsr, err = s.usrSvc.CreateEmailPasswordUser(ctx, email, rawPassword); err != nil {
		return
	}

	var tp *TokenPair
	if tp, err = s.tokenMgr.GenerateTokenPair(createdUsr.UserIdentity.String(), createdUsr.Roles.Strings()); err != nil {
		return
	}

	session := &Session{
		UserID:    usr.UserIdentity.String(),
		JTI:       tp.JTI,
		ExpiresAt: tp.RefreshExpiresAt,
	}

	if err = s.repo.SaveSession(ctx, session); err != nil {
		return
	}

	usr = createdUsr
	accessToken = tp.AccessToken
	refreshToken = tp.RefreshToken
	return
}

// Login with Email & Password
func (s *authServiceImpl) LoginEmailPassword(ctx context.Context, email, rawPassword, ip string) (usr *user.User, accessToken, refreshToken string, err error) {
	var retrievedUsr *user.User
	if retrievedUsr, err = s.usrSvc.GetByEmail(ctx, email); err != nil || usr == nil {
		return nil, "", "", ErrInvalidCredentials
	}

	// Verify password hash on the domain model
	var ok bool
	ok, err = retrievedUsr.VerifyPassword(rawPassword)
	if err != nil {
		return // Internal error (e.g., hash corruption, bcrypt error)
	}
	if !ok {
		err = ErrInvalidCredentials // Authentication failure (wrong password)
		return
	}

	// Update latest login IP
	// _ = s.usrSvc.UpdateLastIP(ctx, usr.ID, ip)

	// Generate session/token
	var tp *TokenPair
	if tp, err = s.tokenMgr.GenerateTokenPair(retrievedUsr.UserIdentity.String(), retrievedUsr.Roles.Strings()); err != nil {
		return
	}

	session := &Session{
		UserID: usr.UserIdentity.String(),
		JTI:    tp.JTI,
		//Device:    "",
		ExpiresAt: tp.RefreshExpiresAt,
	}

	if err = s.repo.SaveSession(ctx, session); err != nil {
		return
	}

	usr = retrievedUsr
	accessToken = tp.AccessToken
	refreshToken = tp.RefreshToken
	return
}

// Social Login (Google, Apple, Steam, etc.)
func (s *authServiceImpl) LoginSocial(ctx context.Context, platform user.DevicePlatform, platformUUID, ip string) (usr *user.User, accessToken, refreshToken string, err error) {
	// Step A: Find user by identity (platform + platformUUID) or create one on the fly
	var targetUsr *user.User
	if targetUsr, err = s.usrSvc.GetByIdentity(ctx, platform, platformUUID); err != nil {

		// If user doesn't exist, auto-register social account
		if createdUsr, lerr := s.usrSvc.CreatePlatformUser(ctx, platform, platformUUID); lerr != nil {
			err = lerr
			return
		} else {
			targetUsr = createdUsr
		}
	}

	// Step B: Update IP
	//_ = s.usrSvc.UpdateLastIP(ctx, usr.ID, ip)

	// Step C: Generate session/token
	var tp *TokenPair
	if tp, err = s.tokenMgr.GenerateTokenPair(targetUsr.UserIdentity.String(), targetUsr.Roles.Strings()); err != nil {
		return
	}
	session := &Session{
		UserID: targetUsr.UserIdentity.String(),
		JTI:    tp.JTI,
		//Device:    "",
		ExpiresAt: tp.RefreshExpiresAt,
	}

	if err = s.repo.SaveSession(ctx, session); err != nil {
		return
	}

	usr = targetUsr
	accessToken = tp.AccessToken
	refreshToken = tp.RefreshToken
	return
}

// // 4. Logout
// func (s *usrServiceImpl) Logout(ctx context.Context, sessionID string) error {
// 	s.sesRepo.Get(ctx,)
// 	// Invalidate session in Redis
// 	return s.sesRepo.Delete(ctx, sessionID)
// }

func (s *authServiceImpl) ValidateAccessToken(ctx context.Context, accessTokenStr string) (*AccessClaims, error) {
	return s.tokenMgr.ValidateAccessToken(accessTokenStr)
}

func (s *authServiceImpl) RefreshToken(ctx context.Context, refreshTokenStr string) (accessToken, refreshToken string, err error) {
	// 1. Statelessly parse and verify the Refresh Token via jwt.go
	claims, err := s.tokenMgr.ValidateRefreshToken(refreshTokenStr)
	if err != nil {
		return "", "", err
	}

	// 2. Fetch the stored session from Redis via repo using claims.UserID
	session, err := s.repo.Get(ctx, claims.UserID, claims.ID)
	if err != nil {
		return "", "", err
	}

	now := time.Now()
	// 3. Compare JTIs (Check if token is blacklisted/revoked)
	if !session.Matches(claims, now) {
		return "", "", ErrTokenRevoked
	}

	var usrIdentity *user.UserIdentity
	if usrIdentity, err = user.ParseUserIdentity(claims.UserID); err != nil {
		return
	}

	var usr *user.User
	if usr, err = s.usrSvc.GetByIdentity(ctx, usrIdentity.Platform, usrIdentity.PlatformUUID); err != nil {
		return
	}
	if usr == nil {
		err = user.ErrUserNotFound
		return
	}

	// 4. Generate new token pair via jwt.go
	var tp *TokenPair
	if tp, err = s.tokenMgr.GenerateTokenPair(claims.UserID, usr.Roles.Strings()); err != nil {
		return
	}

	// 5. Update active session in Redis via repo
	newSession := session.Rotate(tp.JTI, tp.RefreshExpiresAt)
	if err = s.repo.SaveSession(ctx, newSession); err != nil {
		return
	}

	// 6. Clean up the old session key immediately
	_ = s.repo.Delete(ctx, claims.UserID, claims.ID)

	accessToken = tp.AccessToken
	refreshToken = tp.RefreshToken
	return
}
