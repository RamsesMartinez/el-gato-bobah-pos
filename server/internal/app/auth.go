package app

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

const RefreshTokenTTL = 30 * 24 * time.Hour

type AuthService struct {
	store *store.Store
	jwt   *auth.Manager
	now   func() time.Time
}

func NewAuthService(s *store.Store, jm *auth.Manager, now func() time.Time) *AuthService {
	if now == nil {
		now = time.Now
	}
	return &AuthService{store: s, jwt: jm, now: now}
}

// Session is the result of a successful auth: an access token, an opaque refresh
// token, and the user it belongs to.
type Session struct {
	AccessToken  string      `json:"accessToken"`
	RefreshToken string      `json:"refreshToken"`
	User         domain.User `json:"user"`
}

// Login authenticates a backoffice user by username + password.
func (s *AuthService) Login(ctx context.Context, username, password string) (*Session, error) {
	u, err := s.store.Q.GetUserByUsername(ctx, &username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// bcrypt de descarte: sin él esta rama responde en ~µs y la de "password
			// incorrecto" en decenas de ms, revelando qué usuarios existen (A07).
			auth.CheckDummySecret(password)
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}
	if u.PasswordHash == nil {
		// Usuario sin password: misma igualación de latencia que la rama de arriba.
		auth.CheckDummySecret(password)
		return nil, domain.ErrInvalidCredentials
	}
	if !auth.CheckSecret(*u.PasswordHash, password) {
		return nil, domain.ErrInvalidCredentials
	}
	return s.issue(ctx, u)
}

// PinSwitch re-mints a session for a different operator via their PIN. Requires an
// already-valid device session (enforced at the HTTP layer).
func (s *AuthService) PinSwitch(ctx context.Context, userID int64, pin string) (*Session, error) {
	u, err := s.store.Q.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}
	if !u.IsActive || u.PinHash == nil || !auth.CheckSecret(*u.PinHash, pin) {
		return nil, domain.ErrInvalidCredentials
	}
	return s.issue(ctx, u)
}

// Refresh rotates a refresh token and returns a fresh session.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*Session, error) {
	hash := auth.HashToken(refreshToken)
	rt, err := s.store.Q.GetRefreshToken(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}
	if rt.RevokedAt.Valid || rt.ExpiresAt.Before(s.now()) {
		return nil, domain.ErrUnauthorized
	}
	if err := s.store.Q.RevokeRefreshToken(ctx, hash); err != nil {
		return nil, err
	}
	u, err := s.store.Q.GetUserByID(ctx, rt.UserID)
	if err != nil {
		return nil, err
	}
	// El token viejo ya se revocó arriba. Si el usuario fue dado de baja, no emitimos
	// uno nuevo: así una cuenta desactivada deja de funcionar de inmediato (antes seguía
	// viva hasta 30 días mientras rotara su refresh token).
	if !u.IsActive {
		return nil, domain.ErrUnauthorized
	}
	return s.issue(ctx, u)
}

// Logout revokes a refresh token.
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	return s.store.Q.RevokeRefreshToken(ctx, auth.HashToken(refreshToken))
}

func (s *AuthService) issue(ctx context.Context, u db.User) (*Session, error) {
	du := toDomainUser(u)
	access, err := s.jwt.Issue(du)
	if err != nil {
		return nil, err
	}
	token, hash, err := auth.NewRefreshToken()
	if err != nil {
		return nil, err
	}
	if _, err := s.store.Q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID:    u.ID,
		TokenHash: hash,
		ExpiresAt: s.now().Add(RefreshTokenTTL),
	}); err != nil {
		return nil, err
	}
	return &Session{AccessToken: access, RefreshToken: token, User: du}, nil
}

func toDomainUser(u db.User) domain.User {
	return domain.User{
		ID:        u.ID,
		Name:      u.Name,
		Username:  u.Username,
		Role:      domain.Role(u.Role),
		IsActive:  u.IsActive,
		CreatedAt: u.CreatedAt,
	}
}
