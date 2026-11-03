//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// TestCatalogNameKeyMatchesGo: la llave con que la base decide un choque es la misma que la de Go.
// Si difieren, la pantalla dice «se puede» y la base dice «choca» (o al revés). Los casos son los
// que ya mordieron: el tabulador en la orilla que `btrim` no recortaba y el espacio duro, que el
// `\s` de Go no ve y el de Postgres a veces sí.
func TestCatalogNameKeyMatchesGo(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	// Con standard_conforming_strings apagado también: el cuerpo de la función se vuelve a leer en
	// cada llamada con el ajuste de la sesión que llama (revisión del db-architect).
	for _, conforming := range []string{"on", "off"} {
		conn, err := st.Pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, "set standard_conforming_strings = "+conforming); err != nil {
			t.Fatal(err)
		}
		checkCatalogKeyParity(t, conforming, func(in string) (string, error) {
			var out string
			err := conn.QueryRow(ctx, `select catalog_name_key($1)`, in).Scan(&out)
			return out, err
		})
		_, _ = conn.Exec(ctx, "reset standard_conforming_strings")
		conn.Release()
	}
}

func checkCatalogKeyParity(t *testing.T, conforming string, key func(string) (string, error)) {
	t.Helper()
	for _, in := range []string{
		"Café Frío", "  café   frío ", "Cafe\tfrio", "Café\t", "\nCafé\n", "café frío",
		" café ", "PIÑA Ü", "Leche  Lala", "\t\n ", "ÁÉÍÓÚ",
	} {
		sql, err := key(in)
		if err != nil {
			t.Fatal(err)
		}
		if goKey := domain.CatalogNameKey(in); sql != goKey {
			t.Errorf("standard_conforming_strings=%s, %q: la base dice %q y Go dice %q", conforming, in, sql, goKey)
		}
	}
}
