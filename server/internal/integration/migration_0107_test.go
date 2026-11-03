//go:build integration

package integration

import (
	"context"
	"testing"
)

// altNamesVersion agrega los nombres alternativos de productos y artículos (spec 063).
const altNamesVersion = 107

// LA 0107 SOBRE UN RESPALDO REAL CON DOS EMPRESAS (constitución IV).
//
// Solo agrega dos columnas con default y una función. Lo que solo se ve sobre datos reales: que
// ninguna fila cambia, que la API anterior —que no nombra la columna— sigue creando y editando
// productos e ingredientes, y que el check impide lo que tumbaría el menú de toda la empresa.
func TestMigrationAltNamesOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	exigeDosEmpresas(t, st)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start < altNamesVersion-1 {
		migrarArriba(t, st.Pool)
	}
	migrarAbajoHasta(t, st.Pool, altNamesVersion-1)
	t.Cleanup(func() {
		if versionDeEsquema(t, st) < start {
			migrarArriba(t, st.Pool)
		}
	})
	fingerprint := func(t *testing.T) (string, string) {
		t.Helper()
		var p, i string
		if err := st.Pool.QueryRow(ctx, `select md5(string_agg(id || name || price || is_active, ',' order by id)) from products`).Scan(&p); err != nil {
			t.Fatal(err)
		}
		if err := st.Pool.QueryRow(ctx, `select md5(string_agg(id || name || is_active, ',' order by id)) from ingredients`).Scan(&i); err != nil {
			t.Fatal(err)
		}
		return p, i
	}
	p0, i0 := fingerprint(t)
	migrarArriba(t, st.Pool)

	t.Run("ninguna fila cambia y todas nacen sin alternativos", func(t *testing.T) {
		p1, i1 := fingerprint(t)
		if p1 != p0 || i1 != i0 {
			t.Fatal("la migración reescribió productos o ingredientes")
		}
		var withAlts int64
		if err := st.Pool.QueryRow(ctx, `select (select count(*) from products where alt_names <> '{}') + (select count(*) from ingredients where alt_names <> '{}')`).Scan(&withAlts); err != nil || withAlts != 0 {
			t.Fatalf("%d filas nacieron con alternativos (%v)", withAlts, err)
		}
	})

	var company, category int64
	if err := st.Pool.QueryRow(ctx, `select company_id, category_id from products where system_kind is null order by id limit 1`).Scan(&company, &category); err != nil {
		t.Fatal(err)
	}

	t.Run("la API anterior crea y edita sin nombrar la columna", func(t *testing.T) {
		var pid, iid int64
		if err := st.Pool.QueryRow(ctx, `insert into products (company_id, name, category_id, price) values ($1, 'Prueba 0107', $2, 10) returning id`, company, category).Scan(&pid); err != nil {
			t.Fatalf("el alta vieja de producto truena: %v", err)
		}
		if _, err := st.Pool.Exec(ctx, `update products set name = 'Prueba 0107 b', price = 11 where id = $1`, pid); err != nil {
			t.Fatalf("la edición vieja de producto truena: %v", err)
		}
		if err := st.Pool.QueryRow(ctx, `insert into ingredients (company_id, name, base_unit_id) values ($1, 'Insumo 0107', (select id from units where code = 'g')) returning id`, company).Scan(&iid); err != nil {
			t.Fatalf("el alta vieja de ingrediente truena: %v", err)
		}
		var alts string
		if err := st.Pool.QueryRow(ctx, `select alt_names::text from products where id = $1`, pid).Scan(&alts); err != nil || alts != "{}" {
			t.Fatalf("el producto nuevo quedó con %q (%v)", alts, err)
		}
	})

	t.Run("el check rechaza 11, un nulo y un vacío", func(t *testing.T) {
		for _, bad := range []string{
			`array['a','b','c','d','e','f','g','h','i','j','k']`,
			`array['a', null]`,
			`array['a', '']`,
		} {
			_, err := st.Pool.Exec(ctx, `update products set alt_names = `+bad+` where id = (select min(id) from products where system_kind is null)`)
			exigeViolacionDeRestriccion(t, err, "productos con "+bad)
			_, err = st.Pool.Exec(ctx, `update ingredients set alt_names = `+bad+` where id = (select min(id) from ingredients)`)
			exigeViolacionDeRestriccion(t, err, "ingredientes con "+bad)
		}
	})

	t.Run("bajar quita columnas y función y deja las filas", func(t *testing.T) {
		if _, err := st.Pool.Exec(ctx, `update products set alt_names = array['Otro nombre 0107'] where id = (select min(id) from products where system_kind is null)`); err != nil {
			t.Fatal(err)
		}
		var before int64
		if err := st.Pool.QueryRow(ctx, `select count(*) from products`).Scan(&before); err != nil {
			t.Fatal(err)
		}
		migrarAbajoHasta(t, st.Pool, altNamesVersion-1)
		var after int64
		var fn bool
		if err := st.Pool.QueryRow(ctx, `select count(*), exists (select 1 from pg_proc where proname = 'catalog_name_key') from products`).Scan(&after, &fn); err != nil {
			t.Fatal(err)
		}
		if after != before || fn {
			t.Fatalf("productos %d→%d, función viva: %v", before, after, fn)
		}
		migrarArriba(t, st.Pool)
	})
}
