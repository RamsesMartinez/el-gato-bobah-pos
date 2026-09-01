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
	users := app.NewUsersService(st, nil, false, "pepper-de-prueba")
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
	tras, err := svc.PinSwitchEnEstacion(ctx, luis, "4827", ana, sesion.RefreshToken)
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

// EL RELOJ ES DE LA ESTACIÓN, NO DE LA PERSONA.
//
// `LatestLiveRefreshExpiry` tomaba el vencimiento MÁS LEJANO de cualquier sesión viva de esa
// persona, así que una tableta heredaba el reloj de otra. Caso concreto: Ana entra a las 08:00 en
// la estación 1 (vence 16:00) y a las 15:00 entra fresca en la estación 2 (vence 23:00). Al
// desbloquear la estación 1 a las 15:30, esa estación saltaba de 16:00 a 23:00 — siete horas de
// sesión regaladas con un PIN, y repetible.
func TestElRelojEsDeLaEstacionYNoDeLaPersona(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ahora := fixedNow
	reloj := func() time.Time { return ahora }
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", reloj)
	svc := app.NewAuthService(st, jm, reloj)
	users := app.NewUsersService(st, nil, false, "pepper-de-prueba")

	ana := makeUser(t, st, "ana_dos_estaciones", "cajero")
	luis := makeUser(t, st, "luis_dos_estaciones", "cajero")
	hash, _ := auth.HashSecret("Contrasena-Larga-1!")
	if _, err := st.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, ana, hash); err != nil {
		t.Fatalf("set password: %v", err)
	}
	if err := users.SetPIN(ctx, luis, "4827"); err != nil {
		t.Fatalf("SetPIN: %v", err)
	}

	// Estación 1, temprano.
	estacion1, err := svc.Login(ctx, "ana_dos_estaciones", "gatobobah", "Contrasena-Larga-1!")
	if err != nil {
		t.Fatalf("Login estación 1: %v", err)
	}
	vence1 := venceDe(t, st, estacion1.RefreshToken)

	// Siete horas después, la misma persona entra fresca en la estación 2: su sesión vence mucho
	// más tarde.
	ahora = ahora.Add(7 * time.Hour)
	if _, err := svc.Login(ctx, "ana_dos_estaciones", "gatobobah", "Contrasena-Larga-1!"); err != nil {
		t.Fatalf("Login estación 2: %v", err)
	}

	// Media hora después Ana entrega la ESTACIÓN 1 a Luis.
	ahora = ahora.Add(30 * time.Minute)
	tras, err := svc.PinSwitchEnEstacion(ctx, luis, "4827", ana, estacion1.RefreshToken)
	if err != nil {
		t.Fatalf("PinSwitch: %v", err)
	}

	if nuevo := venceDe(t, st, tras.RefreshToken); !nuevo.Equal(vence1) {
		t.Errorf("la estación 1 pasó de vencer %s a %s: heredó el reloj de la otra tableta",
			vence1.Format(time.RFC3339), nuevo.Format(time.RFC3339))
	}
}

// Y el relevo en UNA estación no puede tumbar la sesión de esa persona en las DEMÁS.
//
// Se revocaban todos los refresh vivos del actor, así que entregar la estación 1 dejaba al
// compañero de la estación 2 con "Terminó el turno" a media venta. El modo de fallo del resto de
// la feature es "deja trabajar"; este era el contrario.
func TestEntregarUnaEstacionNoTumbaLasDemas(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ahora := fixedNow
	reloj := func() time.Time { return ahora }
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", reloj)
	svc := app.NewAuthService(st, jm, reloj)
	users := app.NewUsersService(st, nil, false, "pepper-de-prueba")

	ana := makeUser(t, st, "ana_no_tumba", "cajero")
	luis := makeUser(t, st, "luis_no_tumba", "cajero")
	hash, _ := auth.HashSecret("Contrasena-Larga-1!")
	if _, err := st.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, ana, hash); err != nil {
		t.Fatalf("set password: %v", err)
	}
	if err := users.SetPIN(ctx, luis, "4827"); err != nil {
		t.Fatalf("SetPIN: %v", err)
	}

	estacion1, _ := svc.Login(ctx, "ana_no_tumba", "gatobobah", "Contrasena-Larga-1!")
	estacion2, _ := svc.Login(ctx, "ana_no_tumba", "gatobobah", "Contrasena-Larga-1!")

	if _, err := svc.PinSwitchEnEstacion(ctx, luis, "4827", ana, estacion1.RefreshToken); err != nil {
		t.Fatalf("PinSwitch: %v", err)
	}

	// La estación 2 sigue viva: Ana sigue trabajando ahí.
	if _, err := svc.Refresh(ctx, defaultCompanyID, estacion2.RefreshToken); err != nil {
		t.Errorf("entregar la estación 1 tumbó la sesión de la estación 2: %v", err)
	}
	// Y la 1 sí quedó revocada: era la que se entregó.
	if _, err := svc.Refresh(ctx, defaultCompanyID, estacion1.RefreshToken); err == nil {
		t.Error("la sesión de la estación entregada sigue viva")
	}
}
