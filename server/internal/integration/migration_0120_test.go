//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"
)

const warehousesVersion = 120

// LA 0120 (varios almacenes, spec 094) CONTRA UN RESPALDO REAL CON DOS EMPRESAS. Lo que se prueba es lo
// que no se puede arreglar después de subirla: que ningún registro quede sin almacén, que ninguna
// existencia cambie de total, y que la API anterior —que no conoce la columna— siga escribiendo
// durante la subida.
func TestMigration0120OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	migrarAbajoHasta(t, st.Pool, warehousesVersion-1)
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	totals := func() map[[2]int64]string {
		rows, err := st.Pool.Query(ctx, `
			select branch_id, coalesce(ingredient_id, -product_id), sum(on_hand)::text
			  from stock_levels group by 1, 2`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := map[[2]int64]string{}
		for rows.Next() {
			var b, a int64
			var s string
			if err := rows.Scan(&b, &a, &s); err != nil {
				t.Fatal(err)
			}
			out[[2]int64{b, a}] = s
		}
		return out
	}
	before := totals()
	if len(before) == 0 {
		t.Fatal("el respaldo no trae existencias: la prueba no probaría nada")
	}

	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", warehousesVersion); err != nil {
		t.Fatal(err)
	}

	t.Run("ninguna existencia cambia de total por sucursal y artículo", func(t *testing.T) {
		after := totals()
		if len(after) != len(before) {
			t.Fatalf("antes había %d existencias por sucursal y artículo, después %d", len(before), len(after))
		}
		for k, v := range before {
			if after[k] != v {
				t.Errorf("sucursal %d, artículo %d: antes %s, después %s", k[0], k[1], v, after[k])
			}
		}
	})

	t.Run("nada queda sin almacén y cada sucursal tiene su almacén de venta", func(t *testing.T) {
		var n int
		if err := st.Pool.QueryRow(ctx, `
			select (select count(*) from stock_levels where warehouse_id is null)
			     + (select count(*) from stock_movements where warehouse_id is null)
			     + (select count(*) from stock_counts where warehouse_id is null)
			     + (select count(*) from waste_records where warehouse_id is null)
			     + (select count(*) from branch_stock_minimums where warehouse_id is null)
			     + (select count(*) from notifications where warehouse_id is null)
			     + (select count(*) from stock_out_reports where warehouse_id is null)
			     + (select count(*) from expense_items where item_type is not null and warehouse_id is null)
			     + (select count(*) from count_places where warehouse_id is null)`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%d registros quedaron sin almacén", n)
		}
		if err := st.Pool.QueryRow(ctx, `
			select count(*) from branches b where (select count(*) from warehouses w
			  where w.company_id = b.company_id and w.branch_id = b.id and w.is_sales) <> 1`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%d sucursales sin exactamente un almacén de venta", n)
		}
	})

	var company, branch, ingredient int64
	if err := st.Pool.QueryRow(ctx, `
		select s.company_id, s.branch_id, s.ingredient_id from stock_levels s
		 where s.ingredient_id is not null order by s.company_id desc, s.ingredient_id limit 1`).Scan(&company, &branch, &ingredient); err != nil {
		t.Fatal(err)
	}
	conn := conexionDeEmpresa(t, st, company)

	t.Run("la API anterior escribe sin almacén y cae en el de venta", func(t *testing.T) {
		var before, after string
		if err := conn.QueryRow(ctx, `select on_hand::text from stock_levels where branch_id = $1 and ingredient_id = $2`,
			branch, ingredient).Scan(&before); err != nil {
			t.Fatal(err)
		}
		// El insert de un ajuste tal como lo hace la versión anterior: sin sucursal ni almacén.
		if _, err := conn.Exec(ctx, `
			insert into stock_movements (item_type, ingredient_id, movement_type, quantity, reason)
			values ('ingrediente', $1, 'ajuste', 1, 'prueba 0120')`, ingredient); err != nil {
			t.Fatalf("un ajuste sin almacén: %v", err)
		}
		if err := conn.QueryRow(ctx, `select on_hand::text from stock_levels where branch_id = $1 and ingredient_id = $2`,
			branch, ingredient).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if before == after {
			t.Fatal("el ajuste no movió la existencia")
		}
		var sales bool
		if err := conn.QueryRow(ctx, `
			select w.is_sales from stock_movements m join warehouses w on w.id = m.warehouse_id
			 where m.reason = 'prueba 0120'`).Scan(&sales); err != nil {
			t.Fatal(err)
		}
		if !sales {
			t.Fatal("el movimiento sin almacén no cayó en el de venta")
		}
		// Un lugar de conteo y un renglón de compra nacen con el almacén de venta de la matriz.
		if _, err := conn.Exec(ctx, `insert into count_places (name, position) values ('Lugar 0120', 99)`); err != nil {
			t.Fatalf("crear un lugar de conteo como la versión anterior: %v", err)
		}
		var placeSales bool
		if err := conn.QueryRow(ctx, `select w.is_sales from count_places p join warehouses w on w.id = p.warehouse_id
			where p.name = 'Lugar 0120'`).Scan(&placeSales); err != nil || !placeSales {
			t.Fatalf("el lugar nuevo no quedó en el almacén de venta: %v", err)
		}
		// Los upserts de la versión anterior siguen encontrando su llave por sucursal.
		if _, err := conn.Exec(ctx, `
			insert into branch_stock_minimums (branch_id, item_type, ingredient_id, min_stock, updated_by)
			values ($1, 'ingrediente', $2, 3, (select id from users where company_id = $3 order by id limit 1))
			on conflict (branch_id, ingredient_id) where ingredient_id is not null do update set min_stock = excluded.min_stock`,
			branch, ingredient, company); err != nil {
			t.Fatalf("el mínimo como lo guarda la versión anterior: %v", err)
		}
	})

	var bodega int64
	t.Run("nada entra a otro almacén mientras rijan las llaves por sucursal", func(t *testing.T) {
		if err := conn.QueryRow(ctx, `insert into warehouses (branch_id, name) values ($1, 'Bodega') returning id`,
			branch).Scan(&bodega); err != nil {
			t.Fatalf("dar de alta la bodega: %v", err)
		}
		_, err := conn.Exec(ctx, `
			insert into stock_movements (warehouse_id, item_type, ingredient_id, movement_type, quantity, reason)
			values ($1, 'ingrediente', $2, 'ajuste', 1, 'a la bodega')`, bodega, ingredient)
		exigeCodigo(t, err, "EGB04", "un movimiento a la bodega")
		_, err = conn.Exec(ctx, `update warehouses set is_sales = true where id = $1`, bodega)
		exigeCodigo(t, err, "EGB04", "cambiar el almacén de venta")
		_, err = conn.Exec(ctx, `update warehouses set is_active = false where branch_id = $1 and is_sales`, branch)
		exigeViolacionDeRestriccion(t, err, "archivar el almacén de venta")
	})

	t.Run("un almacén de otra sucursal no se guarda aunque RLS no estuviera", func(t *testing.T) {
		var other int64
		if err := st.Pool.QueryRow(ctx, `select id from warehouses where company_id <> $1 limit 1`, company).Scan(&other); err != nil {
			t.Fatal(err)
		}
		// Como owner: RLS no aplica, la llave compuesta es la única barrera.
		_, err := st.Pool.Exec(ctx, `update stock_levels set warehouse_id = $1 where branch_id = $2 and ingredient_id = $3`,
			other, branch, ingredient)
		exigeViolacionDeRestriccion(t, err, "una existencia en el almacén de otra empresa")
	})

	t.Run("el Down se niega con otro almacén y, sin él, vuelve exacto", func(t *testing.T) {
		if err := goose.DownContext(ctx, prepararGoose(t, st.Pool), "."); err == nil {
			t.Fatal("el Down bajó con una bodega dada de alta")
		}
		migrarAbajoHasta(t, st.Pool, warehousesVersion-1)
		after := totals()
		for k, v := range before {
			if k[1] == ingredient && k[0] == branch {
				continue // el ajuste de la prueba
			}
			if after[k] != v {
				t.Errorf("después del Down, sucursal %d artículo %d: %s, antes %s", k[0], k[1], after[k], v)
			}
		}
		// La versión anterior vuelve a sumar por sucursal.
		if _, err := conn.Exec(ctx, `
			insert into stock_movements (item_type, ingredient_id, movement_type, quantity, reason)
			values ('ingrediente', $1, 'ajuste', -1, 'prueba 0120 de regreso')`, ingredient); err != nil {
			t.Fatalf("un ajuste después del Down: %v", err)
		}
	})
}

// exigeCodigo falla si err no trae el SQLSTATE esperado.
func exigeCodigo(t *testing.T, err error, code, what string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Fatalf("%s: esperaba %s, llegó %v", what, code, err)
	}
}
