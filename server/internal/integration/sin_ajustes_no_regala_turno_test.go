//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// UNA EMPRESA SIN FILA DE AJUSTES NO PUEDE HEREDAR UNA SESIÓN DE 30 DÍAS.
//
// El respaldo de `duracionDeSesion` era `RefreshTokenTTL` —30 días—, así que un negocio recién
// provisionado arrancaba con el comportamiento que esta feature vino a quitar, y sin nada que lo
// dijera: la pantalla de ajustes mostraba ceros y el turno duraba un mes. El respaldo tiene que ser
// el MISMO default que trae la columna, que es lo que el negocio vería si abriera la pantalla.
func TestSinFilaDeAjustesLaSesionDuraElDefaultDelNegocio(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", clock)
	svc := app.NewAuthService(st, jm, clock)

	ana := makeUser(t, st, "ana_sin_ajustes", "cajero")
	hash, err := auth.HashSecret("Contrasena-Larga-1!")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, ana, hash); err != nil {
		t.Fatalf("set password: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `delete from business_settings`); err != nil {
		t.Fatalf("borrar los ajustes: %v", err)
	}

	sesion, err := svc.Login(ctx, "ana_sin_ajustes", "gatobobah", "Contrasena-Larga-1!")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	quiere := clock().Add(time.Duration(domain.DefaultIdentity().SessionHours) * time.Hour)
	if d := venceDe(t, st, sesion.RefreshToken).Sub(quiere); d > time.Minute || d < -time.Minute {
		t.Errorf("sin ajustes la sesión dura hasta %s y el default del negocio dice %s: el turno se alarga solo",
			venceDe(t, st, sesion.RefreshToken).Format(time.RFC3339), quiere.Format(time.RFC3339))
	}

	// Y la pantalla de ajustes tiene que mostrar ese mismo default, no ceros: con ceros el dueño
	// lee "sin bloqueo, sin caducidad" y no puede guardar hasta escribirlos a mano.
	ajustes, err := app.NewSettingsService(st, "pepper-de-prueba").Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ajustes.SessionHours != domain.DefaultIdentity().SessionHours ||
		ajustes.LockAfterSeconds != domain.DefaultIdentity().LockAfterSeconds {
		t.Errorf("los ajustes sin fila dicen bloqueo=%ds sesión=%dh, quiere el default del negocio",
			ajustes.LockAfterSeconds, ajustes.SessionHours)
	}
}
