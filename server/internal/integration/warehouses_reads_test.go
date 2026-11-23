//go:build integration

package integration

import (
	"context"
	"errors"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// El código de la primera subida de la spec 094 es la «versión anterior» durante la segunda: ahí ya
// puede haber existencias del mismo artículo en dos almacenes de una sucursal. Estas pruebas ponen la
// base como quedará tras la segunda migración (sin las llaves por sucursal ni la compuerta EGB04) y
// comprueban que cada lectura, bloqueo y reposición sigue diciendo la verdad.

// comoEnLaSegundaSubida da de alta «Bodega» en la matriz. Desde la 0122 la base ya admite existencias
// del mismo artículo en dos almacenes; antes, esta función quitaba a mano lo que la 0120 dejaba. Devuelve
// los dos almacenes.
func comoEnLaSegundaSubida(t *testing.T, st *store.Store, company int64) (local, bodega int64) {
	t.Helper()
	ctx := context.Background()
	if err := st.Pool.QueryRow(ctx, `
		select w.id from warehouses w join branches b on b.id = w.branch_id
		 where w.company_id = $1 and b.is_headquarters and w.is_sales`, company).Scan(&local); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `
		insert into warehouses (company_id, branch_id, name)
		select $1, id, 'Bodega' from branches where company_id = $1 and is_headquarters returning id`, company).Scan(&bodega); err != nil {
		t.Fatal(err)
	}
	return local, bodega
}

func adjustIn(t *testing.T, st *store.Store, warehouse int64, itemType string, id int64, qty string) {
	t.Helper()
	col := "product_id"
	if itemType == "ingrediente" {
		col = "ingredient_id"
	}
	if _, err := st.Pool.Exec(context.Background(),
		"insert into stock_movements (warehouse_id, item_type, "+col+", movement_type, quantity) values ($1, $2::stock_item_type, $3, 'ajuste', $4::numeric)",
		warehouse, itemType, id, qty); err != nil {
		t.Fatal(err)
	}
}

func onHandIn(t *testing.T, st *store.Store, warehouse int64, col string, id int64) decimal.Decimal {
	t.Helper()
	var v decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(),
		"select coalesce((select on_hand from stock_levels where warehouse_id = $1 and "+col+" = $2), 0)", warehouse, id).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func headquarters(t *testing.T, st *store.Store, company int64) int64 {
	t.Helper()
	var b int64
	if err := st.Pool.QueryRow(context.Background(), `select id from branches where company_id = $1 and is_headquarters`, company).Scan(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

// Con leche en Local (5) y en Bodega (20), lo que la pantalla y el conteo leen es lo del almacén de
// venta: ni el artículo repetido, ni la suma, ni «more than one row».
func TestStockReadsUseTheSalesWarehouse(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	_, bodega := comoEnLaSegundaSubida(t, st, defaultCompanyID)
	leche := ingredientIn(t, st, defaultCompanyID, "Leche dos almacenes", "ml")
	adjust(t, st, "ingrediente", leche, "5")
	adjustIn(t, st, bodega, "ingrediente", leche, "20")
	branch := headquarters(t, st, defaultCompanyID)
	cinco := decimal.RequireFromString("5")

	t.Run("Existencias", func(t *testing.T) {
		svc, tctx := asTenant(t, defaultCompanyID)
		rows, err := svc.StockLevels(tctx, domain.WarehouseSel{})
		if err != nil {
			t.Fatal(err)
		}
		var got []db.ListStockLevelsRow
		for _, r := range rows {
			if r.ItemName == "Leche dos almacenes" {
				got = append(got, r.ListStockLevelsRow)
			}
		}
		if len(got) != 1 || !got[0].OnHand.Equal(cinco) {
			t.Fatalf("Existencias trae la leche %d veces (%v): debe salir una, con lo del almacén de venta (5)", len(got), got)
		}
	})
	t.Run("lista de insumos", func(t *testing.T) {
		rows, err := st.Q.ListIngredients(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, r := range rows {
			if r.ID == leche {
				n++
				if !r.OnHand.Equal(cinco) {
					t.Fatalf("la lista de insumos dice %s: debe ser lo del almacén de venta (5)", r.OnHand)
				}
			}
		}
		if n != 1 {
			t.Fatalf("la lista de insumos trae la leche %d veces", n)
		}
	})
	t.Run("lo que hay por sucursal suma sus almacenes", func(t *testing.T) {
		rows, err := st.Q.ArticleOnHandByBranch(ctx, &leche)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || !rows[0].OnHand.Equal(decimal.RequireFromString("25")) {
			t.Fatalf("por sucursal: %v, quería una fila con 25", rows)
		}
	})
	t.Run("captura del conteo", func(t *testing.T) {
		got, err := st.Q.OnHandForCapture(ctx, db.OnHandForCaptureParams{IngredientID: leche, BranchID: branch})
		if err != nil {
			t.Fatalf("la captura del conteo truena con dos almacenes: %v", err)
		}
		if !got.OnHand.Equal(cinco) {
			t.Fatalf("la captura lee %s: debe ser lo del almacén de venta (5)", got.OnHand)
		}
	})
	t.Run("se acabó", func(t *testing.T) {
		got, err := st.Q.StockOnHandForUpdate(ctx, db.StockOnHandForUpdateParams{BranchID: branch, IngredientID: &leche})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || !got[0].Equal(cinco) {
			t.Fatalf("«se acabó» tomaría %v: debe ser solo el almacén de venta (5), o dejaría la bodega en cero", got)
		}
	})
	t.Run("vista previa de alertas", func(t *testing.T) {
		got, err := st.Q.LowStockSnapshot(ctx, db.LowStockSnapshotParams{BranchID: branch, IngredientID: &leche})
		if err != nil {
			t.Fatalf("la vista previa de alertas truena con dos almacenes: %v", err)
		}
		if !got.OnHand.Equal(cinco) {
			t.Fatalf("la vista previa lee %s: debe ser lo del almacén de venta (5)", got.OnHand)
		}
	})
	t.Run("recalcular tras cambiar de unidad, por almacén", func(t *testing.T) {
		if _, err := st.Pool.Exec(ctx, `update stock_levels set on_hand = 999 where ingredient_id = $1`, leche); err != nil {
			t.Fatal(err)
		}
		if err := st.Q.RecomputeArticleStock(ctx, &leche); err != nil {
			t.Fatal(err)
		}
		if b := onHandIn(t, st, bodega, "ingredient_id", leche); !b.Equal(decimal.RequireFromString("20")) {
			t.Fatalf("la bodega quedó en %s tras recalcular: es la suma de sus movimientos (20), no la de la sucursal", b)
		}
	})
}

// Unir dos insumos con existencia en los dos almacenes traspasa cada almacén por separado: la bodega
// del origen pasa a la bodega del destino, no al local.
func TestMergeMovesEachWarehouse(t *testing.T) {
	t.Parallel()
	k := newMergeKit(t)
	local, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	adjustIn(t, k.st, bodega, "ingrediente", k.b, "200")
	k.merge(t, k.a, k.req(k.b))
	if v := onHandIn(t, k.st, bodega, "ingredient_id", k.b); !v.IsZero() {
		t.Fatalf("la bodega del origen quedó en %s: la unión no la traspasó", v)
	}
	if v := onHandIn(t, k.st, bodega, "ingredient_id", k.a); !v.Equal(decimal.RequireFromString("200")) {
		t.Fatalf("la bodega del destino quedó en %s, quería 200", v)
	}
	if v := onHandIn(t, k.st, local, "ingredient_id", k.a); !v.Equal(decimal.RequireFromString("800")) {
		t.Fatalf("el local del destino quedó en %s, quería 800 (500 + 300)", v)
	}
}

// Cancelar un renglón repone al almacén del que salió la venta aunque después hayan cambiado cuál es
// el de venta (FR-011): si no, el local queda descontado para siempre y la bodega gana lo que nunca
// tuvo.
func TestCancellingRestocksTheWarehouseTheSaleLeft(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	local, bodega := comoEnLaSegundaSubida(t, st, defaultCompanyID)
	cajero := makeUser(t, st, "cajero_repone_almacen", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	refresco := makeProduct(t, st, "Refresco del local", pesos("25"), true)
	sinPreparacion(t, st, refresco)

	appSt := appRoleStore(t)
	tctx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	svc := app.NewOrdersService(appSt, clock)
	ord := crearPedidoSimple(t, tctx, svc, refresco, cajero)
	linea, _ := primerRenglon(t, st, ord)
	if v := onHandIn(t, st, local, "product_id", refresco); !v.Equal(decimal.RequireFromString("-1")) {
		t.Fatalf("la venta no salió del local: %s", v)
	}

	// Dos sentencias en una transacción: el índice de «uno de venta por sucursal» se revisa renglón por
	// renglón, y la 0122 exige que al confirmar haya uno.
	tx, err := st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `update warehouses set is_sales = false where id = $1`, local); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `update warehouses set is_sales = true where id = $1`, bodega); err != nil {
		t.Fatalf("hacer de la bodega el almacén de venta: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CancelarRenglon(tctx, ord, linea, cajero, "Ya no lo quiere"); err != nil {
		t.Fatalf("CancelarRenglon: %v", err)
	}
	if v := onHandIn(t, st, local, "product_id", refresco); !v.IsZero() {
		t.Fatalf("el local quedó en %s: la cancelación no repuso donde salió la venta", v)
	}
	if v := onHandIn(t, st, bodega, "product_id", refresco); !v.IsZero() {
		t.Fatalf("la bodega quedó en %s: la cancelación repuso donde nunca hubo venta", v)
	}
}

// En el reemplazo por cambio de unidad, la cifra corregida a mano es de la sucursal. Con existencia
// en dos almacenes de esa sucursal no hay a cuál aplicarla: se rechaza en vez de descartarla en
// silencio y guardar algo distinto de lo que confirmó la pantalla (revisión de la 094).
func TestAStockOverrideWithTwoWarehousesIsRejected(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	_, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	if _, err := k.svc.CreateExpense(ctx, k.bultos("1", "500")); err != nil {
		t.Fatal(err)
	}
	adjustIn(t, k.st, bodega, "ingrediente", k.azucar, "1000")
	branch := headquarters(t, k.st, defaultCompanyID)
	err := k.svc.ChangeUnit(ctx, k.azucar, app.UnitChangeRequest{BaseUnitID: unitID(t, k.st, "pieza"), Factor: qty("0.0002"),
		Overrides: map[string]decimal.Decimal{"stock:" + itoa(int(branch)): qty("7")}})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("corregir a mano la existencia de una sucursal con dos almacenes = %v; quiere ErrValidation", err)
	}
}

// «¿Hay existencia de este producto en este lugar de venta?» (spec 092, la única puerta) mide el
// almacén de venta: con la botella en la bodega y ninguna en el local, en el mostrador está agotada
// (#65, actualización del 2026-11-23).
func TestSoldOutMeasuresTheSalesWarehouse(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	_, bodega := comoEnLaSegundaSubida(t, st, defaultCompanyID)
	menu := app.NewMenuService(st, clock)
	bottle := makeProduct(t, st, "Botella en la bodega", dec("20"), true)
	setOversell(t, st, bottle, false)
	adjustIn(t, st, bodega, "producto", bottle, "10")
	ids, err := menu.SoldOut(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != bottle {
		t.Fatalf("agotados = %v; con 10 en la bodega y 0 en el local, la botella está agotada en el mostrador", ids)
	}
}
