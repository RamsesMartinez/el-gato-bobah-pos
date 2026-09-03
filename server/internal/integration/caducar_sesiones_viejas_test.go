//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// LA MIGRACIÓN 0052, PROBADA SOBRE LO QUE HAY EN PRODUCCIÓN.
//
// El arreglo de la rotación conserva el vencimiento del refresh, y eso es correcto — pero las
// credenciales emitidas ANTES de 0050 traen 30 días. Sin caducarlas, la tableta que ya estaba
// dentro se queda con un mes de sesión y el límite de horas no aplica a nadie hasta que cada una
// vuelva a entrar por su cuenta, semanas después. En producción hay una persona con cuatro vivas.
//
// Se prueba con DOS empresas: con una sola, cualquier defecto de alcance —una migración que solo
// toca la empresa "actual"— es un no-op y pasa verde para romper en producción.
func TestLaMigracionCaducaLasSesionesDeTodasLasEmpresas(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	otra := makeCompany(t, st, "otra-empresa-0052")
	ana := makeUser(t, st, "ana_0052", "cajero")
	beto := makeUserIn(t, st, otra, "beto_0052", "cajero")

	// Sesiones como las de antes de 0050: 30 días.
	viejo := func(u int64) string {
		t.Helper()
		token, hash, err := auth.NewRefreshToken()
		if err != nil {
			t.Fatalf("NewRefreshToken: %v", err)
		}
		if _, err := st.Q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
			UserID: u, TokenHash: hash, ExpiresAt: clock().Add(30 * 24 * time.Hour),
		}); err != nil {
			t.Fatalf("CreateRefreshToken: %v", err)
		}
		return token
	}
	deAna, deBeto := viejo(ana), viejo(beto)

	// Se corre el SQL DEL ARCHIVO, no una copia escrita aquí: goose ya había migrado al arrancar el
	// store, antes de que estas filas existieran, así que hay que volver a aplicarla sobre datos
	// previos. Leerla del archivo es lo que hace que este test siga a la migración si alguien la
	// edita; una copia se quedaría probando la versión de ayer.
	if _, err := st.Pool.Exec(ctx, sqlDeLaMigracion(t, "0052_caducar_las_sesiones_viejas.sql")); err != nil {
		t.Fatalf("correr la migración: %v", err)
	}

	for _, c := range []struct {
		quien string
		token string
	}{{"la empresa por default", deAna}, {"la otra empresa", deBeto}} {
		// Se comprueba el EFECTO —que la credencial ya no sirva— y no el mecanismo. La migración
		// caduca en vez de revocar a propósito: revocar clasifica como ROBO el siguiente refresh y
		// arrastra las sesiones vivas de la misma cuenta en las otras estaciones (ver
		// dos_estaciones_no_se_tumban_test.go). Assertar "revoked_at is not null" ataba este test al
		// mecanismo y habría bloqueado esa corrección.
		var sigueViva bool
		if err := st.Pool.QueryRow(ctx,
			`select revoked_at is null and expires_at > now() from refresh_tokens where token_hash = $1`,
			auth.HashToken(c.token)).Scan(&sigueViva); err != nil {
			t.Fatalf("leer el token de %s: %v", c.quien, err)
		}
		if sigueViva {
			t.Errorf("la sesión de 30 días de %s sigue viva: esa tableta se queda un mes dentro y el límite de horas no le aplica", c.quien)
		}
	}

	// Y las filas NO se borran: el histórico es lo que deja ver un reuso más adelante.
	var filas int
	if err := st.Pool.QueryRow(ctx, `select count(*) from refresh_tokens`).Scan(&filas); err != nil {
		t.Fatalf("contar: %v", err)
	}
	if filas < 2 {
		t.Errorf("quedaron %d filas de refresh; borrarlas ciega la detección de reuso", filas)
	}
}

// sqlDeLaMigracion devuelve el bloque Up del archivo de migración.
func sqlDeLaMigracion(t *testing.T, nombre string) string {
	t.Helper()
	crudo, err := os.ReadFile(filepath.Join("..", "..", "migrations", nombre))
	if err != nil {
		t.Fatalf("leer la migración: %v", err)
	}
	_, resto, ok := strings.Cut(string(crudo), "-- +goose Up")
	if !ok {
		t.Fatalf("%s no tiene bloque Up", nombre)
	}
	up, _, _ := strings.Cut(resto, "-- +goose Down")
	if strings.TrimSpace(up) == "" {
		t.Fatalf("%s tiene el bloque Up vacío", nombre)
	}
	return up
}
