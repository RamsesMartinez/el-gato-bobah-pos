//go:build integration

package integration

import (
	"context"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Inventario y centavos de la spec 031: que cancelar no invente insumos y que cobrar o partir no
// inventen centavos.

// D11: CANCELAR EL PEDIDO COMPLETO NO REPONE LO QUE YA SALIÓ A COCINA.
//
// Quitar el renglón de un frappé ya preparado no reponía nada —se consumió— pero cancelar el pedido
// entero sí reponía todo: el mismo hecho físico dejaba dos inventarios distintos según el botón.
func TestCancellingTheWholeOrderFollowsTheSameRestockRuleAsRemovingALine(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_d11", "gerente")
	abrirCajaPrincipal(t, st, cajero)
	enCocina := makeProduct(t, st, "Frappé en cocina d11", dec("60"), true)
	refresco := makeProduct(t, st, "Refresco d11", dec("25"), true)
	sinPreparacion(t, st, refresco)

	ord, lines := twoLineOrderOf(t, ctx, orders, enCocina, refresco, cajero)
	_ = lines
	antesFrappe, antesRefresco := existencias(t, st, enCocina), existencias(t, st, refresco)
	if err := orders.CancelarConDevolucion(ctx, app.CancelacionCmd{CardFolio: "F-1", OrderID: ord, Motivo: "se fue", ActorID: cajero}); err != nil {
		t.Fatal(err)
	}
	if e := existencias(t, st, enCocina); !e.Equal(antesFrappe) {
		t.Fatalf("el frappé ya preparado pasó de %s a %s: se repuso un insumo que se consumió", antesFrappe, e)
	}
	if e := existencias(t, st, refresco); !e.GreaterThan(antesRefresco) {
		t.Fatalf("el refresco quedó en %s (antes %s): lo que no se prepara sí vuelve", e, antesRefresco)
	}
}

// D16: COBRAR UN CENTAVO DE MENOS NO SALDA EL PEDIDO.
//
// La tolerancia de un centavo daba por saldado un pedido de $100 cobrado en $99.99: el centavo
// restante rebotaba con «ya está cobrado» y la venta y el corte diferían para siempre.
func TestChargingOneCentLessDoesNotSettleTheOrder(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_d16", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)
	prod := makeProduct(t, st, "Centavo d16", dec("100"), false)
	sinPreparacion(t, st, prod)
	ord := crearPedidoSimple(t, ctx, orders, prod, cajero)

	res, err := orders.Charge(ctx, app.ChargeCmd{OrderID: ord, MethodID: efectivo, Amount: dec("99.99"), ActorID: cajero})
	if err != nil {
		t.Fatal(err)
	}
	if res.Paid || !res.Outstanding.Equal(dec("0.01")) {
		t.Fatalf("tras cobrar 99.99 de 100: saldado=%v, falta %s; quiere falta 0.01", res.Paid, res.Outstanding)
	}
	if _, err := orders.Charge(ctx, app.ChargeCmd{OrderID: ord, MethodID: efectivo, Amount: dec("0.01"), ActorID: cajero}); err != nil {
		t.Fatalf("cobrar el centavo que falta: %v", err)
	}
}

// D17: PARTIR UN RENGLÓN NO CREA UN CENTAVO.
//
// $45.55 partido a la mitad quedaba en 22.78 + 22.78 = 45.56: cada mitad se redondeaba hacia arriba.
func TestSplittingALineKeepsItsTotalToTheCent(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_d17", "gerente")
	abrirCajaPrincipal(t, st, cajero)
	prod := makeProduct(t, st, "Mitad d17", dec("45.55"), false)
	sinPreparacion(t, st, prod)
	ord := crearPedidoSimple(t, ctx, orders, prod, cajero)
	line, _ := primerRenglon(t, st, ord)
	half := dec("0.5")
	if _, err := orders.RemovePieces(ctx, ord, line, cajero, "media porción", &half); err != nil {
		t.Fatal(err)
	}
	var sum = dec("0")
	if err := st.Pool.QueryRow(ctx, `select sum(line_total) from order_lines where order_id = $1`, ord).Scan(&sum); err != nil {
		t.Fatal(err)
	}
	if !sum.Equal(dec("45.55")) {
		t.Fatalf("las dos mitades suman %s, quiere 45.55: partir no puede inventar un centavo", sum)
	}
}

// twoLineOrderOf deja un pedido vivo con un renglón de cada producto.
func twoLineOrderOf(t *testing.T, ctx context.Context, svc *app.OrdersService, a, b, actor int64) (int64, []int64) {
	t.Helper()
	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: actor,
		Lines: []domain.OrderLineInput{{ProductID: a, Qty: dec("1")}, {ProductID: b, Qty: dec("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return ord.ID, nil
}
