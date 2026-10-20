//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// migrarAbajoHastaErr es migrarAbajoHasta que devuelve el error en vez de tronar el test: el Down
// que se niega es lo que se está probando.
func migrarAbajoHastaErr(t *testing.T, pool *pgxpool.Pool, version int64) error {
	t.Helper()
	return goose.DownToContext(context.Background(), prepararGoose(t, pool), ".", version)
}

// ingredientManagementVersion es la migración de la gestión de artículos (spec 042).
const ingredientManagementVersion = 96

// LA 0096 SOBRE UN RESPALDO REAL CON DOS EMPRESAS (constitución IV).
//
// Solo agrega: dos tablas, una columna en los movimientos y dos índices. Lo que se mide es que las
// tablas nuevas nazcan con las mismas barreras que las demás de empresa, que una liga no cruce
// empresas aunque la llave foránea salte RLS, que borrar una empresa no se atore en ellas, y que el
// Down no tire correcciones de costo ni reemplazos ya registrados.
func TestMigrationIngredientManagementOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start < ingredientManagementVersion-1 {
		migrarArriba(t, st.Pool)
	}
	migrarAbajoHasta(t, st.Pool, ingredientManagementVersion-1)
	t.Cleanup(func() {
		if versionDeEsquema(t, st) < start {
			migrarArriba(t, st.Pool)
		}
	})
	migrarArriba(t, st.Pool)

	var coA, coB, ingA, ingA2, ingB, userA int64
	if err := st.Pool.QueryRow(ctx, `
		with c as (select company_id from ingredients group by company_id having count(*) >= 2 order by company_id limit 2)
		select (select min(company_id) from c), (select max(company_id) from c),
		       (select min(id) from ingredients where company_id = (select min(company_id) from c)),
		       (select max(id) from ingredients where company_id = (select min(company_id) from c)),
		       (select min(id) from ingredients where company_id = (select max(company_id) from c)),
		       (select min(id) from users where company_id = (select min(company_id) from c))`).
		Scan(&coA, &coB, &ingA, &ingA2, &ingB, &userA); err != nil {
		t.Fatal(err)
	}
	if coA == coB {
		t.Fatal("el respaldo necesita dos empresas con artículos")
	}

	t.Run("las dos tablas nacen con RLS por nullif y solo select e insert para la app", func(t *testing.T) {
		for _, tbl := range []string{"ingredient_cost_corrections", "ingredient_replacements"} {
			var rls, sel, ins, upd, del bool
			var qual string
			if err := st.Pool.QueryRow(ctx, `
				select c.relrowsecurity,
				       has_table_privilege('gatobobah_app', c.oid, 'SELECT'), has_table_privilege('gatobobah_app', c.oid, 'INSERT'),
				       has_table_privilege('gatobobah_app', c.oid, 'UPDATE'), has_table_privilege('gatobobah_app', c.oid, 'DELETE'),
				       pg_get_expr(p.polqual, p.polrelid)
				  from pg_class c join pg_policy p on p.polrelid = c.oid where c.relname = $1`, tbl).
				Scan(&rls, &sel, &ins, &upd, &del, &qual); err != nil {
				t.Fatalf("%s: %v", tbl, err)
			}
			if !rls || !sel || !ins || upd || del || !strings.Contains(strings.ToLower(qual), "nullif") {
				t.Fatalf("%s: rls %v select %v insert %v update %v delete %v política %q", tbl, rls, sel, ins, upd, del, qual)
			}
		}
	})

	t.Run("una corrección no puede apuntar al artículo de otra empresa", func(t *testing.T) {
		_, err := st.Pool.Exec(ctx, `insert into ingredient_cost_corrections (company_id, ingredient_id, previous_cost, new_cost, user_id)
			values ($1, $2, 0, 1, $3)`, coA, ingB, userA)
		exigeViolacionDeRestriccion(t, err, "ligar la corrección de una empresa al artículo de otra")
	})

	t.Run("un reemplazo no apunta a sí mismo", func(t *testing.T) {
		_, err := st.Pool.Exec(ctx, `insert into ingredient_replacements (company_id, old_ingredient_id, new_ingredient_id, factor, user_id)
			values ($1, $2, $2, 1, $3)`, coA, ingA, userA)
		exigeViolacionDeRestriccion(t, err, "reemplazar un artículo por sí mismo")
	})

	t.Run("los índices de la bitácora por artículo y del historial de compras existen", func(t *testing.T) {
		for _, idx := range []string{"audit_events_by_entity", "stock_movements_purchases_by_ingredient", "stock_movements_by_replacement"} {
			var def string
			if err := st.Pool.QueryRow(ctx, `select indexdef from pg_indexes where indexname = $1`, idx).Scan(&def); err != nil {
				t.Fatalf("%s: %v", idx, err)
			}
			if idx != "stock_movements_by_replacement" && !strings.Contains(def, "(company_id,") {
				t.Fatalf("%s no empieza por company_id: %s", idx, def)
			}
		}
	})

	t.Run("el Down se niega con una corrección registrada", func(t *testing.T) {
		tx, err := st.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, `insert into ingredient_cost_corrections (company_id, ingredient_id, previous_cost, new_cost, user_id)
			values ($1, $2, 0, 1, $3)`, coA, ingA, userA); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := migrarAbajoHastaErr(t, st.Pool, ingredientManagementVersion-1); err == nil {
			t.Fatal("el Down tiró una corrección de costo; debía negarse")
		}
		if _, err := st.Pool.Exec(ctx, `delete from ingredient_cost_corrections`); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("un movimiento no se liga al reemplazo de otra empresa, y un artículo se reemplaza una vez", func(t *testing.T) {
		tx, err := st.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		var rep int64
		if err := tx.QueryRow(ctx, `insert into ingredient_replacements (company_id, old_ingredient_id, new_ingredient_id, factor, user_id)
			values ($1, $2, $3, 2, $4) returning id`, coA, ingA, ingA2, userA).Scan(&rep); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `savepoint s1`); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity, replacement_id)
			values ($1, 'ingrediente', $2, 'ajuste', 1, $3)`, coB, ingB, rep)
		exigeViolacionDeRestriccion(t, err, "ligar un movimiento de una empresa al reemplazo de otra")
		if _, err := tx.Exec(ctx, `rollback to savepoint s1`); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, `insert into ingredient_replacements (company_id, old_ingredient_id, new_ingredient_id, factor, user_id)
			values ($1, $2, $3, 3, $4)`, coA, ingA, ingA2, userA)
		exigeViolacionDeRestriccion(t, err, "reemplazar dos veces el mismo artículo")
	})

	t.Run("el Down se niega también con solo un reemplazo registrado", func(t *testing.T) {
		if _, err := st.Pool.Exec(ctx, `insert into ingredient_replacements (company_id, old_ingredient_id, new_ingredient_id, factor, user_id)
			values ($1, $2, $3, 2, $4)`, coA, ingA, ingA2, userA); err != nil {
			t.Fatal(err)
		}
		if err := migrarAbajoHastaErr(t, st.Pool, ingredientManagementVersion-1); err == nil {
			t.Fatal("el Down tiró un reemplazo; debía negarse")
		}
		if _, err := st.Pool.Exec(ctx, `delete from ingredient_replacements`); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ida y vuelta en vacío", func(t *testing.T) {
		migrarAbajoHasta(t, st.Pool, ingredientManagementVersion-1)
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from pg_class where relname in ('ingredient_cost_corrections', 'ingredient_replacements')`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		var col int
		if err := st.Pool.QueryRow(ctx, `select count(*) from information_schema.columns where table_name = 'stock_movements' and column_name = 'replacement_id'`).Scan(&col); err != nil {
			t.Fatal(err)
		}
		if n != 0 || col != 0 {
			t.Fatalf("tras el Down quedan %d tablas y %d columnas", n, col)
		}
		migrarArriba(t, st.Pool)
	})
}

// Borrar una empresa que tiene correcciones, reemplazos y un movimiento ligado a un reemplazo no se
// atora en las tablas nuevas: todo cae en la misma sentencia, y una llave con restrict truena según el
// orden en que llegue la cascada. Sobre una base nueva (la restaurada tiene bitácora, que no se borra).
func TestDeletingACompanyWithReplacementsCascades(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	co := makeCompany(t, st, "borrar-con-reemplazos")
	user := makeUserIn(t, st, co, "borrar_reemplazos", "admin")
	viejo := ingredientIn(t, st, co, "Viejo en piezas", "pieza")
	nuevo := ingredientIn(t, st, co, "Nuevo en gramos", "g")
	var rep int64
	if err := st.Pool.QueryRow(ctx, `insert into ingredient_replacements (company_id, old_ingredient_id, new_ingredient_id, factor, user_id)
		values ($1, $2, $3, 250, $4) returning id`, co, viejo, nuevo, user).Scan(&rep); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into ingredient_cost_corrections (company_id, ingredient_id, previous_cost, new_cost, user_id)
		values ($1, $2, 0, 1, $3)`, co, nuevo, user); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity, replacement_id)
		values ($1, 'ingrediente', $2, 'ajuste', 4, $3)`, co, nuevo, rep); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `delete from companies where id = $1`, co); err != nil {
		t.Fatalf("borrar la empresa se atoró: %v", err)
	}
}
