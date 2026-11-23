//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LA MERMA REAL DE 90 DÍAS (spec 096): lo registrado como merma que restó entre lo que las ventas
// consumieron, por almacén. Solo informa. Aquí: la leche del catálogo de costos con 10,000 g vendidos
// (y 1,000 devueltos por una cancelación) y 450 g mermados en «Local», más 400 g mermados en una
// «Bodega» que no vende. Lo que no restó (por aprobar, comida del personal) no cuenta.
type realWasteFixture struct {
	k              costCatalog
	local, bodega  int64
	order, leche1L int64
}

func seedRealWaste(t *testing.T, owner *store.Store) realWasteFixture {
	t.Helper()
	ctx := context.Background()
	f := realWasteFixture{k: seedCostCatalog(t, owner, defaultCompanyID, "")}
	q := func(dst *int64, sql string, args ...any) {
		t.Helper()
		if err := owner.Pool.QueryRow(ctx, sql, args...).Scan(dst); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	ex := func(sql string, args ...any) {
		t.Helper()
		if _, err := owner.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	co := f.k.company
	q(&f.local, `select w.id from warehouses w join branches b on b.id = w.branch_id
		where w.company_id = $1 and b.is_headquarters and w.is_sales`, co)
	q(&f.bodega, `insert into warehouses (company_id, branch_id, name, is_sales)
		select company_id, branch_id, 'Bodega merma', false from warehouses where id = $1 returning id`, f.local)
	q(&f.order, `insert into orders (company_id, client_uuid, business_date, daily_number, service_type, subtotal, total, opened_by)
		values ($1, gen_random_uuid(), current_date, 9602, 'mostrador', 60, 60, $2) returning id`, co, f.k.admin)
	// Un artículo en litros que después se une a la leche (en g, factor 1000): su venta y su merma
	// de antes de la unión siguen con su id.
	q(&f.leche1L, `insert into ingredients (company_id, name, base_unit_id, current_cost)
		select $1, 'Leche litro merma', id, 20 from units where code = 'l' returning id`, co)

	mov := func(ing int64, qty string, kind string) {
		// Con su almacén: una cancelación sin él la rechaza la base (repone al almacén del que salió).
		ex(`insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity, order_id, warehouse_id)
			values ($1, 'ingrediente', $2, $3::stock_movement_type, $4::numeric, $5, $6)`, co, ing, kind, qty, f.order, f.local)
	}
	mov(f.k.leche, "-10000", "venta")
	mov(f.k.leche, "1000", "cancelacion")
	mov(f.leche1L, "-1", "venta") // 1 l = 1,000 g

	waste := func(ing, wh int64, qty, status string, staff bool) {
		ex(`insert into waste_records (company_id, branch_id, warehouse_id, reason_id, item_type, ingredient_id, quantity, entered_qty, status, recorded_by, is_staff_meal)
			select $1, w.branch_id, w.id, (select min(id) from waste_reasons where company_id = $1), 'ingrediente', $3, $4::numeric, $4::numeric, $5, $6, $7
			  from warehouses w where w.id = $2`, co, wh, ing, qty, status, f.k.admin, staff)
	}
	waste(f.k.leche, f.local, "400", "applied", false)
	waste(f.leche1L, f.local, "0.05", "applied", false) // 50 g
	waste(f.k.leche, f.local, "300", "pending", false)  // por aprobar: no restó
	waste(f.k.leche, f.local, "200", "applied", true)   // comida del personal: no es pérdida
	waste(f.k.leche, f.bodega, "400", "applied", false)

	ex(`insert into ingredient_replacements (company_id, old_ingredient_id, new_ingredient_id, factor, kind, user_id)
		values ($1, $2, $3, 1000, 'merge', $4)`, co, f.leche1L, f.k.leche, f.k.admin)
	return f
}

func TestRealWasteByWarehouse(t *testing.T) {
	owner := newTestStore(t)
	f := seedRealWaste(t, owner)
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, f.k.company, f.k.admin)

	got, err := app.NewCostingService(st).ArticleRealWaste(ctx, f.k.leche)
	if err != nil {
		t.Fatal(err)
	}
	by := map[int64]app.RealWasteView{}
	for _, v := range got {
		by[v.WarehouseID] = v
	}
	// Local: (400 + 50) mermados / (10,000 − 1,000 + 1,000) vendidos = 4.5 %.
	if l := by[f.local]; l.State != domain.RealWasteRatio || l.Ratio == nil || *l.Ratio != 4.5 {
		t.Fatalf("Local = %+v; quiere 4.5 %% (sin la merma por aprobar, sin la del personal, con lo del artículo unido)", l)
	}
	if b := by[f.bodega]; b.State != domain.RealWasteNoSales || b.Ratio != nil || !b.Wasted.Equal(wpct("400").Round(4)) {
		t.Fatalf("Bodega = %+v; quiere 400 g mermados y «sin ventas»", b)
	}
}

func TestProductCostingJSONAndRealWaste(t *testing.T) {
	owner := newTestStore(t)
	f := seedRealWaste(t, owner)
	st := appRoleStore(t)
	ctx := tenantCtx(t, st, f.k.company, f.k.admin)
	svc := app.NewCostingService(st)

	v, err := svc.ProductCosting(ctx, f.k.frappe, true)
	if err != nil {
		t.Fatal(err)
	}
	if v.Source != "receta" || len(v.Lines) != 1 || !v.Cost.Equal(wpct("4").Round(4)) {
		t.Fatalf("desglose del frappé = %+v", v)
	}
	// El paquete es combo: sin renglones de receta, con sus productos. El JSON crudo dice [] y no null:
	// la pantalla recorre los dos arreglos (AGENTS.md, slices nil).
	combo, err := svc.ProductCosting(ctx, f.k.paquete, true)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(combo)
	if !strings.Contains(string(raw), `"lines":[]`) || !strings.Contains(string(raw), `"components":[{`) {
		t.Fatalf("JSON del combo: %s", raw)
	}
	// Quien configura el menú pero no ve el almacén recibe el desglose sin la merma real (arreglo vacío,
	// nunca null): son consumo y mermas por almacén, que su ruta propia pide con inventory.view.
	sinAlmacen, err := svc.ProductCosting(ctx, f.k.frappe, false)
	if err != nil {
		t.Fatal(err)
	}
	oculto, _ := json.Marshal(sinAlmacen)
	if !strings.Contains(string(oculto), `"realWaste":[]`) || strings.Contains(string(oculto), `"warehouseName"`) {
		t.Fatalf("sin permiso de almacén el desglose trae la merma real: %s", oculto)
	}
	frappe, _ := json.Marshal(v)
	if !strings.Contains(string(frappe), `"components":[]`) || !strings.Contains(string(frappe), `"realWaste":[`) {
		t.Fatalf("JSON del frappé: %s", frappe)
	}
}

func TestRealWasteAndCostingAreIsolated(t *testing.T) {
	owner := newTestStore(t)
	f := seedRealWaste(t, owner)
	other := makeCompany(t, owner, "otra-merma-real")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewCostingService(st)
		if _, err := svc.ArticleRealWaste(ctx, f.k.leche); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("merma real de un artículo ajeno = %v; quiere ErrNotFound", err)
		}
		if _, err := svc.ProductCosting(ctx, f.k.frappe, true); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("desglose de un producto ajeno = %v; quiere ErrNotFound", err)
		}
	})
}
