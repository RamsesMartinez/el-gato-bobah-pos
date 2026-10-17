//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// countPlacesVersion es la migración de los lugares para contar (spec 040).
const countPlacesVersion = 95

// LA 0095 SOBRE UN RESPALDO REAL CON DOS EMPRESAS (constitución IV).
//
// Pasa el lugar escrito a mano de cada artículo a una lista por empresa. Lo que solo se ve sobre
// datos reales: que lo escrito con otra mayúscula o con espacios de más se junta, que el mismo texto
// en dos empresas no se junta, y que ningún artículo que tenía lugar se queda sin él.
func TestMigrationCountPlacesOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start < countPlacesVersion-1 {
		migrarArriba(t, st.Pool)
	}
	migrarAbajoHasta(t, st.Pool, countPlacesVersion-1)
	t.Cleanup(func() {
		if versionDeEsquema(t, st) < start {
			migrarArriba(t, st.Pool)
		}
	})

	// Tres artículos de una empresa con el mismo lugar escrito de tres formas, y uno de otra empresa
	// con el mismo texto. Se toman los de menor id de cada empresa.
	var a1, a2, a3, b1, coA, coB int64
	if err := st.Pool.QueryRow(ctx, `
		with c as (select company_id from ingredients group by company_id having count(*) >= 3 order by company_id limit 2),
		     r as (select i.id, i.company_id, row_number() over (partition by i.company_id order by i.id) n
		             from ingredients i join c using (company_id))
		select max(id) filter (where company_id = (select min(company_id) from c) and n = 1),
		       max(id) filter (where company_id = (select min(company_id) from c) and n = 2),
		       max(id) filter (where company_id = (select min(company_id) from c) and n = 3),
		       max(id) filter (where company_id = (select max(company_id) from c) and n = 1),
		       (select min(company_id) from c), (select max(company_id) from c)
		  from r`).Scan(&a1, &a2, &a3, &b1, &coA, &coB); err != nil {
		t.Fatal(err)
	}
	if coA == coB {
		t.Fatal("el respaldo necesita dos empresas con insumos")
	}
	if _, err := st.Pool.Exec(ctx, `update ingredients set count_place = null`); err != nil {
		t.Fatal(err)
	}
	for id, place := range map[int64]string{a1: "Refri", a2: "refri ", a3: "Refri", b1: "REFRI"} {
		if _, err := st.Pool.Exec(ctx, `update ingredients set count_place = $2 where id = $1`, id, place); err != nil {
			t.Fatal(err)
		}
	}
	var withPlace int64
	if err := st.Pool.QueryRow(ctx, `select count(*) from ingredients where count_place is not null`).Scan(&withPlace); err != nil {
		t.Fatal(err)
	}
	migrarArriba(t, st.Pool)

	t.Run("lo escrito con otra mayúscula o espacios se junta, y cada empresa tiene el suyo", func(t *testing.T) {
		rows, err := st.Pool.Query(ctx, `select company_id, name, count(*) over (partition by company_id) from count_places order by company_id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		got := map[int64]string{}
		for rows.Next() {
			var co, n int64
			var name string
			if err := rows.Scan(&co, &name, &n); err != nil {
				t.Fatal(err)
			}
			if n != 1 {
				t.Errorf("la empresa %d quedó con %d lugares; los textos solo diferían en mayúsculas y espacios", co, n)
			}
			got[co] = name
		}
		if got[coA] != "Refri" || got[coB] != "REFRI" {
			t.Fatalf("lugares = %v; quiere «Refri» (el más usado) en %d y «REFRI» en %d", got, coA, coB)
		}
	})

	t.Run("ningún artículo que tenía lugar se queda sin él, y cada uno apunta a uno de su empresa", func(t *testing.T) {
		var n, cruzados int64
		if err := st.Pool.QueryRow(ctx, `
			select count(*), count(*) filter (where p.company_id <> i.company_id)
			  from ingredients i join count_places p on p.id = i.count_place_id`).Scan(&n, &cruzados); err != nil {
			t.Fatal(err)
		}
		if n != withPlace || cruzados != 0 {
			t.Fatalf("%d artículos con lugar (antes %d), %d apuntando a otra empresa", n, withPlace, cruzados)
		}
	})

	t.Run("RLS con nullif, grants sin borrar, y un nombre repetido con otra mayúscula se rechaza", func(t *testing.T) {
		var rls, del bool
		var qual string
		if err := st.Pool.QueryRow(ctx, `
			select c.relrowsecurity, has_table_privilege('gatobobah_app', c.oid, 'DELETE'), pg_get_expr(p.polqual, p.polrelid)
			  from pg_class c join pg_policy p on p.polrelid = c.oid where c.relname = 'count_places'`).Scan(&rls, &del, &qual); err != nil {
			t.Fatal(err)
		}
		if !rls || del || !strings.Contains(strings.ToLower(qual), "nullif") {
			t.Fatalf("rls %v, delete %v, política %q", rls, del, qual)
		}
		if _, err := st.Pool.Exec(ctx, `insert into count_places (company_id, name, position) values ($1, ' refrí ', 9)`, coA); err == nil {
			t.Fatal("se creó « refrí » junto a «Refri»")
		}
	})

	t.Run("la base normaliza igual que el servidor", func(t *testing.T) {
		for _, name := range []string{"Ñandú", "Café  de  olla", "Barra fría", " ÚLTIMO "} {
			var sql string
			if err := st.Pool.QueryRow(ctx,
				`select regexp_replace(translate(lower(btrim($1)), 'áéíóúüñ', 'aeiouun'), '\s+', ' ', 'g')`,
				domain.CleanCountPlaceName(name)).Scan(&sql); err != nil {
				t.Fatal(err)
			}
			if want := domain.NormalizeReasonName(name); sql != want {
				t.Errorf("%q: la base dice %q y el servidor %q; el 409 no encontraría el que ya existe", name, sql, want)
			}
		}
	})

	t.Run("el Down regresa el nombre a cada artículo y pierde solo el orden", func(t *testing.T) {
		migrarAbajoHasta(t, st.Pool, countPlacesVersion-1)
		var refri, otro int64
		if err := st.Pool.QueryRow(ctx, `
			select count(*) filter (where count_place = 'Refri' and company_id = $1),
			       count(*) filter (where count_place = 'REFRI' and company_id = $2)
			  from ingredients`, coA, coB).Scan(&refri, &otro); err != nil {
			t.Fatal(err)
		}
		if refri != 3 || otro != 1 {
			t.Fatalf("tras el Down: %d con «Refri» y %d con «REFRI»; quiere 3 y 1", refri, otro)
		}
		var tbl bool
		if err := st.Pool.QueryRow(ctx, `select to_regclass('count_places') is not null`).Scan(&tbl); err != nil || tbl {
			t.Fatalf("la tabla sigue tras el Down: %v %v", tbl, err)
		}
		migrarArriba(t, st.Pool)
	})
}
