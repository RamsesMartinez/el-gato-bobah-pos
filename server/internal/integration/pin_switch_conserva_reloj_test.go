//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// EL HALLAZGO QUE SOSTIENE TODA LA FEATURE.
//
// PinSwitch emitía una sesión NUEVA con el plazo completo por delante. Con la pantalla de bloqueo
// usándolo, cada desbloqueo reiniciaría el reloj del turno: una tableta que se usa cada veinte
// minutos no caducaría nunca y `session_hours` sería decorativo.
//
// También explica algo que ya estaba pasando en producción: nada revocaba al emitir, y por eso un
// usuario llegó a tener 4 refresh tokens vivos, el más viejo de tres días antes.
func TestCambiarDeOperadorConservaElRelojDeLaSesion(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	// Reloj PROPIO y avanzable. Con el `clock` fijo del harness, un vencimiento recalculado da
	// exactamente el mismo instante que el original y el test pasaría con el defecto puesto —
	// que es justo lo que pasó al escribirlo.
	ahora := fixedNow
	reloj := func() time.Time { return ahora }
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", reloj)
	svc := app.NewAuthService(st, jm, reloj)

	ana := makeUser(t, st, "ana_reloj", "cajero")
	luis := makeUser(t, st, "luis_reloj", "cajero")
	hash, err := auth.HashSecret("Contrasena-Larga-1!")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, ana, hash); err != nil {
		t.Fatalf("set password: %v", err)
	}
	users := app.NewUsersService(st, nil, false)
	if err := users.SetPIN(ctx, luis, "4827"); err != nil {
		t.Fatalf("SetPIN: %v", err)
	}

	sesion, err := svc.Login(ctx, "ana_reloj", "gatobobah", "Contrasena-Larga-1!")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	vence := venceDe(t, st, sesion.RefreshToken)

	// Pasa una hora de turno antes del relevo. Si el vencimiento se recalculara, se correría esa
	// hora hacia adelante.
	ahora = ahora.Add(time.Hour)

	// Ana le pasa la estación a Luis: él se identifica con su PIN.
	tras, err := svc.PinSwitch(ctx, luis, "4827", ana)
	if err != nil {
		t.Fatalf("PinSwitch: %v", err)
	}
	if tras.User.ID != luis {
		t.Fatalf("el operador activo quedó en %d, quiere %d", tras.User.ID, luis)
	}

	nuevoVence := venceDe(t, st, tras.RefreshToken)
	if !nuevoVence.Equal(vence) {
		t.Errorf("el reloj se movió de %s a %s: cada desbloqueo estaría reiniciando el turno",
			vence.Format(time.RFC3339), nuevoVence.Format(time.RFC3339))
	}

	// Y el refresh de quien estaba queda revocado: si siguiera vivo, cada relevo dejaría una
	// credencial más suelta, que es exactamente lo que produjo las 4 sesiones de producción.
	var revocado bool
	if err := st.Pool.QueryRow(ctx,
		`select revoked_at is not null from refresh_tokens where token_hash = $1`,
		auth.HashToken(sesion.RefreshToken)).Scan(&revocado); err != nil {
		t.Fatalf("leer el token viejo: %v", err)
	}
	if !revocado {
		t.Error("el refresh de quien estaba sigue vivo: cada relevo deja una credencial suelta")
	}
}

func venceDe(t *testing.T, st *store.Store, refresh string) time.Time {
	t.Helper()
	var v time.Time
	if err := st.Pool.QueryRow(context.Background(),
		`select expires_at from refresh_tokens where token_hash = $1`,
		auth.HashToken(refresh)).Scan(&v); err != nil {
		t.Fatalf("leer el vencimiento: %v", err)
	}
	return v
}
