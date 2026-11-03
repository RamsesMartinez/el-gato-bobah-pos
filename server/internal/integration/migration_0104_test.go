//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// mergeVersion es la migración de unir insumos y la marca en la presentación (spec 056).
const mergeVersion = 104

// LA 0104 SOBRE UN RESPALDO REAL CON DOS EMPRESAS (constitución IV).
//
// Agrega columnas a tablas vivas (reemplazos, presentaciones, renglones de compra), cambia el índice
// único de las presentaciones y pone guardas en todo lo que apunta a un insumo. Lo que solo se ve
// sobre datos reales: que los reemplazos que ya existen quedan como cambio de unidad, que el índice
// nuevo se construye sobre las presentaciones que hay, que las guardas no tocan los movimientos
// históricos y que una unión de una empresa no mueve nada de la otra.
func TestMigrationIngredientMergeOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start < mergeVersion-1 {
		migrarArriba(t, st.Pool)
	}
	migrarAbajoHasta(t, st.Pool, mergeVersion-1)
	t.Cleanup(func() {
		if versionDeEsquema(t, st) < start {
			migrarArriba(t, st.Pool)
		}
	})
	var replacements, movements, presentations int64
	if err := st.Pool.QueryRow(ctx, `select (select count(*) from ingredient_replacements), (select count(*) from stock_movements),
		(select count(*) from ingredient_presentations)`).Scan(&replacements, &movements, &presentations); err != nil {
		t.Fatal(err)
	}
	migrarArriba(t, st.Pool)

	t.Run("los reemplazos que ya existían quedan como cambio de unidad y nada se reescribe", func(t *testing.T) {
		var unitChanges, total, mov, pres int64
		if err := st.Pool.QueryRow(ctx, `select count(*) filter (where kind = 'unit_change'), count(*),
			(select count(*) from stock_movements), (select count(*) from ingredient_presentations)
			from ingredient_replacements`).Scan(&unitChanges, &total, &mov, &pres); err != nil {
			t.Fatal(err)
		}
		if total != replacements || unitChanges != replacements {
			t.Fatalf("reemplazos %d → %d, %d como cambio de unidad", replacements, total, unitChanges)
		}
		if mov != movements || pres != presentations {
			t.Fatalf("movimientos %d → %d, presentaciones %d → %d: la migración no reescribe nada", movements, mov, presentations, pres)
		}
	})

	t.Run("la tabla de pares descartados tiene RLS y solo select e insert", func(t *testing.T) {
		var rls, sel, ins, upd, del bool
		if err := st.Pool.QueryRow(ctx, `
			select c.relrowsecurity,
			       has_table_privilege('gatobobah_app', c.oid, 'SELECT'), has_table_privilege('gatobobah_app', c.oid, 'INSERT'),
			       has_table_privilege('gatobobah_app', c.oid, 'UPDATE'), has_table_privilege('gatobobah_app', c.oid, 'DELETE')
			  from pg_class c where c.relname = 'ingredient_distinct_pairs'`).Scan(&rls, &sel, &ins, &upd, &del); err != nil {
			t.Fatal(err)
		}
		if !rls || !sel || !ins || upd || del {
			t.Fatalf("rls=%v select=%v insert=%v update=%v delete=%v: un par descartado es un hecho sin editar", rls, sel, ins, upd, del)
		}
	})

	t.Run("las funciones de la base no se pueden llamar desde la app", func(t *testing.T) {
		rows, err := st.Pool.Query(ctx, `select p.proname from pg_proc p
			where p.proname in ('redirect_merged_ingredient_movement', 'reject_movement_on_merged_ingredient', 'reject_merged_ingredient',
			                    'reject_reactivating_merged_ingredient', 'check_ingredient_merge', 'snapshot_presentation_brand')
			  and has_function_privilege('gatobobah_app', p.oid, 'EXECUTE')`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			_ = rows.Scan(&name)
			t.Errorf("gatobobah_app puede ejecutar %s: corre como dueño y salta RLS", name)
		}
	})

	gatobobah := companyBySlug(t, st, "gatobobah")
	other := companyBySlug(t, st, "bobah-pruebas")
	var user int64
	if err := st.Pool.QueryRow(ctx, `select id from users where company_id = $1 order by id limit 1`, gatobobah).Scan(&user); err != nil {
		t.Fatal(err)
	}
	src := ingredientIn(t, st, gatobobah, "Leche mig 56 marca B", "ml")
	dst := ingredientIn(t, st, gatobobah, "Leche mig 56", "ml")
	otherIng := ingredientIn(t, st, other, "Leche mig 56 marca B", "ml")

	t.Run("dos presentaciones con el mismo nombre conviven si su marca es distinta", func(t *testing.T) {
		if _, err := st.Pool.Exec(ctx, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, brand)
			values ($1, $2, 'Caja', 12000, 'Marca A'), ($1, $2, 'Caja', 12000, 'Marca B')`, gatobobah, dst); err != nil {
			t.Fatalf("dos «Caja» con marca distinta: %v", err)
		}
		_, err := st.Pool.Exec(ctx, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, brand)
			values ($1, $2, ' caja ', 8000, 'marca a')`, gatobobah, dst)
		exigeViolacionDeRestriccion(t, err, "otra «Caja · Marca A» en el mismo insumo")
		if _, err := st.Pool.Exec(ctx, `delete from ingredient_presentations where ingredient_id = $1`, dst); err != nil {
			t.Fatal(err)
		}
	})

	var mergeID int64
	if err := st.Pool.QueryRow(ctx, `insert into ingredient_replacements (company_id, old_ingredient_id, new_ingredient_id, factor, user_id, kind)
		values ($1, $2, $3, 1, $4, 'merge') returning id`, gatobobah, src, dst, user).Scan(&mergeID); err != nil {
		t.Fatalf("registrar la unión: %v", err)
	}

	t.Run("un movimiento que llega al origen unido entra al destino; el de la otra empresa no se toca", func(t *testing.T) {
		var got int64
		var qty decimal.Decimal
		if err := st.Pool.QueryRow(ctx, `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity)
			values ($1, 'ingrediente', $2, 'ajuste', -150) returning ingredient_id, quantity`, gatobobah, src).Scan(&got, &qty); err != nil {
			t.Fatal(err)
		}
		if got != dst || !qty.Equal(decimal.NewFromInt(-150)) {
			t.Fatalf("el movimiento quedó en %d con %s; debía entrar a %d con -150", got, qty, dst)
		}
		if err := st.Pool.QueryRow(ctx, `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity)
			values ($1, 'ingrediente', $2, 'ajuste', 5) returning ingredient_id`, other, otherIng).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != otherIng {
			t.Fatalf("un movimiento de otra empresa se redirigió a %d", got)
		}
	})

	t.Run("lo que no es un movimiento y apunta al origen unido se rechaza nombrando el destino", func(t *testing.T) {
		recipe := makeRecipe(t, st, map[int64]string{})
		_, err := st.Pool.Exec(ctx, `insert into recipe_items (company_id, recipe_id, ingredient_id, quantity, unit_id)
			select $1, $2, $3, 10, id from units where code = 'ml'`, gatobobah, recipe, src)
		exigeUnido(t, err, "Leche mig 56")
		_, err = st.Pool.Exec(ctx, `update ingredients set is_active = false where id = $1`, src)
		if err != nil {
			t.Fatal(err)
		}
		_, err = st.Pool.Exec(ctx, `update ingredients set is_active = true where id = $1`, src)
		exigeUnido(t, err, "Leche mig 56")
	})

	t.Run("una unión no puede tener como destino un insumo que ya se unió a otro", func(t *testing.T) {
		third := ingredientIn(t, st, gatobobah, "Leche mig 56 marca C", "ml")
		_, err := st.Pool.Exec(ctx, `insert into ingredient_replacements (company_id, old_ingredient_id, new_ingredient_id, factor, user_id, kind)
			values ($1, $2, $3, 1, $4, 'merge')`, gatobobah, third, src, user)
		exigeViolacionDeRestriccion(t, err, "unir en un insumo que ya es origen de otra unión")
	})

	t.Run("el Down se niega con una unión y baja con su limpieza", func(t *testing.T) {
		if err := migrarAbajoHastaErr(t, st.Pool, mergeVersion-1); err == nil {
			t.Fatal("el Down corrió con una unión registrada: perdería de quién es sucesor cada insumo")
		}
		clearWhatTheDownGuards(t, st.Pool, mergeVersion)
		if err := migrarAbajoHastaErr(t, st.Pool, mergeVersion-1); err != nil {
			t.Fatalf("sin uniones ni marcas el Down debió bajar: %v", err)
		}
		var cols int
		if err := st.Pool.QueryRow(ctx, `select count(*) from information_schema.columns
			where (table_name, column_name) in (('ingredient_replacements', 'kind'), ('ingredient_presentations', 'brand'),
			                                   ('expense_items', 'presentation_brand'))`).Scan(&cols); err != nil {
			t.Fatal(err)
		}
		if cols != 0 {
			t.Fatalf("el Down dejó %d columnas", cols)
		}
		migrarArriba(t, st.Pool)
	})
}

// companyBySlug es la empresa por su slug: en la base restaurada, la 1 NO es El Gato Bobah (AGENTS.md).
func companyBySlug(t *testing.T, st *store.Store, slug string) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(), `select id from companies where slug = $1`, slug).Scan(&id); err != nil {
		t.Fatalf("empresa %s: %v", slug, err)
	}
	return id
}

// exigeUnido falla si err no es el rechazo de la base por un insumo unido (EGB02) que nombre el destino.
func exigeUnido(t *testing.T, err error, target string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "EGB02" {
		t.Fatalf("se esperaba el rechazo EGB02 por insumo unido; llegó %v", err)
	}
	if !strings.Contains(pgErr.Message, target) {
		t.Fatalf("el rechazo no nombra el destino %q: %s", target, pgErr.Message)
	}
}
