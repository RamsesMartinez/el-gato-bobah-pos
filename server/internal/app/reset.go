package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/hibp"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/mailer"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// PasswordResetTTL: los tokens de recuperación caducan pronto (menor ventana de robo del email).
const PasswordResetTTL = time.Hour

// ResetService orquesta la recuperación de contraseña por email. Endpoints PÚBLICOS (sin auth),
// así que corre bajo WithTenant EXPLÍCITO (resuelto por slug en request, por cid del link en
// confirm) y usa el `q` de la tx, no QC(ctx).
type ResetService struct {
	store       *store.Store
	mail        *mailer.Mailer
	hibp        *hibp.Client
	hibpEnabled bool
	baseURL     string
	now         func() time.Time
}

func NewResetService(s *store.Store, m *mailer.Mailer, hc *hibp.Client, hibpEnabled bool, baseURL string, now func() time.Time) *ResetService {
	if now == nil {
		now = time.Now
	}
	return &ResetService{store: s, mail: m, hibp: hc, hibpEnabled: hibpEnabled, baseURL: baseURL, now: now}
}

// Request genera y envía un enlace de recuperación para username@slug. Anti-enumeración: SIEMPRE
// termina sin error visible; si la empresa/usuario no existe, o el usuario no tiene email de
// recuperación, o el email está deshabilitado, simplemente no se envía nada (el handler responde
// 200 igual). El link embebe el cid como la cookie de refresh (cid.token) para fijar el tenant.
func (s *ResetService) Request(ctx context.Context, slug, username string) error {
	if !s.mail.Enabled() {
		return nil // email deshabilitado: el admin resetea a mano
	}
	companyID, err := s.store.Q.ResolveCompanyBySlug(ctx, slug)
	if err != nil || companyID == 0 {
		return err // nil si simplemente no existe (no revela nada)
	}
	// Solo trabajo de BD (rápido) en el camino del request; el ENVÍO SMTP (lento, segundos) se
	// dispara aparte para que /forgot responda en tiempo ~constante exista o no la cuenta/email
	// → cierra el oráculo de temporización de enumeración (§5 anti-enumeración).
	var toEmail, name, link string
	err = s.store.WithTenant(ctx, companyID, func(q *db.Queries) error {
		u, err := q.GetUserByUsername(ctx, &username)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil // usuario inexistente → no-op silencioso
			}
			return err
		}
		if u.RecoveryEmail == nil || *u.RecoveryEmail == "" {
			logging.SecurityEvent(ctx, "password_reset_no_email", "user_id", u.ID)
			return nil // sin email registrado → no-op (el admin lo resetea)
		}
		token, hash, err := auth.NewRefreshToken() // token opaco genérico (32 bytes) + sha256
		if err != nil {
			return err
		}
		if _, err := q.CreatePasswordResetToken(ctx, db.CreatePasswordResetTokenParams{
			UserID: u.ID, TokenHash: hash, ExpiresAt: s.now().Add(PasswordResetTTL),
		}); err != nil {
			return err
		}
		toEmail, name = *u.RecoveryEmail, u.Name
		link = fmt.Sprintf("%s/reset?token=%d.%s", s.baseURL, companyID, token)
		logging.SecurityEvent(ctx, "password_reset_requested", "user_id", u.ID)
		return nil
	})
	if err != nil {
		return err
	}
	if toEmail != "" {
		// El envío corre ASÍNCRONO y a propósito con context.Background: sobrevive al request
		// (si usara su ctx, se cancelaría al responder el 204). Término garantizado: mailer.Send
		// está acotado por dial-timeout + deadline de conexión.
		go func() { //nolint:gosec // G118: Background intencional; el envío desacoplado del request
			if err := s.mail.Send(toEmail, "Recupera tu contraseña · El Gato Bobah", resetEmailHTML(name, link)); err != nil {
				logging.SecurityEvent(context.Background(), "password_reset_email_failed", "error", err.Error())
			}
		}()
	}
	return nil
}

// Confirm valida el token (no usado, no vencido) y fija la nueva contraseña. companyID viene del
// link (cid.token). Al terminar: marca el token usado, invalida otros pendientes y revoca todos
// los refresh del usuario (cierra sesiones activas tras un reset).
func (s *ResetService) Confirm(ctx context.Context, companyID int64, token, newPassword string) error {
	if err := checkPasswordStrength(ctx, newPassword, s.hibp, s.hibpEnabled); err != nil {
		return err
	}
	hash := auth.HashToken(token)
	return s.store.WithTenant(ctx, companyID, func(q *db.Queries) error {
		rt, err := q.GetPasswordResetToken(ctx, hash)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrResetInvalid // token inexistente para este tenant
			}
			return err
		}
		if rt.UsedAt.Valid || s.now().After(rt.ExpiresAt) {
			return domain.ErrResetInvalid // ya usado o vencido
		}
		pwHash, err := auth.HashSecret(newPassword)
		if err != nil {
			return err
		}
		if err := q.SetUserPassword(ctx, db.SetUserPasswordParams{ID: rt.UserID, PasswordHash: &pwHash, MustChangePassword: false}); err != nil {
			return err
		}
		if err := q.MarkPasswordResetTokenUsed(ctx, rt.ID); err != nil {
			return err
		}
		if err := q.InvalidateUserResetTokens(ctx, rt.UserID); err != nil {
			return err
		}
		// Fuerza re-login en todos los dispositivos tras el reset (posible compromiso).
		if err := q.RevokeUserRefreshTokens(ctx, rt.UserID); err != nil {
			return err
		}
		logging.SecurityEvent(ctx, "password_reset_completed", "user_id", rt.UserID)
		return nil
	})
}

func resetEmailHTML(name, link string) string {
	return fmt.Sprintf(`<div style="font-family:system-ui,sans-serif;max-width:480px;margin:0 auto">
<h2>Recupera tu contraseña</h2>
<p>Hola %s, recibimos una solicitud para restablecer tu contraseña.</p>
<p><a href="%s" style="display:inline-block;background:#E23B2E;color:#fff;padding:12px 20px;border-radius:8px;text-decoration:none">Elegir nueva contraseña</a></p>
<p style="color:#666;font-size:13px">El enlace caduca en 1 hora. Si no fuiste tú, ignora este correo: tu contraseña no cambia.</p>
</div>`, name, link)
}
