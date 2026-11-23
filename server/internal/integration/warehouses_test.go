//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// ALMACENES (spec 094, historia 1): dar de alta la bodega sin que cambie nada de lo que hay.

func TestANewCompanyIsBornWithItsSalesWarehouse(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "almacen-nace")
	svc, ctx := asWaste(t, co)
	ws, err := svc.ListWarehouses(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 1 || !ws[0].IsSales || ws[0].Name != "Local" {
		t.Fatalf("una empresa nueva trae %+v; quiere un solo «Local» de venta", ws)
	}
}

func TestWarehouseLifecycle(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "almacen-ciclo")
	svc, ctx := asWaste(t, co)

	bodega, err := svc.CreateWarehouse(ctx, "  Bodega  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Run("nace vacía y no es la de venta", func(t *testing.T) {
		ws, _ := svc.ListWarehouses(ctx, false)
		if len(ws) != 2 || !ws[0].IsSales || ws[1].ID != bodega || ws[1].IsSales || ws[1].ItemsWithStock != 0 || ws[1].Name != "Bodega" {
			t.Fatalf("lista = %+v", ws)
		}
	})
	t.Run("el nombre no se repite sin distinguir mayúsculas, acentos ni espacios", func(t *testing.T) {
		if _, err := svc.CreateWarehouse(ctx, " BODEGA "); !errors.Is(err, domain.ErrWarehouseNameTaken) {
			t.Fatalf("otra «BODEGA» = %v; quiere nombre tomado", err)
		}
		if _, err := svc.CreateWarehouse(ctx, "local"); !errors.Is(err, domain.ErrWarehouseNameTaken) {
			t.Fatalf("otro «local» = %v; quiere nombre tomado", err)
		}
		if err := svc.RenameWarehouse(ctx, bodega, "LOCAL"); !errors.Is(err, domain.ErrWarehouseNameTaken) {
			t.Fatalf("renombrar como el de venta = %v; quiere nombre tomado", err)
		}
	})
	t.Run("el de venta no se archiva", func(t *testing.T) {
		ws, _ := svc.ListWarehouses(ctx, false)
		if err := svc.ArchiveWarehouse(ctx, ws[0].ID); !errors.Is(err, domain.ErrWarehouseIsSales) {
			t.Fatalf("archivar «Local» = %v", err)
		}
	})
	t.Run("archivar, reintentar y reactivar", func(t *testing.T) {
		if err := svc.ArchiveWarehouse(ctx, bodega); err != nil {
			t.Fatal(err)
		}
		if err := svc.ArchiveWarehouse(ctx, bodega); err != nil {
			t.Fatalf("un reintento de archivar no es error: %v", err)
		}
		if ws, _ := svc.ListWarehouses(ctx, false); len(ws) != 1 {
			t.Fatalf("archivada sigue en la lista: %+v", ws)
		}
		if err := svc.RenameWarehouse(ctx, bodega, "Otra"); !errors.Is(err, domain.ErrWarehouseNotFound) {
			t.Fatalf("renombrar una archivada = %v", err)
		}
		other, err := svc.CreateWarehouse(ctx, "Bodega")
		if err != nil {
			t.Fatalf("el nombre de una archivada queda libre: %v", err)
		}
		if err := svc.RestoreWarehouse(ctx, bodega); !errors.Is(err, domain.ErrWarehouseNameTaken) {
			t.Fatalf("reactivar con el nombre ya tomado = %v", err)
		}
		if err := svc.ArchiveWarehouse(ctx, other); err != nil {
			t.Fatal(err)
		}
		if err := svc.RestoreWarehouse(ctx, bodega); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("con un conteo abierto no se archiva", func(t *testing.T) {
		// El conteo de la bodega lo abre la base directo: abrirlo por la pantalla es de la subida 4.
		who := makeUserIn(t, st, co, "cuenta-bodega", "gerente")
		if _, err := st.Pool.Exec(ctx, `insert into stock_counts (company_id, branch_id, warehouse_id, opened_by)
			select w.company_id, w.branch_id, w.id, $2 from warehouses w where w.id = $1`, bodega, who); err != nil {
			t.Fatalf("abrir un conteo de prueba en la bodega: %v", err)
		}
		if err := svc.ArchiveWarehouse(ctx, bodega); !errors.Is(err, domain.ErrWarehouseCountOpen) {
			t.Fatalf("archivar con conteo abierto = %v", err)
		}
	})
	t.Run("deja su rastro en la bitácora", func(t *testing.T) {
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from audit_events
			where company_id = $1 and entity_type = 'warehouse' and action in
			  ('warehouse.created', 'warehouse.archived', 'warehouse.restored')`, co).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n < 4 {
			t.Fatalf("%d eventos de almacén; quiere al menos alta, alta, archivo y reactivación", n)
		}
	})
}

func TestAWarehouseWithStockIsNotArchived(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "almacen-con-existencia")
	svc, ctx := asWaste(t, co)
	bodega, err := svc.CreateWarehouse(ctx, "Bodega")
	if err != nil {
		t.Fatal(err)
	}
	// Mientras rige la 0117 nada entra a la bodega por un movimiento (EGB04): se siembra la existencia
	// directo, como owner.
	ing := placeIngredient(t, st, co, "Leche entera")
	if _, err := st.Pool.Exec(ctx, `insert into stock_levels (company_id, branch_id, warehouse_id, item_type, ingredient_id, on_hand)
		select w.company_id, w.branch_id, w.id, 'ingrediente', $2, 20 from warehouses w where w.id = $1`, bodega, ing); err != nil {
		t.Fatal(err)
	}
	var has domain.WarehouseHasStock
	if err := svc.ArchiveWarehouse(ctx, bodega); !errors.As(err, &has) || has.Items != 1 {
		t.Fatalf("archivar con 20 L = %v; quiere «tiene 1 artículo con existencia»", err)
	}
}

func TestWarehousesAreIsolatedInTheThreeCases(t *testing.T) {
	st := newTestStore(t)
	owner := makeCompany(t, st, "almacen-duena")
	other := makeCompany(t, st, "almacen-ajena")
	svc, ctx := asWaste(t, owner)
	bodega, err := svc.CreateWarehouse(ctx, "Bodega de la dueña")
	if err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, owner, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		s := app.NewBackofficeService(as, clock)
		ws, err := s.ListWarehouses(ctx, true)
		if err == nil {
			for _, w := range ws {
				if w.ID == bodega {
					t.Fatalf("se ve la bodega de la otra empresa: %+v", w)
				}
			}
		}
		if err := s.RenameWarehouse(ctx, bodega, "Mía"); err == nil {
			t.Fatal("se renombró la bodega de otra empresa")
		}
		if err := s.ArchiveWarehouse(ctx, bodega); err == nil {
			t.Fatal("se archivó la bodega de otra empresa")
		}
	})
}
