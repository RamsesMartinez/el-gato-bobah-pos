//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// setOwnStock deja la existencia propia de un producto en la sucursal matriz de su empresa.
func setOwnStock(t *testing.T, st *store.Store, product int64, onHand string) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(), `
		insert into stock_levels (company_id, branch_id, item_type, product_id, on_hand)
		select p.company_id, b.id, 'producto', p.id, $2::numeric
		  from products p join branches b on b.company_id = p.company_id and b.is_headquarters
		 where p.id = $1
		on conflict (branch_id, product_id) do update set on_hand = excluded.on_hand`, product, onHand); err != nil {
		t.Fatal(err)
	}
}

func setOversell(t *testing.T, st *store.Store, product int64, on bool) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(), `update products set allow_oversell = $2 where id = $1`, product, on); err != nil {
		t.Fatal(err)
	}
}

// LA LISTA DE AGOTADOS (spec 092, US2). Siempre un arreglo; solo los productos con «vender sin
// existencia» apagado y sin existencia para uno; la existencia de otra empresa no cuenta.
func TestSoldOutList(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	menu := app.NewMenuService(st, clock)
	ctx := context.Background()

	bottle := makeProduct(t, st, "Botella de agua", dec("20"), true)
	setOwnStock(t, st, bottle, "0")

	t.Run("sin productos con la opción apagada es un arreglo vacío, no null", func(t *testing.T) {
		ids, err := menu.SoldOut(ctx)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(app.SoldOutView{ProductIDs: ids})
		if string(raw) != `{"productIds":[]}` {
			t.Fatalf("respondió %s: un null tumba la pantalla de venta sin un solo error en el servidor", raw)
		}
	})

	setOversell(t, st, bottle, false)

	t.Run("con la opción apagada y sin existencia, sale", func(t *testing.T) {
		ids, err := menu.SoldOut(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(ids) != 1 || ids[0] != bottle {
			t.Fatalf("agotados = %v, se esperaba [%d]", ids, bottle)
		}
	})

	t.Run("reponer lo saca", func(t *testing.T) {
		setOwnStock(t, st, bottle, "3")
		ids, err := menu.SoldOut(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(ids) != 0 {
			t.Fatalf("un producto repuesto sigue agotado: %v", ids)
		}
		setOwnStock(t, st, bottle, "0")
	})

	t.Run("aislamiento en los tres casos", func(t *testing.T) {
		coB := makeCompany(t, st, "agotado-b")
		inTheThreeCases(t, defaultCompanyID, coB, func(t *testing.T, ast *store.Store, actx context.Context) {
			ids, err := app.NewMenuService(ast, clock).SoldOut(actx)
			if err != nil && !errors.Is(err, domain.ErrBranchAmbiguous) {
				// Sin empresa no hay sucursal: la consulta no puede resolverla, y eso no es una fuga.
				return
			}
			for _, id := range ids {
				if id == bottle {
					t.Fatal("una sesión ajena (o sin empresa) ve los agotados de otra empresa")
				}
			}
		})
	})
}

// DOS TABLETAS VENDEN LA ÚLTIMA PIEZA A LA VEZ: una pasa y la otra recibe «se agotó». Con la opción
// encendida las dos pasan; un pedido de plataforma pasa siempre; agregar a un pedido abierto también
// revisa.
func TestTwoCountersCannotSellTheLastPiece(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_agotado", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	last := makeProduct(t, st, "Última lata", dec("25"), true)
	setOversell(t, st, last, false)
	setOwnStock(t, st, last, "1")

	sell := func(qty string) error {
		_, err := orders.Create(ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines: []domain.OrderLineInput{{ProductID: last, Qty: dec(qty)}}})
		return err
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) { defer wg.Done(); errs[i] = sell("1") }(i)
	}
	wg.Wait()
	ok, out := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, domain.ErrOutOfStock):
			out++
		default:
			t.Fatalf("una de las ventas falló por otra cosa: %v", err)
		}
	}
	if ok != 1 || out != 1 {
		t.Fatalf("pasaron %d y se rechazaron %d: la última pieza se vendió dos veces o ninguna", ok, out)
	}

	t.Run("pedir de más dice cuántas quedan", func(t *testing.T) {
		setOwnStock(t, st, last, "1")
		var oos domain.OutOfStock
		if err := sell("2"); !errors.As(err, &oos) || oos.Left.String() != "1" || oos.ProductID != last {
			t.Fatalf("se esperaba OutOfStock con 1 de %d, fue %v", last, err)
		}
	})

	t.Run("agregar a un pedido abierto también revisa", func(t *testing.T) {
		setOwnStock(t, st, last, "0")
		other := makeProduct(t, st, "Café para agregar", dec("30"), false)
		ord, err := orders.Create(ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines: []domain.OrderLineInput{{ProductID: other, Qty: dec("1")}}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = orders.AddLines(ctx, ord.ID, []domain.OrderLineInput{{ProductID: last, Qty: dec("1")}}, cajero, uuid.New())
		if !errors.Is(err, domain.ErrOutOfStock) {
			t.Fatalf("agregar un producto agotado a un pedido abierto pasó: %v", err)
		}
	})

	t.Run("un pedido de plataforma pasa sin existencia", func(t *testing.T) {
		setOwnStock(t, st, last, "0")
		uber := platformID(t, st, defaultCompanyID, "Uber Eats")
		if _, err := orders.Create(ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "domicilio", OpenedBy: cajero,
			DeliveryPlatformID: &uber, Lines: []domain.OrderLineInput{{ProductID: last, Qty: dec("1")}}}); err != nil {
			t.Fatalf("un pedido de plataforma se rechazó por existencia: %v", err)
		}
	})

	t.Run("con la opción encendida se vende y la existencia queda negativa", func(t *testing.T) {
		setOversell(t, st, last, true)
		setOwnStock(t, st, last, "0")
		if err := sell("1"); err != nil {
			t.Fatal(err)
		}
		var onHand string
		if err := st.Pool.QueryRow(ctx, `select on_hand::text from stock_levels where product_id = $1`, last).Scan(&onHand); err != nil {
			t.Fatal(err)
		}
		if onHand != "-1.0000" {
			t.Fatalf("la existencia quedó en %s", onHand)
		}
	})
}
