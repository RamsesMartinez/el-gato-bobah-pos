//go:build integration

package integration

import (
	"context"
	"errors"
	"github.com/shopspring/decimal"
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
	t.Run("renombrar cambia el nombre y deja rastro", func(t *testing.T) {
		if err := svc.RenameWarehouse(ctx, bodega, "Bodega grande"); err != nil {
			t.Fatal(err)
		}
		if err := svc.RenameWarehouse(ctx, bodega, "Bodega"); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from audit_events where company_id = $1 and action = 'warehouse.updated'`, co).Scan(&n); err != nil || n != 2 {
			t.Fatalf("%d renombres en la bitácora, %v; quiere 2", n, err)
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
	// La existencia de la bodega se siembra directo, como owner.
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
	t.Parallel()
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
		if err := s.RestoreWarehouse(ctx, bodega); err == nil {
			t.Fatal("se reactivó la bodega de otra empresa")
		}
		if rows, err := s.StockLevels(ctx, domain.WarehouseSel{ID: bodega}); err == nil {
			t.Fatalf("se leyeron existencias de la bodega de otra empresa: %d renglones", len(rows))
		}
		if rows, err := s.StockLevels(ctx, domain.WarehouseSel{All: true}); err == nil && len(rows) > 0 {
			t.Fatalf("«todos» trajo existencias de otra empresa: %d renglones", len(rows))
		}
		ing := int64(1)
		if err := s.RecordMovement(ctx, domain.WarehouseSel{ID: bodega}, "ingrediente", &ing, nil, "ajuste", decimal.NewFromInt(1), "ajeno", 1); err == nil {
			t.Fatal("se ajustó en la bodega de otra empresa")
		}
	})
}

// EXISTENCIAS POR ALMACÉN (historia 3): por omisión el de venta, uno elegido o todos sumados.
func TestStockLevelsByWarehouse(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "existencias-por-almacen")
	other := makeCompany(t, st, "existencias-otra")
	svc, ctx := asWaste(t, co)
	bodega, err := svc.CreateWarehouse(ctx, "Bodega")
	if err != nil {
		t.Fatal(err)
	}
	leche := placeIngredient(t, st, co, "Leche")
	azucar := placeIngredient(t, st, co, "Azúcar")
	// Leche en el local y azúcar en la bodega, sembrados directo.
	if _, err := st.Pool.Exec(ctx, `
		insert into stock_levels (company_id, branch_id, warehouse_id, item_type, ingredient_id, on_hand)
		select w.company_id, w.branch_id, w.id, 'ingrediente'::stock_item_type, $2::bigint, 10 from warehouses w where w.company_id = $1 and w.is_sales
		union all
		select w.company_id, w.branch_id, w.id, 'ingrediente'::stock_item_type, $4::bigint, 25 from warehouses w where w.id = $3`,
		co, leche, bodega, azucar); err != nil {
		t.Fatal(err)
	}
	names := func(sel domain.WarehouseSel) map[string]string {
		rows, err := svc.StockLevels(ctx, sel)
		if err != nil {
			t.Fatalf("StockLevels(%+v): %v", sel, err)
		}
		out := map[string]string{}
		for _, r := range rows {
			out[r.ItemName] = r.OnHand.String()
		}
		return out
	}
	if got := names(domain.WarehouseSel{}); got["Leche"] != "10" || got["Azúcar"] != "" {
		t.Fatalf("por omisión (el de venta) = %v; quiere solo la leche", got)
	}
	if got := names(domain.WarehouseSel{ID: bodega}); got["Azúcar"] != "25" || got["Leche"] != "" {
		t.Fatalf("la bodega = %v; quiere solo el azúcar", got)
	}
	if got := names(domain.WarehouseSel{All: true}); got["Leche"] != "10" || got["Azúcar"] != "25" {
		t.Fatalf("todos = %v; quiere los dos", got)
	}
	osvc, octx := asWaste(t, other)
	if _, err := osvc.StockLevels(octx, domain.WarehouseSel{ID: bodega}); !errors.Is(err, domain.ErrWarehouseNotFound) {
		t.Fatalf("la bodega de otra empresa = %v; quiere «no existe»", err)
	}
	if err := svc.ArchiveWarehouse(ctx, bodega); err == nil {
		t.Fatal("se archivó una bodega con existencias")
	}
}

// Un ajuste manual (pantalla y MCP stock_adjust) va al almacén de venta por omisión; mientras rige la
// 0121 la bodega también recibe, y uno ajeno no existe.
func TestAManualAdjustmentGoesToTheWarehouseAsked(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "ajuste-por-almacen")
	other := makeCompany(t, st, "ajuste-otra")
	svc, ctx := asWaste(t, co)
	user := makeUserIn(t, st, co, "ajusta-almacen", "gerente")
	bodega, err := svc.CreateWarehouse(ctx, "Bodega")
	if err != nil {
		t.Fatal(err)
	}
	ing := placeIngredient(t, st, co, "Frijol")
	if err := svc.RecordMovement(ctx, domain.WarehouseSel{}, "ingrediente", &ing, nil, "ajuste", decimal.NewFromInt(5), "llegó", user); err != nil {
		t.Fatalf("ajuste al de venta: %v", err)
	}
	var sales bool
	if err := st.Pool.QueryRow(ctx, `select w.is_sales from stock_movements m join warehouses w on w.id = m.warehouse_id
		where m.ingredient_id = $1`, ing).Scan(&sales); err != nil || !sales {
		t.Fatalf("el ajuste sin almacén no quedó en el de venta: %v", err)
	}
	// Desde la 0121 la bodega recibe ajustes: entra a ella y no al local.
	if err := svc.RecordMovement(ctx, domain.WarehouseSel{ID: bodega}, "ingrediente", &ing, nil, "ajuste", decimal.NewFromInt(3), "a la bodega", user); err != nil {
		t.Fatalf("ajuste a la bodega: %v", err)
	}
	var inBodega string
	if err := st.Pool.QueryRow(ctx, `select on_hand::text from stock_levels where warehouse_id = $1 and ingredient_id = $2`, bodega, ing).Scan(&inBodega); err != nil || inBodega != "3.0000" {
		t.Fatalf("la bodega quedó con %q, %v; quiere 3", inBodega, err)
	}
	osvc, octx := asWaste(t, other)
	ouser := makeUserIn(t, st, other, "ajusta-ajena", "gerente")
	oing := placeIngredient(t, st, other, "Arroz")
	if err := osvc.RecordMovement(octx, domain.WarehouseSel{ID: bodega}, "ingrediente", &oing, nil, "ajuste", decimal.NewFromInt(1), "ajena", ouser); !errors.Is(err, domain.ErrWarehouseNotFound) {
		t.Fatalf("ajuste a la bodega de otra empresa = %v; quiere «no existe»", err)
	}
}

// Un almacén de OTRA SUCURSAL de la misma empresa: RLS no lo separa, lo separa el filtro de sucursal de
// la sesión. Con dos sucursales y sin selector la sesión no tiene sucursal (BRANCH_AMBIGUOUS); lo que
// importa es que ninguna ruta alcance el almacén de la otra.
func TestAWarehouseOfAnotherBranchIsNotReachable(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "almacen-otra-sucursal")
	svc, ctx := asWaste(t, co)
	var norte int64
	if err := st.Pool.QueryRow(ctx, `with b as (insert into branches (company_id, branch_number, code, name)
		values ($1, 2, 'NORTE', 'Norte') returning *)
		insert into warehouses (company_id, branch_id, name, is_sales) select company_id, id, 'Local Norte', true from b returning id`,
		co).Scan(&norte); err != nil {
		t.Fatal(err)
	}
	ing := placeIngredient(t, st, co, "Leche norte")
	if _, err := svc.StockLevels(ctx, domain.WarehouseSel{ID: norte}); err == nil {
		t.Fatal("se leyeron las existencias del almacén de otra sucursal")
	}
	if err := svc.RecordMovement(ctx, domain.WarehouseSel{ID: norte}, "ingrediente", &ing, nil, "ajuste", decimal.NewFromInt(1), "otra sucursal", 1); err == nil {
		t.Fatal("se ajustó en el almacén de otra sucursal")
	}
	if err := svc.RenameWarehouse(ctx, norte, "Mío"); err == nil {
		t.Fatal("se renombró el almacén de otra sucursal")
	}
	if err := svc.ArchiveWarehouse(ctx, norte); err == nil {
		t.Fatal("se archivó el almacén de otra sucursal")
	}
}

// Cambiar cuál es el almacén de venta (desde la 0121): la marca pasa entera, la sucursal nunca se
// queda sin uno, y uno archivado no puede ser el de venta.
func TestTheSalesWarehouseCanChange(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "cambia-almacen-venta")
	svc, ctx := asWaste(t, co)
	bodega, err := svc.CreateWarehouse(ctx, "Bodega")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetSalesWarehouse(ctx, bodega); err != nil {
		t.Fatalf("hacer de la bodega el de venta: %v", err)
	}
	ws, _ := svc.ListWarehouses(ctx, false)
	sales := 0
	for _, w := range ws {
		if w.IsSales {
			sales++
			if w.ID != bodega {
				t.Fatalf("el de venta quedó en %q", w.Name)
			}
		}
	}
	if sales != 1 {
		t.Fatalf("%d almacenes de venta; quiere uno", sales)
	}
	if err := svc.SetSalesWarehouse(ctx, bodega); err != nil {
		t.Fatalf("repetirlo no es error: %v", err)
	}
	other, _ := svc.CreateWarehouse(ctx, "Cuarto frío")
	if err := svc.ArchiveWarehouse(ctx, other); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetSalesWarehouse(ctx, other); !errors.Is(err, domain.ErrWarehouseNotFound) {
		t.Fatalf("hacer de venta uno archivado = %v; quiere «no existe»", err)
	}
	var n int
	if err := st.Pool.QueryRow(ctx, `select count(*) from audit_events where company_id = $1 and action = 'warehouse.updated'`, co).Scan(&n); err != nil || n != 1 {
		t.Fatalf("%d cambios en la bitácora, %v; quiere 1", n, err)
	}
}
