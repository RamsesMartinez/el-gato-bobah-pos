//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

const alertsPerWarehouseVersion = 125

// LA 0125 (avisos, lugares y reglas de traspaso por almacén, spec 094) CONTRA UN RESPALDO REAL CON DOS
// EMPRESAS. Lo que se prueba: que valida las dos reglas que la 0124 dejó pendientes sin tocar filas,
// que un movimiento en un almacén que no es el de venta abre el aviso en ESE almacén cuando tiene
// mínimo propio, que el nombre de un lugar se repite entre almacenes y no dentro de uno, y que el Down
// se niega con nombres repetidos entre almacenes.
func TestMigration0125OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	migrarAbajoHasta(t, st.Pool, alertsPerWarehouseVersion-1)
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	var before int
	if err := st.Pool.QueryRow(ctx, `select count(*) from notifications`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", alertsPerWarehouseVersion); err != nil {
		t.Fatal(err)
	}
	var after int
	if err := st.Pool.QueryRow(ctx, `select count(*) from notifications`).Scan(&after); err != nil || after != before {
		t.Fatalf("la migración cambió los avisos: %d antes, %d después (%v)", before, after, err)
	}

	t.Run("las reglas de traspaso quedan validadas", func(t *testing.T) {
		var pending int
		if err := st.Pool.QueryRow(ctx, `select count(*) from pg_constraint where conrelid = 'stock_movements'::regclass
			and conname in ('stock_movements_transfer_of_branch', 'stock_movements_transfer_link') and not convalidated`).Scan(&pending); err != nil || pending != 0 {
			t.Fatalf("quedan %d reglas sin validar (%v)", pending, err)
		}
	})

	var company, branch, ingredient int64
	if err := st.Pool.QueryRow(ctx, `select s.company_id, s.branch_id, s.ingredient_id from stock_levels s
		join warehouses w on w.id = s.warehouse_id and w.is_sales
		where s.ingredient_id is not null order by s.company_id desc, s.ingredient_id limit 1`).Scan(&company, &branch, &ingredient); err != nil {
		t.Fatal(err)
	}
	conn := conexionDeEmpresa(t, st, company)
	var bodega int64
	if err := conn.QueryRow(ctx, `insert into warehouses (branch_id, name) values ($1, 'Bodega 0125') returning id`, branch).Scan(&bodega); err != nil {
		t.Fatal(err)
	}

	t.Run("el aviso mide el almacén del movimiento", func(t *testing.T) {
		if _, err := conn.Exec(ctx, `update ingredients set is_active = true, low_stock_enabled = true, low_stock_on_min = true where id = $1`, ingredient); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, `insert into branch_stock_minimums (branch_id, warehouse_id, item_type, ingredient_id, min_stock, updated_by)
			select $1, $2, 'ingrediente', $3, 50, min(id) from users where company_id = $4`, branch, bodega, ingredient, company); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, `insert into stock_movements (warehouse_id, item_type, ingredient_id, movement_type, quantity)
			values ($1, 'ingrediente', $2, 'ajuste', 10)`, bodega, ingredient); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := conn.QueryRow(ctx, `select count(*) from notifications where warehouse_id = $1 and ingredient_id = $2
			and kind = 'stock_low' and status = 'open'`, bodega, ingredient).Scan(&n); err != nil || n != 1 {
			t.Fatalf("Bodega con 10 y mínimo propio de 50 tiene %d avisos de stock bajo (%v); quiere 1", n, err)
		}
	})

	t.Run("un lugar se repite entre almacenes, no dentro de uno", func(t *testing.T) {
		if _, err := conn.Exec(ctx, `insert into count_places (name, warehouse_id, position) values ('Refri 0125', $1, 0)`, bodega); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(ctx, `insert into count_places (name, position) values ('refri 0125', 0)`); err != nil {
			t.Fatalf("el mismo nombre en el almacén de venta: %v", err)
		}
		_, err := conn.Exec(ctx, `insert into count_places (name, warehouse_id, position) values ('REFRI 0125', $1, 1)`, bodega)
		exigeViolacionDeRestriccion(t, err, "dos lugares con el mismo nombre en Bodega")
	})

	t.Run("el Down se niega con nombres repetidos entre almacenes", func(t *testing.T) {
		if err := goose.DownContext(ctx, prepararGoose(t, st.Pool), "."); err == nil {
			t.Fatal("el Down bajó con «Refri 0125» en dos almacenes")
		}
	})
}
