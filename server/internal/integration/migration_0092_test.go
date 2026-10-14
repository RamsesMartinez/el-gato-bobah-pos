//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Las migraciones 0091 (tipo de unidad «longitud») y 0092 (compras al almacén, spec 038), probadas
// ANTES de escribirlas.
//
// Como la 0089, no rellenan datos existentes: crean tablas vacías, columnas con default constante
// y un check `not valid` sobre `expense_items` (la copia de producción del 14-oct trae un renglón
// con importe 0, que el check nuevo no reescribe). Por eso no exigen un respaldo restaurado; lo que
// sí necesita dos empresas es el aislamiento y las llaves compuestas, que se arman aquí.

var tables0092 = []string{"ingredient_presentations", "stock_counts", "stock_count_lines"}

// ingredientIn crea, como owner, un insumo de `company` medido en la unidad `unit` (g, ml, pieza…).
func ingredientIn(t *testing.T, st *store.Store, company int64, name, unit string) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(), `
		insert into ingredients (company_id, name, base_unit_id)
		select $1, $2, id from units where code = $3 returning id`, company, name, unit).Scan(&id); err != nil {
		t.Fatalf("insumo %s: %v", name, err)
	}
	return id
}

// purchaseIn crea, como owner, un gasto de `company` con un renglón del insumo y devuelve el
// gasto y el renglón.
func purchaseIn(t *testing.T, st *store.Store, company, user, ingredient int64) (expense, item int64) {
	t.Helper()
	ctx := context.Background()
	var cat int64
	if err := st.Pool.QueryRow(ctx, `insert into expense_categories (company_id, name, financial_group) values ($1, 'Compras ' || gen_random_uuid()::text, 'operacional') returning id`,
		company).Scan(&cat); err != nil {
		t.Fatalf("categoría: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, `
		insert into expenses (company_id, category_id, amount, expense_date, created_by)
		values ($1, $2, 100, current_date, $3) returning id`, company, cat, user).Scan(&expense); err != nil {
		t.Fatalf("gasto: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, `
		insert into expense_items (company_id, expense_id, item_type, ingredient_id, description, quantity, unit_id, amount)
		select $1, $2, 'ingrediente', $3, 'renglón', 1, base_unit_id, 50 from ingredients where id = $3 returning id`,
		company, expense, ingredient).Scan(&item); err != nil {
		t.Fatalf("renglón: %v", err)
	}
	return expense, item
}

func TestMigration0091AddsLengthUnits(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	rows, err := st.Pool.Query(context.Background(), `select code, to_base::text from units where kind = 'longitud' order by to_base`)
	if err != nil {
		t.Fatalf("el tipo de unidad «longitud» no existe: %v", err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var code, toBase string
		if err := rows.Scan(&code, &toBase); err != nil {
			t.Fatal(err)
		}
		got[code] = toBase
	}
	if got["cm"] != "1.000000" || got["m"] != "100.000000" || len(got) != 2 {
		t.Fatalf("unidades de longitud = %v; quiere cm (base, 1) y m (100)", got)
	}
}

func TestMigration0092CreatesTablesWithRLSAndGrants(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	for _, table := range tables0092 {
		var rls, sel, ins, upd, del bool
		if err := st.Pool.QueryRow(context.Background(), `
			select c.relrowsecurity,
			       has_table_privilege('gatobobah_app', c.oid, 'select'),
			       has_table_privilege('gatobobah_app', c.oid, 'insert'),
			       has_table_privilege('gatobobah_app', c.oid, 'update'),
			       has_table_privilege('gatobobah_app', c.oid, 'delete')
			  from pg_class c where c.relname = $1 and c.relnamespace = 'public'::regnamespace`, table).
			Scan(&rls, &sel, &ins, &upd, &del); err != nil {
			t.Fatalf("%s no existe: %v", table, err)
		}
		if !rls || !sel || !ins || !upd {
			t.Errorf("%s: rls=%v select=%v insert=%v update=%v", table, rls, sel, ins, upd)
		}
		// Un empaque se desactiva y un conteo se descarta: nada de esto se borra desde la app.
		// La excepción son los renglones del conteo abierto («dejar sin contar»), que sí se quitan.
		if del && table != "stock_count_lines" {
			t.Errorf("%s: gatobobah_app puede borrar", table)
		}
	}
}

// Los chequeos de integridad referencial saltan RLS: sin llaves compuestas, una escritura como
// owner (un arreglo de datos, la importación) cuelga una fila de una empresa en otra sin protesta.
func TestMigration0092RejectsCrossCompanyReferences(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	a := makeCompany(t, st, "m92-a")
	b := makeCompany(t, st, "m92-b")
	userA := makeUserIn(t, st, a, "m92-ua", "admin")
	userB := makeUserIn(t, st, b, "m92-ub", "admin")
	ingA := ingredientIn(t, st, a, "Leche m92a", "ml")
	ingB := ingredientIn(t, st, b, "Leche m92b", "ml")
	_, itemA := purchaseIn(t, st, a, userA, ingA)

	var presB int64
	if err := st.Pool.QueryRow(ctx, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base)
		values ($1, $2, 'Caja', 12000) returning id`, b, ingB).Scan(&presB); err != nil {
		t.Fatalf("empaque de B: %v", err)
	}
	var countA int64
	if err := st.Pool.QueryRow(ctx, `insert into stock_counts (company_id, branch_id, opened_by)
		select $1, id, $2 from branches where company_id = $1 limit 1 returning id`, a, userA).Scan(&countA); err != nil {
		t.Fatalf("conteo de A: %v", err)
	}

	cases := []struct {
		name string
		sql  string
		args []any
	}{
		{"empaque de A sobre insumo de B", `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base) values ($1, $2, 'Caja', 1000)`, []any{a, ingB}},
		{"renglón de A con empaque de B", `update expense_items set presentation_id = $2 where id = $1`, []any{itemA, presB}},
		{"renglón de conteo de B en conteo de A", `insert into stock_count_lines (company_id, count_id, ingredient_id, counted_qty, system_qty, captured_by) values ($1, $2, $3, 1, 0, $4)`, []any{b, countA, ingB, userB}},
		{"movimiento de B ligado a renglón de A", `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity, expense_item_id) values ($1, 'ingrediente', $2, 'compra', 1, $3)`, []any{b, ingB, itemA}},
		{"movimiento de B ligado a conteo de A", `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity, stock_count_id) values ($1, 'ingrediente', $2, 'ajuste', 1, $3)`, []any{b, ingB, countA}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := st.Pool.Exec(ctx, c.sql, c.args...); !esViolacionDeLlave(err) {
				t.Fatalf("se aceptó una referencia entre empresas: %v", err)
			}
		})
	}
}

// Las reglas de un empaque comparan con la unidad del insumo y con el catálogo de unidades (otras
// tablas), así que viven en un trigger y no en un check (revisión de base de datos del 038).
func TestMigration0092PresentationRules(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	co := makeCompany(t, st, "m92-emp")
	vaso := ingredientIn(t, st, co, "Vaso m92", "pieza")
	leche := ingredientIn(t, st, co, "Leche m92", "ml")
	aguacate := ingredientIn(t, st, co, "Aguacate m92", "pieza")

	ok := []struct {
		name string
		sql  string
		args []any
	}{
		{"paquete entero de piezas", `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base) values ($1, $2, 'Paquete', 50)`, []any{co, vaso}},
		{"caja con decimales en ml", `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base) values ($1, $2, 'Galón', 3785.4)`, []any{co, leche}},
		{"aproximado por kilo de un insumo por pieza", `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, purchase_unit_id, is_approximate)
			select $1, $2, 'Kilo', 4, id, true from units where code = 'kg'`, []any{co, aguacate}},
	}
	for _, c := range ok {
		t.Run(c.name, func(t *testing.T) {
			if _, err := st.Pool.Exec(ctx, c.sql, c.args...); err != nil {
				t.Fatalf("debió aceptarse: %v", err)
			}
		})
	}
	bad := []struct {
		name string
		sql  string
		args []any
	}{
		{"piezas con decimales (regla 16)", `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base) values ($1, $2, 'Bolsa', 62.5)`, []any{co, vaso}},
		{"contenido en cero", `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base) values ($1, $2, 'Caja', 0)`, []any{co, leche}},
		{"nombre de una unidad sin ser aproximado (regla 14)", `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base) values ($1, $2, 'litro', 1000)`, []any{co, leche}},
		{"aproximado del mismo tipo", `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, purchase_unit_id, is_approximate)
			select $1, $2, 'Litro', 1000, id, true from units where code = 'l'`, []any{co, leche}},
		{"de otro tipo sin marcarlo aproximado", `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, purchase_unit_id, is_approximate)
			select $1, $2, 'Kilo', 4, id, false from units where code = 'kg'`, []any{co, vaso}},
		{"nombre repetido sin acentos ni mayúsculas", `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base) values ($1, $2, '  PAQUETE ', 25)`, []any{co, vaso}},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			_, err := st.Pool.Exec(ctx, c.sql, c.args...)
			exigeViolacionDeRestriccion(t, err, c.name)
		})
	}
}

func TestMigration0092ColumnChecks(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	co := makeCompany(t, st, "m92-chk")
	user := makeUserIn(t, st, co, "m92-chk-u", "admin")
	ing := ingredientIn(t, st, co, "Azúcar m92", "g")
	_, item := purchaseIn(t, st, co, user, ing)
	// La configuración nace al primer guardado; aquí se crea para probar sus checks.
	if _, err := st.Pool.Exec(ctx, `insert into business_settings (company_id, business_name) values ($1, 'Negocio') on conflict do nothing`, co); err != nil {
		t.Fatalf("configuración: %v", err)
	}

	bad := []struct {
		name string
		sql  string
		args []any
	}{
		{"renglón nuevo con importe 0 sin ser regalo", `insert into expense_items (company_id, expense_id, item_type, ingredient_id, description, quantity, unit_id, amount)
			select company_id, expense_id, item_type, ingredient_id, 'x', 1, unit_id, 0 from expense_items where id = $1`, []any{item}},
		{"regalo con importe", `update expense_items set is_gift = true where id = $1`, []any{item}},
		{"ya no va a llegar más de lo pedido", `update expense_items set not_coming_qty = 2 where id = $1`, []any{item}},
		{"ya no va a llegar negativo", `update expense_items set not_coming_qty = -1 where id = $1`, []any{item}},
		{"lugar vacío", `update ingredients set count_place = '' where id = $1`, []any{ing}},
		{"método de costeo desconocido", `update business_settings set costing_method = 'fifo' where company_id = $1`, []any{co}},
		{"palabra del almacén de más de 20", `update business_settings set stock_item_label = repeat('x', 21) where company_id = $1`, []any{co}},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			_, err := st.Pool.Exec(ctx, c.sql, c.args...)
			exigeViolacionDeRestriccion(t, err, c.name)
		})
	}

	t.Run("regalo con importe 0", func(t *testing.T) {
		if _, err := st.Pool.Exec(ctx, `insert into expense_items (company_id, expense_id, item_type, ingredient_id, description, quantity, unit_id, amount, is_gift)
			select company_id, expense_id, item_type, ingredient_id, 'regalo', 1, unit_id, 0, true from expense_items where id = $1`, item); err != nil {
			t.Fatalf("un regalo con importe 0 debe aceptarse: %v", err)
		}
	})
	t.Run("los defaults de la configuración", func(t *testing.T) {
		var method, label string
		if err := st.Pool.QueryRow(ctx, `select costing_method, stock_item_label from business_settings where company_id = $1`, co).Scan(&method, &label); err != nil {
			t.Fatal(err)
		}
		if method != "last" || label != "artículo" {
			t.Errorf("defaults = %q, %q; quiere last y artículo", method, label)
		}
	})
}

func TestMigration0092OneOpenCountPerBranch(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	co := makeCompany(t, st, "m92-cnt")
	user := makeUserIn(t, st, co, "m92-cnt-u", "admin")
	open := `insert into stock_counts (company_id, branch_id, opened_by) select $1, id, $2 from branches where company_id = $1 limit 1`
	if _, err := st.Pool.Exec(ctx, open, co, user); err != nil {
		t.Fatalf("primer conteo: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, open, co, user); err == nil {
		t.Fatal("dos conteos abiertos en la misma sucursal: las cifras de uno pisarían las del otro al cerrar")
	}
	if _, err := st.Pool.Exec(ctx, `update stock_counts set status = 'closed' where company_id = $1`, co); err == nil {
		t.Fatal("un conteo cerrado sin quién ni cuándo")
	}
}

// La 0092 se niega a bajar con datos (es la única copia de esos hechos); sin datos baja limpia, y la
// 0091 deja el valor del enum (Postgres no sabe quitarlo) sin tronar.
func TestMigration0092DownRefusesWithDataAndRevertsClean(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	co := makeCompany(t, st, "m92-down")
	user := makeUserIn(t, st, co, "m92-down-u", "admin")
	if _, err := st.Pool.Exec(ctx, `insert into stock_counts (company_id, branch_id, opened_by)
		select $1, id, $2 from branches where company_id = $1 limit 1`, co, user); err != nil {
		t.Fatal(err)
	}
	db := prepararGoose(t, st.Pool)
	if err := downTo(db, 91); err == nil {
		t.Fatal("el Down borró un conteo")
	}
	if _, err := st.Pool.Exec(ctx, `delete from stock_counts`); err != nil {
		t.Fatal(err)
	}
	migrarAbajoHasta(t, st.Pool, 90)
	for _, table := range tables0092 {
		var exists bool
		if err := st.Pool.QueryRow(ctx, `select to_regclass($1) is not null`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Errorf("el Down dejó %s en pie", table)
		}
	}
	var units int
	if err := st.Pool.QueryRow(ctx, `select count(*) from units where code in ('cm', 'm')`).Scan(&units); err != nil {
		t.Fatal(err)
	}
	if units != 0 {
		t.Error("el Down dejó las unidades de longitud")
	}
	migrarArriba(t, st.Pool)
}
