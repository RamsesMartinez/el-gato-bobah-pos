//go:build integration

package integration

import (
	"encoding/json"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// UNA CUENTA EN CAPTURA NO ES VENTA (FR-002, SC-001, caso 24 del lienzo).
//
// Es la razón de que la cuenta viva en su propia tabla (D-1): ninguna consulta de dinero ni de
// operación la puede contar. Se fotografía cada una con una venta real de por medio —para que la foto
// no sea trivialmente vacía—, se abre una cuenta de tres productos de los MISMOS productos, y cada
// foto tiene que salir idéntica. Falla nombrando la consulta que la contó.
func TestADraftIsNeverASale(t *testing.T) {
	k := newDraftsKit(t)
	ctx := k.ctx
	sessionID := abrirCajaPrincipal(t, k.st, k.user)
	taro := makeProduct(t, k.st, "Taro de la venta", pesos("55"), true)
	crepa := makeProduct(t, k.st, "Crepa de la venta", pesos("80"), true)

	// Una venta real, cobrada, para que cada consulta tenga algo que mostrar.
	ord, err := k.orders.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: k.user,
		Lines: []domain.OrderLineInput{{ProductID: taro, Qty: pesos("1")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := k.orders.Charge(ctx, app.ChargeCmd{OrderID: ord.ID, MethodID: paymentMethodID(t, k.st, "Efectivo"),
		Amount: ord.Total, ActorID: k.user}); err != nil {
		t.Fatal(err)
	}

	sales := app.NewSalesService(k.appSt, clock)
	back := app.NewBackofficeService(k.appSt, clock)
	menu := app.NewMenuService(k.appSt, clock)
	f := filtroBase(fixedNow)
	day := fixedNow
	var registerID int64
	if err := k.st.Pool.QueryRow(ctx, `select register_id from register_sessions where id = $1`, sessionID).Scan(&registerID); err != nil {
		t.Fatal(err)
	}

	queries := []struct {
		name string
		run  func() (any, error)
	}{
		{"ventas: lista", func() (any, error) { return sales.List(ctx, f) }},
		{"ventas: resumen", func() (any, error) { return sales.Summary(ctx, f) }},
		{"corte del turno", func() (any, error) { return back.CurrentByRegister(ctx, registerID) }},
		{"reportes: por día", func() (any, error) { return back.SalesByDay(ctx, day, day) }},
		{"reportes: por método", func() (any, error) { return back.SalesByMethod(ctx, day, day) }},
		{"reportes: productos vendidos (recetas y costeo)", func() (any, error) { return back.ProductsSold(ctx, day, day, 50) }},
		{"reportes: márgenes", func() (any, error) { return back.ProductMargins(ctx, day, day, 50) }},
		{"Top / populares", func() (any, error) { return menu.Popular(ctx) }},
		{"tablero de cocina", func() (any, error) { return k.orders.Board(ctx) }},
		{"almacén: existencias", func() (any, error) { return back.StockLevels(ctx) }},
		{"almacén: movimientos", func() (any, error) { return back.StockMovements(ctx, 100) }},
	}
	snap := func(t *testing.T) map[string]string {
		t.Helper()
		out := map[string]string{}
		for _, q := range queries {
			v, err := q.run()
			if err != nil {
				t.Fatalf("%s: %v", q.name, err)
			}
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			out[q.name] = string(b)
		}
		return out
	}
	before := snap(t)

	k.newDraft(t, addOf(taro, "2"), addOf(crepa, "1"), addOf(taro, "1"))

	after := snap(t)
	for _, q := range queries {
		if before[q.name] != after[q.name] {
			t.Errorf("%s contó la cuenta en captura:\n antes:   %s\n después: %s", q.name, before[q.name], after[q.name])
		}
	}
}
