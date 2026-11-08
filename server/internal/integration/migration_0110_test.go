//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
)

// countProductLinesVersion agrega las cifras de productos en el conteo (spec 069).
const countProductLinesVersion = 110

// LA 0110 SOBRE UN RESPALDO REAL CON DOS EMPRESAS (constitución IV).
//
// Solo agrega una tabla. Lo que se prueba sobre datos reales: que nada existente cambia, que la
// tabla nace aislada y con los permisos del rol de servicio, que la base rechaza una cifra de
// producto partida o negativa, y que el Down no tira cifras de conteos sin avisar.
func TestMigrationCountProductLinesOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start < countProductLinesVersion-1 {
		migrarArriba(t, st.Pool)
	}
	migrarAbajoHasta(t, st.Pool, countProductLinesVersion-1)
	t.Cleanup(func() {
		if versionDeEsquema(t, st) < start {
			migrarArriba(t, st.Pool)
		}
	})
	fingerprint := func(t *testing.T) string {
		t.Helper()
		var f string
		if err := st.Pool.QueryRow(ctx, `select md5(
		    (select coalesce(string_agg(id || status, ',' order by id), '') from stock_counts) ||
		    (select coalesce(string_agg(count_id || '-' || ingredient_id || '-' || counted_qty, ',' order by count_id, ingredient_id), '') from stock_count_lines) ||
		    (select coalesce(string_agg(coalesce(product_id, ingredient_id) || '-' || on_hand, ',' order by branch_id, item_type, coalesce(product_id, ingredient_id)), '') from stock_levels))`).Scan(&f); err != nil {
			t.Fatal(err)
		}
		return f
	}
	before := fingerprint(t)
	migrarArriba(t, st.Pool)

	t.Run("no reescribe conteos ni existencias", func(t *testing.T) {
		if after := fingerprint(t); after != before {
			t.Fatal("la migración cambió conteos, renglones de conteo o existencias")
		}
	})

	t.Run("nace con RLS nullif y los permisos del rol de servicio", func(t *testing.T) {
		var rls bool
		var policy string
		if err := st.Pool.QueryRow(ctx, `select c.relrowsecurity, coalesce(pg_get_expr(p.polqual, p.polrelid), '')
		    from pg_class c left join pg_policy p on p.polrelid = c.oid and p.polname = 'tenant_isolation'
		   where c.relname = 'stock_count_product_lines'`).Scan(&rls, &policy); err != nil {
			t.Fatalf("la tabla no existe: %v", err)
		}
		if !rls || !hasAll(strings.ToLower(policy), "nullif", "app.company_id") {
			t.Fatalf("RLS=%v, política %q: sin aislamiento por empresa con nullif", rls, policy)
		}
		var grants []string
		rows, err := st.Pool.Query(ctx, `select privilege_type from information_schema.role_table_grants
		  where grantee = 'gatobobah_app' and table_name = 'stock_count_product_lines' order by 1`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var g string
			if err := rows.Scan(&g); err != nil {
				t.Fatal(err)
			}
			grants = append(grants, g)
		}
		rows.Close()
		if !hasAll(strings.Join(grants, ","), "DELETE", "INSERT", "SELECT", "UPDATE") {
			t.Fatalf("gatobobah_app tiene %v; le faltan select, insert, update o delete (el primer request respondería 42501)", grants)
		}
	})

	// Un conteo abierto de la empresa real, como owner, con un producto de su catálogo y un usuario suyo.
	var company, branch, product, user int64
	if err := st.Pool.QueryRow(ctx, `select p.company_id, b.id, p.id, u.id
	    from products p
	    join companies c on c.id = p.company_id and c.slug = 'gatobobah'
	    join branches b on b.company_id = p.company_id and b.is_headquarters
	    join users u on u.company_id = p.company_id
	   order by p.id, u.id limit 1`).Scan(&company, &branch, &product, &user); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := st.Pool.QueryRow(ctx, `insert into stock_counts (company_id, branch_id, status, opened_by, closed_by, closed_at)
	    values ($1, $2, 'discarded', $3, $3, now()) returning id`, company, branch, user).Scan(&count); err != nil {
		t.Fatal(err)
	}
	insertLine := func(qty string) error {
		_, err := st.Pool.Exec(ctx, `insert into stock_count_product_lines (count_id, company_id, product_id, counted_qty, system_qty, captured_by)
		    values ($1, $2, $3, `+qty+`, -2, $4)`, count, company, product, user)
		return err
	}

	t.Run("la base rechaza una cifra de producto partida o negativa", func(t *testing.T) {
		exigeViolacionDeRestriccion(t, insertLine("2.5"), "2.5 latas")
		exigeViolacionDeRestriccion(t, insertLine("-1"), "−1 latas")
		if err := insertLine("15"); err != nil {
			t.Fatalf("15 latas debería entrar: %v", err)
		}
	})

	t.Run("un producto de otra empresa no entra al conteo", func(t *testing.T) {
		var foreign int64
		if err := st.Pool.QueryRow(ctx, `select id from products where company_id <> $1 order by id limit 1`, company).Scan(&foreign); err != nil {
			t.Fatal(err)
		}
		_, err := st.Pool.Exec(ctx, `insert into stock_count_product_lines (count_id, company_id, product_id, counted_qty, system_qty, captured_by)
		    values ($1, $2, $3, 1, 0, $4)`, count, company, foreign, user)
		if err == nil {
			t.Fatal("la llave compuesta dejó colgar un producto de otra empresa de este conteo")
		}
	})

	t.Run("el Down se niega con cifras y con la tabla vacía la quita", func(t *testing.T) {
		if err := migrarAbajoHastaErr(t, st.Pool, countProductLinesVersion-1); err == nil {
			t.Fatal("el Down tiró cifras de productos de un conteo sin negarse")
		}
		if _, err := st.Pool.Exec(ctx, `delete from stock_count_product_lines`); err != nil {
			t.Fatal(err)
		}
		migrarAbajoHasta(t, st.Pool, countProductLinesVersion-1)
		var exists bool
		if err := st.Pool.QueryRow(ctx, `select to_regclass('stock_count_product_lines') is not null`).Scan(&exists); err != nil || exists {
			t.Fatalf("la tabla sigue tras el Down (%v)", err)
		}
		migrarArriba(t, st.Pool)
	})
}

// hasAll dice si s contiene cada una de las partes.
func hasAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
