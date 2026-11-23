//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LOS LOTES, SUS ASIGNACIONES Y EL DESGLOSE DEL CONTEO, AISLADOS EN LOS TRES CASOS DONDE RLS YA FALLÓ
// (spec 098, FR-060). Y la llave compuesta no deja un lote en el almacén de otra sucursal (FR-062).
func TestStockLotsStayIsolated(t *testing.T) {
	t.Parallel()
	k := newLotKit(t)
	ctx := context.Background()
	other := makeCompany(t, k.st, "ajena-lotes")
	k.move(t, k.local, "compra", "100", day(k.today(t), 10))
	k.move(t, k.local, "venta", "-30", "")
	var countID int64
	if err := k.st.Pool.QueryRow(ctx, `insert into stock_counts (branch_id, warehouse_id, opened_by)
		select branch_id, id, $2 from warehouses where id = $1 returning id`, k.bodega, makeUser(t, k.st, "cuenta_lotes", "gerente")).Scan(&countID); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `insert into stock_count_line_lots (count_id, ingredient_id, expires_on, qty)
		values ($1, $2, current_date + 5, 3)`, countID, k.leche); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"stock_lots", "stock_lot_allocations", "stock_count_line_lots"} {
		t.Run(table, func(t *testing.T) {
			var own int
			if err := k.st.Pool.QueryRow(ctx, "select count(*) from "+table).Scan(&own); err != nil || own == 0 {
				t.Fatalf("la dueña no tiene filas en %s (%d, %v): la prueba no probaría nada", table, own, err)
			}
			inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
				var n int
				if err := st.Pool.QueryRow(ctx, "select count(*) from "+table).Scan(&n); err != nil {
					// Sin empresa, la política compara contra NULL: cero filas, no error.
					t.Fatalf("leer %s: %v", table, err)
				}
				if n != 0 {
					t.Fatalf("%s: se ven %d filas de otra empresa", table, n)
				}
			})
		})
	}

	t.Run("un lote no cae en el almacén de otra sucursal", func(t *testing.T) {
		var otherBranch int64
		if err := k.st.Pool.QueryRow(ctx, `insert into branches (company_id, code, name) values ($1, 'LOTES2', 'Sucursal 2 lotes') returning id`,
			defaultCompanyID).Scan(&otherBranch); err != nil {
			t.Fatal(err)
		}
		_, err := k.st.Pool.Exec(ctx, `insert into stock_lots (branch_id, warehouse_id, ingredient_id, expires_on, qty)
			values ($1, $2, $3, current_date + 3, 1)`, otherBranch, k.local, k.leche)
		if err == nil {
			t.Fatal("se guardó un lote con el almacén de otra sucursal")
		}
	})
}
