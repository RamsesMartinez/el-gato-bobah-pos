//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// VENDER Y CANCELAR CON LOTES (spec 098, historia 2), por el servicio real de pedidos.

type lotSaleKit struct {
	st      *store.Store
	svc     *app.OrdersService
	cashier int64
	perlas  int64
	frappe  int64
	today   time.Time
}

func newLotSaleKit(t *testing.T) *lotSaleKit {
	t.Helper()
	st := newTestStore(t)
	k := &lotSaleKit{st: st}
	k.cashier = makeUser(t, st, "cajero_lotes", "cajero")
	k.perlas = makeIngredient(t, st, "Perlas lotes")
	k.frappe = makeProduct(t, st, "Frappé lotes", decimal.RequireFromString("60"), false)
	// Una lata no se prepara: así cancelar repone aunque ya haya salido a cocina (spec 031, D11).
	sinPreparacion(t, st, k.frappe)
	recipe := makeRecipe(t, st, map[int64]string{k.perlas: "150"})
	if _, err := st.Pool.Exec(context.Background(), `update products set recipe_id = $2, cost_source = 'receta' where id = $1`, k.frappe, recipe); err != nil {
		t.Fatal(err)
	}
	abrirCajaPrincipal(t, st, k.cashier)
	k.svc = app.NewOrdersService(st, clock)
	if err := st.Pool.QueryRow(context.Background(), `select business_today($1)`, defaultCompanyID).Scan(&k.today); err != nil {
		t.Fatal(err)
	}
	return k
}

func (k *lotSaleKit) buy(t *testing.T, qty string, days int) string {
	t.Helper()
	d := k.today.AddDate(0, 0, days).Format(time.DateOnly)
	if _, err := k.st.Pool.Exec(context.Background(), `insert into stock_movements (item_type, ingredient_id, movement_type, quantity, expires_on)
		values ('ingrediente', $1, 'compra', $2::numeric, $3::date)`, k.perlas, qty, d); err != nil {
		t.Fatal(err)
	}
	return d
}

func (k *lotSaleKit) lot(t *testing.T, d string) decimal.Decimal {
	t.Helper()
	var v decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `select coalesce(sum(qty), 0) from stock_lots where ingredient_id = $1 and expires_on = $2::date`,
		k.perlas, d).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func (k *lotSaleKit) sell(t *testing.T, n string) (int64, int64) {
	t.Helper()
	order, err := k.svc.Create(context.Background(), app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: k.cashier,
		Lines: []domain.OrderLineInput{{ProductID: k.frappe, Qty: decimal.RequireFromString(n)}}})
	if err != nil {
		t.Fatal(err)
	}
	return order.ID, order.Lines[0].ID
}

// Cancelar un renglón que descontó de dos lotes repone a esos dos, aunque uno haya caducado mientras.
func TestCancellingALineRestocksTheLotsItTookFrom(t *testing.T) {
	t.Parallel()
	k := newLotSaleKit(t)
	d10, d20 := k.buy(t, "100", 10), k.buy(t, "100", 20)
	order, line := k.sell(t, "1") // 150 g: 100 del 10 y 50 del 20
	if v := k.lot(t, d10); !v.IsZero() {
		t.Fatalf("lote del %s = %s tras vender, quería 0", d10, v)
	}
	// Pasa el tiempo: el primer lote caduca.
	if _, err := k.st.Pool.Exec(context.Background(), `update stock_lots set expires_on = $2::date where ingredient_id = $1 and expires_on = $3::date`,
		k.perlas, k.today.AddDate(0, 0, -1).Format(time.DateOnly), d10); err != nil {
		t.Fatal(err)
	}
	d10 = k.today.AddDate(0, 0, -1).Format(time.DateOnly)
	if _, err := k.svc.CancelarRenglon(context.Background(), order, line, k.cashier, "se equivocó"); err != nil {
		t.Fatal(err)
	}
	if v := k.lot(t, d10); !v.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("el lote caducado recibió %s, quería 100 (lo que salió de él)", v)
	}
	if v := k.lot(t, d20); !v.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("el lote del %s quedó en %s, quería 100", d20, v)
	}
}

// Vender un lote hasta 0 y cancelar el pedido entero lo repone al lote, no a lo sin fecha (R12 A1).
func TestCancellingAnOrderRestocksALotThatWentToZero(t *testing.T) {
	t.Parallel()
	k := newLotSaleKit(t)
	d := k.buy(t, "150", 15)
	order, _ := k.sell(t, "1")
	if v := k.lot(t, d); !v.IsZero() {
		t.Fatalf("lote = %s tras vender todo", v)
	}
	if err := k.svc.CancelarConDevolucion(context.Background(), app.CancelacionCmd{CardFolio: "F-9", OrderID: order, Motivo: "se fue", ActorID: k.cashier}); err != nil {
		t.Fatal(err)
	}
	if v := k.lot(t, d); !v.Equal(decimal.NewFromInt(150)) {
		t.Fatalf("el lote quedó en %s tras cancelar, quería 150", v)
	}
}
