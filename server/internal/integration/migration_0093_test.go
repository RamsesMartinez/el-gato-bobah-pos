//go:build integration

package integration

import (
	"context"
	"testing"
)

// catalogVersionsVersion es la migración de la versión de catálogo (spec 034). Se escribió como
// 0089 y se movió a 0093 al fusionar con develop; si vuelve a moverse, esta constante con ella.
const catalogVersionsVersion = 93

// LA VERSIÓN DE CATÁLOGO SOBRE UN RESPALDO REAL CON DOS EMPRESAS (constitución IV).
//
// La 0093 crea 30 triggers sobre las diez tablas que alimentan el menú de la tableta, con el negocio
// operando. Lo que solo se ve sobre datos reales: que crear los triggers no reescribe ni una fila
// del catálogo, que un cambio de una empresa sube SU versión y no la de la otra (también como el
// rol de la aplicación, bajo RLS), y que el Down se lleva los 30 triggers, la función y la tabla
// sin tocar el catálogo.
func TestMigrationCatalogVersionsOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start < catalogVersionsVersion-1 {
		migrarArriba(t, st.Pool)
	}
	migrarAbajoHasta(t, st.Pool, catalogVersionsVersion-1)
	t.Cleanup(func() {
		if versionDeEsquema(t, st) < start {
			migrarArriba(t, st.Pool)
		}
	})

	var a, b int64
	if err := st.Pool.QueryRow(ctx, `select min(id), max(id) from companies`).Scan(&a, &b); err != nil {
		t.Fatal(err)
	}
	catalog := func(t *testing.T) (products, options int, prices string) {
		t.Helper()
		if err := st.Pool.QueryRow(ctx, `
			select (select count(*) from products), (select count(*) from modifier_options),
			       (select coalesce(sum(price), 0)::text from products)`).Scan(&products, &options, &prices); err != nil {
			t.Fatal(err)
		}
		return
	}
	triggers := func(t *testing.T) int {
		t.Helper()
		var n int
		if err := st.Pool.QueryRow(ctx, `
			select count(*) from pg_trigger where not tgisinternal and tgname like '%\_catalog\_version\_%'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	version := func(t *testing.T, company int64) int64 {
		t.Helper()
		var v int64
		if err := st.Pool.QueryRow(ctx, `select coalesce(max(version), 0) from catalog_versions where company_id = $1`, company).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}

	p0, o0, pr0 := catalog(t)
	migrarArriba(t, st.Pool)

	t.Run("crea los 30 triggers sin reescribir el catálogo", func(t *testing.T) {
		if n := triggers(t); n != 30 {
			t.Fatalf("%d triggers de versión; quiere 30 (diez tablas × alta, cambio y baja)", n)
		}
		if p, o, pr := catalog(t); p != p0 || o != o0 || pr != pr0 {
			t.Fatalf("catálogo %d/%d/%s → %d/%d/%s: la migración no debe tocar una fila", p0, o0, pr0, p, o, pr)
		}
	})

	t.Run("un cambio por SQL directo sube la versión de su empresa y no la de la otra", func(t *testing.T) {
		antesA, antesB := version(t, a), version(t, b)
		if _, err := st.Pool.Exec(ctx, `update products set name = name where id = (select min(id) from products where company_id = $1)`, a); err != nil {
			t.Fatal(err)
		}
		if version(t, a) <= antesA {
			t.Fatal("un update directo no subió la versión de su empresa: el menú viejo se seguiría sirviendo")
		}
		if version(t, b) != antesB {
			t.Fatal("un cambio de una empresa subió la versión de la otra")
		}
	})

	t.Run("el rol de la aplicación sube solo la versión de SU empresa", func(t *testing.T) {
		antesA, antesB := version(t, a), version(t, b)
		conn := conexionDeEmpresa(t, restoredAppRoleStore(t), b)
		if _, err := conn.Exec(ctx, `update categories set name = name where id = (select min(id) from categories)`); err != nil {
			t.Fatalf("cambiar una categoría como gatobobah_app: %v", err)
		}
		if version(t, b) <= antesB {
			t.Fatal("un cambio bajo RLS no subió la versión de su empresa")
		}
		if version(t, a) != antesA {
			t.Fatal("un cambio de la empresa B bajo RLS subió la versión de la A")
		}
	})

	t.Run("el Down se lleva triggers, función y tabla sin tocar el catálogo", func(t *testing.T) {
		p1, o1, pr1 := catalog(t)
		migrarAbajoHasta(t, st.Pool, catalogVersionsVersion-1)
		if n := triggers(t); n != 0 {
			t.Fatalf("quedaron %d triggers de versión tras el Down", n)
		}
		var fn, tbl bool
		if err := st.Pool.QueryRow(ctx, `
			select exists (select 1 from pg_proc where proname = 'bump_catalog_version'),
			       to_regclass('public.catalog_versions') is not null`).Scan(&fn, &tbl); err != nil {
			t.Fatal(err)
		}
		if fn || tbl {
			t.Fatalf("tras el Down: función %v, tabla %v; quiere ninguna", fn, tbl)
		}
		if p, o, pr := catalog(t); p != p1 || o != o1 || pr != pr1 {
			t.Fatalf("el Down movió el catálogo: %d/%d/%s → %d/%d/%s", p1, o1, pr1, p, o, pr)
		}
		// La API anterior no sabe de la versión: con el Down, un cambio de catálogo sigue pasando.
		if _, err := st.Pool.Exec(ctx, `update products set name = name where id = (select min(id) from products)`); err != nil {
			t.Fatalf("tras el Down, un cambio de catálogo falla: %v", err)
		}
	})
}
