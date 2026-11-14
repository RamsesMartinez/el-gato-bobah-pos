//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

const inventoryNotificationsVersion = 115

// LA 0115 (notificaciones de inventario, spec 084) CONTRA UN RESPALDO REAL CON DOS EMPRESAS. Agrega
// columnas, tablas, un trigger sobre los movimientos y un permiso: lo que se prueba es que las ventas
// y compras de la API anterior siguen entrando con el trigger puesto, que el aviso de una empresa no
// cruza a la otra, que el permiso llega a los cuatro sugeridos, y que el Down se niega con avisos.
func TestMigration0115OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	migrarAbajoHasta(t, st.Pool, inventoryNotificationsVersion-1)
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	// Un insumo activo y la primera sucursal de cada empresa: la empresa de pruebas casi no tiene
	// existencias, y el mínimo enorme de abajo deja a cualquiera bajo el mínimo.
	type art struct{ company, branch, ingredient int64 }
	var arts []art
	rows, err := st.Pool.Query(ctx, `
		select distinct on (i.company_id) i.company_id,
		       (select min(b.id) from branches b where b.company_id = i.company_id), i.id
		  from ingredients i
		 where i.is_active
		 order by i.company_id, i.id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a art
		if err := rows.Scan(&a.company, &a.branch, &a.ingredient); err != nil {
			t.Fatal(err)
		}
		arts = append(arts, a)
	}
	rows.Close()
	if len(arts) < 2 {
		t.Fatalf("el respaldo trae insumos de %d empresas; la prueba necesita dos", len(arts))
	}

	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", inventoryNotificationsVersion); err != nil {
		t.Fatal(err)
	}

	t.Run("nada se enciende solo: todas las alertas nacen apagadas", func(t *testing.T) {
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from ingredients where low_stock_enabled`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("alertas encendidas tras migrar: %d %v", n, err)
		}
		if err := st.Pool.QueryRow(ctx, `select count(*) from notifications`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("avisos tras migrar: %d %v", n, err)
		}
	})

	t.Run("el permiso nuevo llega a los cuatro sugeridos", func(t *testing.T) {
		var n int
		if err := st.Pool.QueryRow(ctx, `
			select count(*) from system_role_permissions sp join permissions p on p.id = sp.permission_id
			 where p.code = 'inventory.report_out'`).Scan(&n); err != nil || n != 4 {
			t.Fatalf("inventory.report_out en %d roles sugeridos: %v", n, err)
		}
	})

	a, b := arts[0], arts[1]
	t.Run("una venta de la API anterior entra y avisa solo en su empresa", func(t *testing.T) {
		app := restoredAppRoleStore(t)
		if _, err := st.Pool.Exec(ctx, `update ingredients set low_stock_enabled = true, min_stock = 1000000 where id = any($1)`,
			[]int64{a.ingredient, b.ingredient}); err != nil {
			t.Fatal(err)
		}
		// La API anterior no conoce las columnas nuevas: inserta el movimiento como siempre.
		tx, err := app.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, `select set_config('app.company_id', $1, true)`, itoa(int(a.company))); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `insert into stock_movements (item_type, ingredient_id, branch_id, movement_type, quantity)
			values ('ingrediente', $1, $2, 'venta', -1)`, a.ingredient, a.branch); err != nil {
			t.Fatalf("la venta de la API anterior falló con la 0115 puesta: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		var na, nb int
		if err := st.Pool.QueryRow(ctx, `select count(*) filter (where company_id = $1), count(*) filter (where company_id = $2) from notifications where kind = 'stock_low'`,
			a.company, b.company).Scan(&na, &nb); err != nil {
			t.Fatal(err)
		}
		if na != 1 || nb != 0 {
			t.Fatalf("avisos por empresa: %d en la que vendió, %d en la otra", na, nb)
		}
	})

	t.Run("el Down se niega con avisos y su limpieza lo deja bajar", func(t *testing.T) {
		if err := goose.DownContext(ctx, prepararGoose(t, st.Pool), "."); err == nil {
			t.Fatal("el Down bajó con avisos: perdería quién reportó qué")
		}
		migrarAbajoHasta(t, st.Pool, inventoryNotificationsVersion-1)
		var existe bool
		if err := st.Pool.QueryRow(ctx, `select to_regclass('notifications') is not null`).Scan(&existe); err != nil || existe {
			t.Fatalf("la tabla sigue tras el Down: %v %v", existe, err)
		}
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from permissions where id = 38`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("el permiso 38 sigue tras el Down: %d %v", n, err)
		}
	})
}
