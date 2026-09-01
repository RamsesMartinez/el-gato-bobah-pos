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

// RefreshTokenTTL es el RESPALDO cuando el negocio no tiene ajustes todavía. La duración real sale
// de business_settings.session_hours; esta constante era el valor único antes de que el ajuste
// existiera, y quedarse en 30 días es lo que hacía que una tableta olvidada siguiera autenticada
// durante un mes.
const RefreshTokenTTL = 30 * 24 * time.Hour

type AuthService struct {
	// pinPepper: secreto de la huella determinista. Vacío = no se puede deducir de quién es un PIN.
	pinPepper string
	store     *store.Store
	jwt       *auth.Manager
	now       func() time.Time
}

func NewAuthService(s *store.Store, jm *auth.Manager, now func() time.Time) *AuthService {
	return NewAuthServiceConPepper(s, jm, now, "")
}

// NewAuthServiceConPepper: igual, con el secreto de la huella determinista del PIN. Sin él el modo
// de solo-PIN no puede deducir de quién es un PIN, y por eso tampoco se deja encender.
func NewAuthServiceConPepper(s *store.Store, jm *auth.Manager, now func() time.Time, pinPepper string) *AuthService {
	if now == nil {
		now = time.Now
	}
	return &AuthService{store: s, jwt: jm, now: now, pinPepper: pinPepper}
}

// Session is the result of a successful auth: an access token, an opaque refresh
// token, and the user it belongs to. CompanyID viaja aparte para que el handler lo
// codifique en la cookie de refresh (cid.token): el refresh resuelve su tenant sin conocer
// al usuario todavía.
type Session struct {
	AccessToken  string      `json:"accessToken"`
	RefreshToken string      `json:"refreshToken"`
	CompanyID    int64       `json:"-"`
	User         domain.User `json:"user"`
}

// Login authenticates a user by username + company slug + password. El identificador de login
// es username@slug: se resuelve el slug→company_id (resolver SECURITY DEFINER, sin tenant aún)
// y luego el usuario se busca YA dentro del tenant (RLS lo acota a esa empresa).
func (s *AuthService) Login(ctx context.Context, username, slug, password string) (*Session, error) {
	companyID, err := s.store.Q.ResolveCompanyBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if companyID == 0 {
		// Empresa inexistente: iguala latencia (bcrypt de descarte) para no filtrar qué slugs existen.
		auth.CheckDummySecret(password)
		return nil, domain.ErrInvalidCredentials
	}
	var sess *Session
	err = s.store.WithTenant(ctx, companyID, func(q *db.Queries) error {
		u, err := q.GetUserByUsername(ctx, &username)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// bcrypt de descarte: sin él la rama "no existe" responde en ~µs y la de
				// "password incorrecto" en decenas de ms, revelando qué usuarios existen (A07).
				auth.CheckDummySecret(password)
				return domain.ErrInvalidCredentials
			}
			return err
		}
		if u.PasswordHash == nil {
			auth.CheckDummySecret(password)
			return domain.ErrInvalidCredentials
		}
		if !auth.CheckSecret(*u.PasswordHash, password) {
			return domain.ErrInvalidCredentials
		}
		sess, err = s.issue(ctx, q, u)
		return err
	})
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// PinSwitch re-mints a session for a different operator via their PIN, DENTRO de la misma
// empresa: corre bajo el tenant del request (QC/WithTx), así RLS impide cambiar a un usuario
// de otra empresa (el userID de otra empresa simplemente no existe para esta sesión).
func (s *AuthService) PinSwitchEnEstacion(ctx context.Context, userID int64, pin string, actorID int64, refreshActual string) (*Session, error) {
	var sess *Session
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		u, err := q.GetUserByID(ctx, userID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// bcrypt de descarte: pin-switch está exento del throttle per-IP y el lockout es
				// per-userID; sin igualar latencia un atacante autenticado enumera userIDs (A07).
				auth.CheckDummySecret(pin)
				return domain.ErrInvalidCredentials
			}
			return err
		}
		if !u.IsActive || u.PinHash == nil {
			auth.CheckDummySecret(pin)
			return domain.ErrInvalidCredentials
		}
		if !auth.CheckSecret(*u.PinHash, pin) {
			return domain.ErrInvalidCredentials
		}
		// El vencimiento sale del token que ESTA estación viene presentando, y no se repone.
		//
		// Por token y no por persona: buscar por user_id tomaba el vencimiento más lejano de
		// cualquiera de sus tabletas, así que entrar fresco en una le regalaba horas a la otra.
		// Y reponer el plazo completo haría que una tableta usada cada veinte minutos no caducara
		// nunca, con lo que el límite del turno sería decorativo.
		hashActual := auth.HashToken(refreshActual)
		vence, err := q.LiveRefreshExpiry(ctx, db.LiveRefreshExpiryParams{
			TokenHash: hashActual, ExpiresAt: s.now(),
		})
		if err != nil {
			// Sin sesión viva en esta estación no hay reloj que conservar, y arrancar uno nuevo
			// sería regalar un turno completo a cambio de un PIN. Se niega: la salida es entrar con
			// usuario y contraseña, que la pantalla de bloqueo ofrece a la vista.
			return domain.ErrUnauthorized
		}
		sess, err = s.issueUntil(ctx, q, u, vence)
		if err != nil {
			return err
		}
		// Se revoca SOLO la de esta estación. Revocar todas las de la persona tumbaba sus otras
		// tabletas: entregar una dejaba al compañero de la otra fuera a media venta.
		return q.RevokeRefreshTokenByHash(ctx, hashActual)
	})
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// Refresh rotates a refresh token and returns a fresh session. companyID viene de la cookie
// (cid.token): fija el tenant antes de buscar el token, así RLS acota la búsqueda a esa empresa
// (un token válido presentado con el cid equivocado no matchea → 401).
func (s *AuthService) Refresh(ctx context.Context, companyID int64, refreshToken string) (*Session, error) {
	hash := auth.HashToken(refreshToken)
	var sess *Session
	var reusedUserID int64 // != 0 → se detectó reuso; se responde 401 TRAS commitear la revocación
	err := s.store.WithTenant(ctx, companyID, func(q *db.Queries) error {
		rt, err := q.GetRefreshToken(ctx, hash)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrUnauthorized
			}
			return err
		}
		switch domain.ClassifyRefresh(rt.RevokedAt.Valid, rt.ExpiresAt, s.now()) {
		case domain.RefreshReused:
			// Reuse-detection: un refresh ya revocado que reaparece delata robo/reuso → se revoca
			// TODA la familia. CLAVE: la revocación va en ESTA tx; devolver un error aquí la haría
			// rollback. Por eso devolvemos nil (commit) y señalamos el reuso por variable, para
			// responder 401 después del commit.
			if err := q.RevokeUserRefreshTokens(ctx, rt.UserID); err != nil {
				return err // fail-closed: sin revocar, abortamos (rollback) y el llamador ve error
			}
			reusedUserID = rt.UserID
			return nil
		case domain.RefreshExpired:
			return domain.ErrUnauthorized
		}
		// Rotación atómica TOCTOU-safe: revoca solo si sigue activo; revoked==0 => otro request
		// ya rotó (dos pestañas o reuso concurrente) → se deniega a quien pierde la carrera.
		revoked, err := q.RevokeRefreshTokenIfActive(ctx, hash)
		if err != nil {
			return err
		}
		if revoked == 0 {
			return domain.ErrUnauthorized
		}
		u, err := q.GetUserByID(ctx, rt.UserID)
		if err != nil {
			return err
		}
		// Cuenta dada de baja deja de refrescar de inmediato (antes seguía viva hasta 30 días).
		if !u.IsActive {
			return domain.ErrUnauthorized
		}
		sess, err = s.issue(ctx, q, u)
		return err
	})
	if reusedUserID != 0 {
		logging.SecurityEvent(ctx, "refresh_reuse", "user_id", reusedUserID)
		return nil, domain.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	return sess, nil
}

// Logout revokes a refresh token dentro de su empresa (companyID de la cookie).
func (s *AuthService) Logout(ctx context.Context, companyID int64, refreshToken string) error {
	return s.store.WithTenant(ctx, companyID, func(q *db.Queries) error {
		return q.RevokeRefreshToken(ctx, auth.HashToken(refreshToken))
	})
}

// issue firma el access token y crea el refresh token usando la Queries YA scopeada al tenant
// (q): CreateRefreshToken auto-sella company_id desde el GUC. Rellena el slug consultando la
// propia empresa (para mostrar user@slug en el front) — barato y evita threading del slug.
// issue arranca una sesión NUEVA con el plazo del negocio. Es el camino del login.
func (s *AuthService) issue(ctx context.Context, q *db.Queries, u db.User) (*Session, error) {
	return s.issueUntil(ctx, q, u, s.now().Add(s.duracionDeSesion(ctx, q)))
}

// duracionDeSesion: cuánto vive una sesión en este negocio.
//
// Sale del ajuste y no de una constante porque un local con turnos de 12 horas lo sube y otro que
// quiera más control lo baja. Sin fila de ajustes —empresa recién creada— cae a RefreshTokenTTL,
// que es el comportamiento que había antes de que el ajuste existiera: el modo de fallo deja
// entrar, no deja fuera.
func (s *AuthService) duracionDeSesion(ctx context.Context, q *db.Queries) time.Duration {
	ajustes, err := q.GetBusinessSettings(ctx)
	if err != nil || ajustes.SessionHours <= 0 {
		return RefreshTokenTTL
	}
	return time.Duration(ajustes.SessionHours) * time.Hour
}

// issueUntil arma la sesión con un vencimiento DADO. Existe para que el cambio de operador conserve
// el reloj del turno en vez de reponerlo.
func (s *AuthService) issueUntil(ctx context.Context, q *db.Queries, u db.User, vence time.Time) (*Session, error) {
	du := toDomainUser(u)
	if co, err := q.GetCompany(ctx, u.CompanyID); err == nil {
		du.CompanySlug = co.Slug
	}
	access, err := s.jwt.Issue(du)
	if err != nil {
		return nil, err
	}
	token, hash, err := auth.NewRefreshToken()
	if err != nil {
		return nil, err
	}
	if _, err := q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID:    u.ID,
		TokenHash: hash,
		ExpiresAt: vence,
	}); err != nil {
		return nil, err
	}
	return &Session{AccessToken: access, RefreshToken: token, CompanyID: u.CompanyID, User: du}, nil
}

func toDomainUser(u db.User) domain.User {
	return domain.User{
		ID:                 u.ID,
		CompanyID:          u.CompanyID,
		Name:               u.Name,
		Username:           u.Username,
		Role:               domain.Role(u.Role),
		IsActive:           u.IsActive,
		RecoveryEmail:      u.RecoveryEmail,
		MustChangePassword: u.MustChangePassword,
		CreatedAt:          u.CreatedAt,
	}
}

// UnlockOption es una persona que puede desbloquear una estación con su PIN.
// Solo id y nombre: la rejilla se pinta en una tableta a la vista del público.
type UnlockOption struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// UnlockOptions describe qué debe pedir la pantalla de bloqueo.
type UnlockOptions struct {
	PinOnly bool           `json:"pinOnly"`
	Users   []UnlockOption `json:"users"`
}

// UnlockOptions dice quiénes pueden desbloquear esta estación.
//
// Con el modo de solo-PIN encendido la lista viaja VACÍA: listar nombres le quitaría al modo su
// única ventaja —el tap que ahorra— y expondría la plantilla del negocio sin necesidad.
func (s *AuthService) UnlockOptions(ctx context.Context) (UnlockOptions, error) {
	ajustes, err := s.store.QC(ctx).GetBusinessSettings(ctx)
	// Sin fila de ajustes el negocio es nuevo: se cae al modo SEGURO, que pide elegir persona.
	pinOnly := err == nil && ajustes.PinOnlyUnlock

	out := UnlockOptions{PinOnly: pinOnly, Users: []UnlockOption{}}
	if pinOnly {
		return out, nil
	}
	filas, err := s.store.QC(ctx).UnlockCandidates(ctx)
	if err != nil {
		return UnlockOptions{}, err
	}
	for _, f := range filas {
		out.Users = append(out.Users, UnlockOption{ID: f.ID, Name: f.Name})
	}
	return out, nil
}

// PinSwitchSoloPin cambia de operador SIN que la pantalla diga quién es: lo deduce del PIN.
//
// Solo tiene sentido con el modo de solo-PIN encendido, donde el PIN identifica en vez de solo
// probar. La deducción usa la huella determinista; bcrypt no puede hacerla porque saliniza, y
// probar su hash contra cada usuario sería lento y filtraría por tiempo cuántos hay.
//
// La respuesta y la latencia son las mismas que las del camino normal: un PIN que no existe corre
// igual el bcrypt de descarte, para que no se pueda averiguar cuáles están en uso.
func (s *AuthService) PinSwitchSoloPin(ctx context.Context, pin string, actorID int64, refreshActual string) (*Session, error) {
	if s.pinPepper == "" {
		return nil, domain.ErrSinPepper
	}
	fila, err := s.store.QC(ctx).UserByPinLookup(ctx, ptr(domain.PinLookup(pin, s.pinPepper)))
	if err != nil {
		auth.CheckDummySecret(pin)
		return nil, domain.ErrInvalidCredentials
	}
	return s.PinSwitchEnEstacion(ctx, fila.ID, pin, actorID, refreshActual)
}

func ptr[T any](v T) *T { return &v }
