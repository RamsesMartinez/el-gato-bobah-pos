//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// DOS ESTACIONES EN LA MISMA CUENTA NO PUEDEN TUMBARSE ENTRE ELLAS.
//
// El defecto que esto cierra habría dejado el POS pidiendo contraseña cada quince minutos, para
// siempre, y no se veía en ninguna prueba porque hacen falta TRES piezas a la vez:
//
//  1. La migración 0052 marcaba las sesiones vivas como REVOCADAS para forzar un re-login.
//  2. `ClassifyRefresh` mira `revoked` ANTES que `expiresAt`, así que una credencial revocada que
//     reaparece es, por definición, un robo.
//  3. Y el castigo del robo es `RevokeUserRefreshTokens`, que revoca por `user_id` — TODAS las
//     sesiones de esa persona, en todas las tabletas, no solo la cadena comprometida.
//
// En este negocio dos estaciones comparten cuenta (lo dice el comentario de 0050). Con las tres
// piezas juntas: la tableta A entra con contraseña, la B despierta con su cookie vieja y le revoca
// la sesión a A, A refresca y le revoca la sesión a B. Un ping-pong con periodo de quince minutos
// que no converge mientras las dos se usen.
//
// La corrección es que el re-login masivo CADUQUE en vez de REVOCAR: caducar es "tu turno terminó"
// y da un 401 limpio; revocar es "alguien te robó la credencial" y dispara la respuesta de robo.
func TestDosEstacionesConLaMismaCuentaNoSeRevocanEntreEllas(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewAuthService(st, nil, nil)

	usuario := makeUser(t, st, "cajero_dos_estaciones", "cajero")

	// Dos sesiones vivas de la MISMA persona, como las dos tabletas del mostrador.
	tokenA := emitirRefresh(t, st, usuario, time.Now().Add(8*time.Hour))
	tokenB := emitirRefresh(t, st, usuario, time.Now().Add(8*time.Hour))

	// Lo que hace el re-login masivo de la migración: las deja fuera. CADUCADAS, no revocadas.
	caducarSesionesVivas(t, st)

	// La estación B despierta y presenta su cookie vieja. Tiene que rebotar…
	if _, err := svc.Refresh(ctx, defaultCompanyID, tokenB); err == nil {
		t.Fatal("una sesión caducada debe rebotar, no renovarse")
	}

	// …y ese rebote NO puede haber tocado la sesión de la otra tableta.
	vivas := refreshVivos(t, st, usuario)
	if vivas != 0 {
		t.Fatalf("quedaron %d sesiones vivas tras caducarlas todas; el estado de partida está mal", vivas)
	}
	revocadas := refreshRevocados(t, st, usuario)
	if revocadas != 0 {
		t.Fatalf("el rebote de una sesión CADUCADA revocó %d credenciales: se está tratando como robo "+
			"un turno que simplemente terminó, y eso tumba a la otra estación de la misma cuenta", revocadas)
	}
	_ = tokenA
}

// Y el reuso de verdad SÍ tiene que seguir castigándose: la corrección no puede aflojar la
// detección de robo, que es un control de seguridad del principio V.
func TestUnReusoDeVerdadSigueRevocando(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewAuthService(st, nil, nil)

	usuario := makeUser(t, st, "cajero_reuso_real", "cajero")
	token := emitirRefresh(t, st, usuario, time.Now().Add(8*time.Hour))
	revocarUno(t, st, token)

	if _, err := svc.Refresh(ctx, defaultCompanyID, token); err == nil {
		t.Fatal("una credencial revocada que reaparece debe rebotar")
	}
	if refreshRevocados(t, st, usuario) == 0 {
		t.Fatal("un reuso real debe seguir revocando: aflojar esto abre la puerta que el principio V cierra")
	}
	_ = domain.RefreshReused
}

// ---- helpers ----

// emitirRefresh siembra una sesión viva y devuelve el token en claro, como lo tendría la tableta.
func emitirRefresh(t *testing.T, st *store.Store, userID int64, vence time.Time) string {
	t.Helper()
	token, hash, err := auth.NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken: %v", err)
	}
	if _, err := st.Pool.Exec(context.Background(),
		`insert into refresh_tokens (company_id, user_id, token_hash, expires_at)
		 values ($1, $2, $3, $4)`, defaultCompanyID, userID, hash, vence); err != nil {
		t.Fatalf("sembrar refresh: %v", err)
	}
	return token
}

// caducarSesionesVivas corre LA SENTENCIA DE LA MIGRACIÓN 0052, leída del archivo.
//
// No una reimplementación: si aquí se copiara el `update` a mano, el test probaría la copia y la
// migración podría decir otra cosa. Leyéndola del disco, cambiar 0052 mueve este test.
func caducarSesionesVivas(t *testing.T, st *store.Store) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(),
		sqlDeLaMigracion(t, "0052_caducar_las_sesiones_viejas.sql")); err != nil {
		t.Fatalf("correr 0052: %v", err)
	}
}

func revocarUno(t *testing.T, st *store.Store, token string) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(),
		`update refresh_tokens set revoked_at = now() where token_hash = $1`,
		auth.HashToken(token)); err != nil {
		t.Fatalf("revocar: %v", err)
	}
}

func refreshVivos(t *testing.T, st *store.Store, userID int64) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(),
		`select count(*) from refresh_tokens
		  where user_id = $1 and revoked_at is null and expires_at > now()`, userID).Scan(&n); err != nil {
		t.Fatalf("contar vivos: %v", err)
	}
	return n
}

func refreshRevocados(t *testing.T, st *store.Store, userID int64) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(),
		`select count(*) from refresh_tokens where user_id = $1 and revoked_at is not null`,
		userID).Scan(&n); err != nil {
		t.Fatalf("contar revocados: %v", err)
	}
	return n
}
