//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

const stockLotsVersion = 128

// LA 0128 (lotes con caducidad, spec 098) CONTRA UN RESPALDO REAL CON DOS EMPRESAS. Al subir, ninguna
// existencia cambia y no nace ningún lote (FR-005, SC-006); con lotes, asignaciones, desglose y un
// aviso «lo que queda está caducado» cargados, el Down baja sin error y el Up vuelve a subir.
func TestMigration0128OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	migrarAbajoHasta(t, st.Pool, stockLotsVersion-1)
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	const levels = `select coalesce(string_agg(warehouse_id || ':' || coalesce(ingredient_id, -product_id) || ':' || on_hand, ',' order by warehouse_id, ingredient_id, product_id), '') from stock_levels`
	var before string
	if err := st.Pool.QueryRow(ctx, levels).Scan(&before); err != nil {
		t.Fatal(err)
	}
	var companies int
	if err := st.Pool.QueryRow(ctx, `select count(*) from companies`).Scan(&companies); err != nil || companies < 2 {
		t.Fatalf("el respaldo trae %d empresas; la prueba pide dos (%v)", companies, err)
	}
	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", stockLotsVersion); err != nil {
		t.Fatal(err)
	}
	var after string
	if err := st.Pool.QueryRow(ctx, levels).Scan(&after); err != nil || after != before {
		t.Fatalf("la migración cambió existencias (%v)", err)
	}
	var lots int
	if err := st.Pool.QueryRow(ctx, `select count(*) from stock_lots`).Scan(&lots); err != nil || lots != 0 {
		t.Fatalf("la migración inventó %d lotes (%v)", lots, err)
	}

	// Cargar de todo, en la última empresa del respaldo.
	var company, warehouse, ingredient int64
	if err := st.Pool.QueryRow(ctx, `select s.company_id, s.warehouse_id, s.ingredient_id from stock_levels s
		join warehouses w on w.id = s.warehouse_id and w.is_sales
		where s.ingredient_id is not null order by s.company_id desc, s.ingredient_id limit 1`).Scan(&company, &warehouse, &ingredient); err != nil {
		t.Fatal(err)
	}
	conn := conexionDeEmpresa(t, st, company)
	for _, sql := range []string{
		`insert into stock_movements (warehouse_id, item_type, ingredient_id, movement_type, quantity, expires_on)
		   values ($1, 'ingrediente', $2, 'compra', 50, business_today(nullif(current_setting('app.company_id'), '')::bigint) - 1)`,
		`insert into stock_movements (warehouse_id, item_type, ingredient_id, movement_type, quantity, expires_on)
		   values ($1, 'ingrediente', $2, 'compra', 20, business_today(nullif(current_setting('app.company_id'), '')::bigint) + 9)`,
		`insert into stock_movements (warehouse_id, item_type, ingredient_id, movement_type, quantity)
		   select $1, 'ingrediente', $2, 'venta', -(greatest(on_hand, 0) + 30) from stock_levels where warehouse_id = $1 and ingredient_id = $2`,
	} {
		if _, err := conn.Exec(ctx, sql, warehouse, ingredient); err != nil {
			t.Fatal(err)
		}
	}
	var notices, allocs int
	if err := st.Pool.QueryRow(ctx, `select (select count(*) from notifications where kind = 'expired_left'),
		(select count(*) from stock_lot_allocations)`).Scan(&notices, &allocs); err != nil || notices == 0 || allocs == 0 {
		t.Fatalf("la carga no dejó aviso (%d) ni asignaciones (%d): %v", notices, allocs, err)
	}

	migrarAbajoHasta(t, st.Pool, stockLotsVersion-1)
	var left int
	if err := st.Pool.QueryRow(ctx, `select count(*) from information_schema.tables where table_name in ('stock_lots', 'stock_lot_allocations', 'stock_count_line_lots')`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("el Down dejó %d tablas (%v)", left, err)
	}
	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", stockLotsVersion); err != nil {
		t.Fatalf("subir otra vez tras el Down: %v", err)
	}
}
