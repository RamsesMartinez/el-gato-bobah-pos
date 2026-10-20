//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// articleCategoryKeyVersion es la llave de la categoría con la empresa (revisión de la 042).
const articleCategoryKeyVersion = 97

// LA 0097 SOBRE UN RESPALDO REAL CON DOS EMPRESAS (constitución IV): la llave nueva no deja ligar un
// artículo a la categoría de otra empresa, y el Down regresa la llave de antes.
func TestMigrationArticleCategoryKeyOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	start := versionDeEsquema(t, st)
	if start < articleCategoryKeyVersion-1 {
		migrarArriba(t, st.Pool)
	}
	migrarAbajoHasta(t, st.Pool, articleCategoryKeyVersion-1)
	t.Cleanup(func() {
		if versionDeEsquema(t, st) < start {
			migrarArriba(t, st.Pool)
		}
	})

	var coA, coB, ingA int64
	if err := st.Pool.QueryRow(ctx, `
		with c as (select company_id from ingredients group by company_id order by company_id limit 2)
		select (select min(company_id) from c), (select max(company_id) from c),
		       (select min(id) from ingredients where company_id = (select min(company_id) from c))`).Scan(&coA, &coB, &ingA); err != nil {
		t.Fatal(err)
	}
	if coA == coB {
		t.Fatal("el respaldo necesita dos empresas con artículos")
	}
	var catB int64
	if err := st.Pool.QueryRow(ctx, `insert into ingredient_categories (company_id, name) values ($1, 'Categoría 0097') returning id`, coB).Scan(&catB); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = st.Pool.Exec(context.Background(), `delete from ingredient_categories where id = $1`, catB)
	})

	t.Run("con un artículo ligado a la categoría de otra empresa, la subida se niega", func(t *testing.T) {
		if _, err := st.Pool.Exec(ctx, `update ingredients set category_id = $2 where id = $1`, ingA, catB); err != nil {
			t.Fatal(err)
		}
		if err := migrarAbajoHastaErr(t, st.Pool, articleCategoryKeyVersion-1); err != nil {
			t.Fatal(err)
		}
		if err := migrarArribaErr(t, st.Pool); err == nil {
			t.Fatal("la 0097 subió con un artículo ligado a la categoría de otra empresa")
		}
		if _, err := st.Pool.Exec(ctx, `update ingredients set category_id = null where id = $1`, ingA); err != nil {
			t.Fatal(err)
		}
	})
	migrarArriba(t, st.Pool)

	t.Run("ya no se puede ligar a la categoría de otra empresa", func(t *testing.T) {
		_, err := st.Pool.Exec(ctx, `update ingredients set category_id = $2 where id = $1`, ingA, catB)
		exigeViolacionDeRestriccion(t, err, "ligar un artículo a la categoría de otra empresa")
	})
	t.Run("el índice de renglones de conteo por artículo existe y empieza por la empresa", func(t *testing.T) {
		var def string
		if err := st.Pool.QueryRow(ctx, `select indexdef from pg_indexes where indexname = 'stock_count_lines_by_ingredient'`).Scan(&def); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(def, "(company_id, ingredient_id)") {
			t.Fatalf("índice = %s", def)
		}
	})
	t.Run("ida y vuelta", func(t *testing.T) {
		migrarAbajoHasta(t, st.Pool, articleCategoryKeyVersion-1)
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from pg_constraint where conname = 'ingredients_category_id_fkey'`).Scan(&n); err != nil || n != 1 {
			t.Fatalf("tras el Down la llave de antes = %d %v", n, err)
		}
		migrarArriba(t, st.Pool)
	})
}

// migrarArribaErr es migrarArriba que devuelve el error: la subida que se niega es lo que se prueba.
func migrarArribaErr(t *testing.T, pool *pgxpool.Pool) error {
	t.Helper()
	return goose.UpContext(context.Background(), prepararGoose(t, pool), ".")
}
