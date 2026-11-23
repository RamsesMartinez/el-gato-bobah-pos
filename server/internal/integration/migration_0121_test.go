//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

const warehousesContractVersion = 121

// LA 0121 (segunda subida de varios almacenes, spec 094) CONTRA UN RESPALDO REAL CON DOS EMPRESAS.
// Contrae lo que la 0120 dejó por la versión anterior: borra las llaves por sucursal, pone el almacén
// como obligatorio y quita las compuertas. Lo que se prueba es que después de ella sí caben dos
// almacenes por artículo, que nada queda sin almacén, y que el Down se niega cuando bajar perdería en
// cuál está cada existencia.
func TestMigration0121OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	migrarAbajoHasta(t, st.Pool, warehousesContractVersion-1)
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	// Una sucursal que dio de alta la API anterior durante la subida, sin almacén de venta.
	var company int64
	if err := st.Pool.QueryRow(ctx, `select max(id) from companies`).Scan(&company); err != nil {
		t.Fatal(err)
	}
	var orphan int64
	if err := st.Pool.QueryRow(ctx, `insert into branches (company_id, branch_number, code, name, is_active)
		values ($1, 9, 'HUERFANA', 'Sin almacén', false) returning id`, company).Scan(&orphan); err != nil {
		t.Fatal(err)
	}

	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", warehousesContractVersion); err != nil {
		t.Fatal(err)
	}

	t.Run("toda sucursal tiene almacén de venta, también la que nació sin él", func(t *testing.T) {
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from warehouses where branch_id = $1 and is_sales`, orphan).Scan(&n); err != nil || n != 1 {
			t.Fatalf("la sucursal sin almacén quedó con %d de venta, %v", n, err)
		}
	})

	t.Run("el almacén es obligatorio donde hay existencias", func(t *testing.T) {
		for _, tbl := range []string{"stock_levels", "stock_movements", "stock_counts", "waste_records",
			"branch_stock_minimums", "notifications", "stock_out_reports", "count_places"} {
			var nullable string
			if err := st.Pool.QueryRow(ctx, `select is_nullable from information_schema.columns
				where table_name = $1 and column_name = 'warehouse_id'`, tbl).Scan(&nullable); err != nil {
				t.Fatal(err)
			}
			if nullable != "NO" {
				t.Errorf("%s.warehouse_id sigue aceptando nulo", tbl)
			}
		}
		_, err := st.Pool.Exec(ctx, `update expense_items set warehouse_id = null where id = (select min(id) from expense_items where item_type is not null)`)
		exigeViolacionDeRestriccion(t, err, "un renglón de compra con artículo y sin almacén")
	})

	var branch, local, ingredient int64
	if err := st.Pool.QueryRow(ctx, `select s.branch_id, s.warehouse_id, s.ingredient_id from stock_levels s
		where s.company_id = $1 and s.ingredient_id is not null order by s.ingredient_id limit 1`, company).Scan(&branch, &local, &ingredient); err != nil {
		t.Fatal(err)
	}
	conn := conexionDeEmpresa(t, st, company)
	var bodega int64
	if err := conn.QueryRow(ctx, `insert into warehouses (branch_id, name) values ($1, 'Bodega 0121') returning id`, branch).Scan(&bodega); err != nil {
		t.Fatal(err)
	}

	t.Run("dos almacenes de la misma sucursal tienen cada uno su existencia", func(t *testing.T) {
		if _, err := conn.Exec(ctx, `insert into stock_movements (warehouse_id, item_type, ingredient_id, movement_type, quantity, reason)
			values ($1, 'ingrediente', $2, 'ajuste', 20, 'a la bodega 0121')`, bodega, ingredient); err != nil {
			t.Fatalf("un movimiento a la bodega después de la 0121: %v", err)
		}
		var n int
		if err := conn.QueryRow(ctx, `select count(*) from stock_levels where branch_id = $1 and ingredient_id = $2`, branch, ingredient).Scan(&n); err != nil || n != 2 {
			t.Fatalf("el artículo tiene %d filas de existencia en la sucursal, %v; quiere una por almacén", n, err)
		}
	})

	t.Run("una cancelación sin almacén ya no adivina", func(t *testing.T) {
		_, err := conn.Exec(ctx, `insert into stock_movements (item_type, ingredient_id, movement_type, quantity, reason)
			values ('ingrediente', $1, 'cancelacion', 1, 'sin almacén')`, ingredient)
		exigeCodigo(t, err, "EGB04", "una reposición de cancelación sin almacén")
	})

	t.Run("el almacén de venta se puede cambiar, pero siempre hay uno", func(t *testing.T) {
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `update warehouses set is_sales = false where id = $1`, local); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `update warehouses set is_sales = true where id = $1`, bodega); err != nil {
			t.Fatalf("pasar la venta a la bodega: %v", err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		tx, err = conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `update warehouses set is_sales = false where id = $1`, local); err != nil {
			t.Fatal(err)
		}
		exigeViolacionDeRestriccion(t, tx.Commit(ctx), "una sucursal sin almacén de venta")
	})

	t.Run("el Down se niega con dos almacenes por artículo", func(t *testing.T) {
		if err := goose.DownContext(ctx, prepararGoose(t, st.Pool), "."); err == nil {
			t.Fatal("el Down bajó con existencias en dos almacenes: perdería en cuál está cada una")
		}
	})
}
