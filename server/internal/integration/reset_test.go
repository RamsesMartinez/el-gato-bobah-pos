//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/mailer"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// El flujo de confirmación de reset es un control de seguridad: token de un solo uso, con TTL,
// que al usarse fija la contraseña, invalida el token y REVOCA todas las sesiones del usuario.
// Se prueba el Confirm directamente (sin SMTP) creando el token en BD bajo el tenant.
func TestPasswordResetConfirm(t *testing.T) {
	owner := newTestStore(t)
	appSt := appRoleStore(t)
	uid := makeUser(t, owner, "reset_me", "cajero")
	// password inicial + un refresh token vivo (para verificar que el reset lo revoca).
	pwHash, _ := auth.HashSecret("Contrasena-Inicial-1")
	if _, err := owner.Pool.Exec(context.Background(),
		`update users set password_hash=$2 where id=$1`, uid, pwHash); err != nil {
		t.Fatalf("set pw: %v", err)
	}
	if _, err := owner.Pool.Exec(context.Background(),
		`insert into refresh_tokens (company_id, user_id, token_hash, expires_at)
		 values ($1,$2,'live-token-hash', now()+interval '30 days')`, defaultCompanyID, uid); err != nil {
		t.Fatalf("seed refresh: %v", err)
	}

	// Token de reset válido (creado como lo haría el servicio, bajo el tenant).
	token, hash, _ := auth.NewRefreshToken()
	if err := appSt.WithTenant(context.Background(), defaultCompanyID, func(q *db.Queries) error {
		_, e := q.CreatePasswordResetToken(context.Background(), db.CreatePasswordResetTokenParams{
			UserID: uid, TokenHash: hash, ExpiresAt: fixedNow.Add(app.PasswordResetTTL),
		})
		return e
	}); err != nil {
		t.Fatalf("crear token: %v", err)
	}

	svc := app.NewResetService(appSt, mailer.New("", 0, "", "", ""), nil, false, "http://x", clock)
	ctx := context.Background()

	// Contraseña débil → rechazada (política aplicada también en el reset).
	if err := svc.Confirm(ctx, defaultCompanyID, token, "corta"); !errors.Is(err, domain.ErrWeakPassword) {
		t.Fatalf("contraseña débil debe dar ErrWeakPassword, got %v", err)
	}
	// Confirmación válida.
	if err := svc.Confirm(ctx, defaultCompanyID, token, "Contrasena-Nueva-2026"); err != nil {
		t.Fatalf("Confirm válido: %v", err)
	}
	// Un solo uso: reintentar el mismo token falla con el error accionable de reset.
	if err := svc.Confirm(ctx, defaultCompanyID, token, "Otra-Contrasena-2026"); !errors.Is(err, domain.ErrResetInvalid) {
		t.Fatalf("token ya usado debe dar ErrResetInvalid, got %v", err)
	}

	// Efectos en BD: la contraseña cambió y el refresh vivo quedó revocado.
	var newHash string
	var mustChange bool
	if err := owner.Pool.QueryRow(context.Background(),
		`select password_hash, must_change_password from users where id=$1`, uid).Scan(&newHash, &mustChange); err != nil {
		t.Fatalf("leer user: %v", err)
	}
	if !auth.CheckSecret(newHash, "Contrasena-Nueva-2026") {
		t.Fatal("la contraseña no se actualizó a la nueva")
	}
	if mustChange {
		t.Fatal("tras un reset auto-servicio must_change_password debe quedar en false")
	}
	var liveTokens int
	if err := owner.Pool.QueryRow(context.Background(),
		`select count(*) from refresh_tokens where user_id=$1 and revoked_at is null`, uid).Scan(&liveTokens); err != nil {
		t.Fatalf("contar refresh: %v", err)
	}
	if liveTokens != 0 {
		t.Fatalf("el reset debe revocar todas las sesiones; quedan %d refresh vivos", liveTokens)
	}
}
