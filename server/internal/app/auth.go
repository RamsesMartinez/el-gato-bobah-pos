package app

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
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
			// bcrypt de descarte: pin-switch está exento del throttle per-IP y el lockout es
			// per-userID, así que sin igualar la latencia un atacante autenticado enumera qué
			// userIDs existen/están activos/tienen PIN barriendo N (A07).
			auth.CheckDummySecret(pin)
			return nil, domain.ErrInvalidCredentials
		}
		return nil, err
	}
	if !u.IsActive || u.PinHash == nil {
		auth.CheckDummySecret(pin)
		return nil, domain.ErrInvalidCredentials
	}
	if !auth.CheckSecret(*u.PinHash, pin) {
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
	switch domain.ClassifyRefresh(rt.RevokedAt.Valid, rt.ExpiresAt, s.now()) {
	case domain.RefreshReused:
		// Reuse-detection: un refresh ya revocado que reaparece delata robo/reuso. No se
		// puede saber si lo presenta el atacante o el usuario, así que se revoca TODA la
		// familia (ambos quedan fuera y re-autentican). Fail-closed: si la revocación
		// falla, igual denegamos, pero lo registramos como incidente aparte.
		if err := s.store.Q.RevokeUserRefreshTokens(ctx, rt.UserID); err != nil {
			logging.SecurityEvent(ctx, "refresh_reuse_revoke_failed", "user_id", rt.UserID, "error", err.Error())
			return nil, domain.ErrUnauthorized
		}
		logging.SecurityEvent(ctx, "refresh_reuse", "user_id", rt.UserID)
		return nil, domain.ErrUnauthorized
	case domain.RefreshExpired:
		return nil, domain.ErrUnauthorized
	}
	// Rotación atómica: revoca SOLO si sigue activo. revoked==0 significa que otro request ya
	// rotó/revocó este token entre nuestro read y ahora (dos pestañas refrescando a la vez, o
	// un reuso concurrente): se deniega a quien pierde la carrera para no acuñar dos tokens
	// vivos. Un robo real se detecta igual en la siguiente ronda por la rama revoked-at-read
	// de arriba (ClassifyRefresh→RefreshReused→revoca familia). Cierra el TOCTOU del
	// read-then-revoke sin castigar refrescos concurrentes legítimos.
	revoked, err := s.store.Q.RevokeRefreshTokenIfActive(ctx, hash)
	if err != nil {
		return nil, err
	}
	if revoked == 0 {
		return nil, domain.ErrUnauthorized
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
