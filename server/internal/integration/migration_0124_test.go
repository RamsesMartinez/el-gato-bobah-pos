//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

const transfersVersion = 124

// LAS 0123 Y 0124 (traspasos entre almacenes, spec 094) CONTRA UN RESPALDO REAL CON DOS EMPRESAS. Lo
// que se prueba: que el tipo `traspaso` y las tablas entran sin tocar nada de lo que hay, que un
// traspaso queda ligado a sus dos movimientos en su sucursal, y que el Down se niega con traspasos.
func TestMigration0124OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	migrarAbajoHasta(t, st.Pool, transfersVersion-2)
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	var before int
	if err := st.Pool.QueryRow(ctx, `select count(*) from stock_movements`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", transfersVersion); err != nil {
		t.Fatal(err)
	}
	var after int
	if err := st.Pool.QueryRow(ctx, `select count(*) from stock_movements where transfer_id is null`).Scan(&after); err != nil || after != before {
		t.Fatalf("antes %d movimientos, después %d sin traspaso (%v)", before, after, err)
	}

	var company, branch, local, ingredient, user int64
	if err := st.Pool.QueryRow(ctx, `select s.company_id, s.branch_id, s.warehouse_id, s.ingredient_id from stock_levels s
		join warehouses w on w.id = s.warehouse_id and w.is_sales
		where s.ingredient_id is not null order by s.company_id desc, s.ingredient_id limit 1`).Scan(&company, &branch, &local, &ingredient); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `select id from users where company_id = $1 order by id limit 1`, company).Scan(&user); err != nil {
		t.Fatal(err)
	}
	conn := conexionDeEmpresa(t, st, company)
	var bodega, transfer int64
	if err := conn.QueryRow(ctx, `insert into warehouses (branch_id, name) values ($1, 'Bodega 0124') returning id`, branch).Scan(&bodega); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, `insert into stock_transfers (branch_id, from_warehouse_id, to_warehouse_id, client_key, body_hash, created_by)
		values ($1, $2, $3, gen_random_uuid(), 'x', $4) returning id`, branch, local, bodega, user).Scan(&transfer); err != nil {
		t.Fatalf("un traspaso después de la 0124: %v", err)
	}
	for _, leg := range []struct {
		wh  int64
		qty string
	}{{local, "-1"}, {bodega, "1"}} {
		if _, err := conn.Exec(ctx, `insert into stock_movements (warehouse_id, item_type, ingredient_id, movement_type, quantity, transfer_id)
			values ($1, 'ingrediente', $2, 'traspaso', $3::numeric, $4)`, leg.wh, ingredient, leg.qty, transfer); err != nil {
			t.Fatalf("una pata del traspaso: %v", err)
		}
	}

	t.Run("un traspaso no se ve entre sucursales: su movimiento no puede ser de otra", func(t *testing.T) {
		var otherWh int64
		if err := st.Pool.QueryRow(ctx, `select id from warehouses where company_id <> $1 limit 1`, company).Scan(&otherWh); err != nil {
			t.Fatal(err)
		}
		_, err := st.Pool.Exec(ctx, `update stock_movements set warehouse_id = $1 where transfer_id = $2`, otherWh, transfer)
		exigeViolacionDeRestriccion(t, err, "un movimiento de traspaso en el almacén de otra empresa")
	})

	t.Run("el Down se niega con traspasos", func(t *testing.T) {
		if err := goose.DownContext(ctx, prepararGoose(t, st.Pool), "."); err == nil {
			t.Fatal("el Down bajó con un traspaso registrado")
		}
	})
}
