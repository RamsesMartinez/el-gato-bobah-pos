//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Las devoluciones de la spec 031: lo que la auditoría encontró que salía dos veces, de más o por
// el medio equivocado.

// D1: LA SEGUNDA DEVOLUCIÓN SALE POR DONDE QUEDA DINERO, NO OTRA VEZ POR EL PRIMER MEDIO.
//
// $40 en efectivo y $60 con tarjeta; se devuelven $40 y luego $60. La segunda sacaba otros $40 de
// billetes (con su salida de caja) y solo $20 de tarjeta: el corte esperaba $40 menos de lo que
// había en el cajón y la tarjeta quedaba devuelta de menos.
func TestASecondRefundDoesNotRepeatTheFirstMethod(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_d1", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")
	abrirCajaPrincipal(t, st, cajero)

	ord := pedidoCobradoParcial(t, ctx, st, orders, "d1", "100", "40", cajero, efectivo, true)
	charge(t, ctx, orders, ord, tarjeta, "60", cajero)

	before := salidasDeCaja(t, st)
	refund(t, ctx, orders, ord, nil, "40", cajero)
	refund(t, ctx, orders, ord, nil, "60", cajero)

	if out := salidasDeCaja(t, st).Sub(before); !out.Equal(decimal.RequireFromString("40")) {
		t.Fatalf("del cajón salieron %s, quiere 40: la segunda devolución volvió a salir en efectivo", out)
	}
	byMethod := refundedByMethod(t, st, ord)
	if !byMethod[efectivo].Equal(decimal.RequireFromString("40")) || !byMethod[tarjeta].Equal(decimal.RequireFromString("60")) {
		t.Fatalf("devuelto por medio = efectivo %s, tarjeta %s; quiere 40 y 60", byMethod[efectivo], byMethod[tarjeta])
	}
}

// D3: DOS DEVOLUCIONES A LA VEZ NO PASAN LAS DOS EL TOPE.
//
// Devolver no bloqueaba el pedido: dos «devolver todo» simultáneos (dos tabletas, un doble toque con
// la red lenta) leían lo mismo y se registraban los dos, con dos salidas de caja.
func TestTwoSimultaneousRefundsDoNotBothPass(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_d3", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	for intento := 0; intento < 5; intento++ {
		ord := pedidoCobradoParcial(t, ctx, st, orders, "d3_"+itoa(intento), "400", "400", cajero, efectivo, true)
		errs := concurrently(2, func(int) error {
			return orders.Devolver(ctx, app.DevolucionCmd{
				OrderID: ord, Monto: decimal.RequireFromString("400"), Motivo: "doble toque", ActorID: cajero,
			})
		})
		ok := 0
		for _, err := range errs {
			switch {
			case err == nil:
				ok++
			case !errors.Is(err, domain.ErrDevolucionExcede):
				t.Fatalf("la devolución que pierde: err = %v, quiere ErrDevolucionExcede", err)
			}
		}
		if total := refundedTotal(t, st, ord); ok != 1 || !total.Equal(decimal.RequireFromString("400")) {
			t.Fatalf("intento %d: pasaron %d devoluciones y se devolvieron %s de 400 cobrados", intento, ok, total)
		}
	}
}

// D3: UNA DEVOLUCIÓN Y UNA CANCELACIÓN CON DEVOLUCIÓN A LA VEZ TAMPOCO DEVUELVEN DE MÁS.
func TestARefundAndACancellationTogetherDoNotOverRefund(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_d3b", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	for intento := 0; intento < 5; intento++ {
		ord := pedidoCobradoParcial(t, ctx, st, orders, "d3b_"+itoa(intento), "200", "200", cajero, efectivo, true)
		concurrently(2, func(i int) error {
			if i == 0 {
				return orders.Devolver(ctx, app.DevolucionCmd{
					OrderID: ord, Monto: decimal.RequireFromString("200"), Motivo: "a la vez", ActorID: cajero,
				})
			}
			return orders.CancelarConDevolucion(ctx, app.CancelacionCmd{OrderID: ord, Motivo: "a la vez", ActorID: cajero, Devolver: true})
		})
		if total := refundedTotal(t, st, ord); total.GreaterThan(decimal.RequireFromString("200")) {
			t.Fatalf("intento %d: se devolvieron %s de 200 cobrados", intento, total)
		}
	}
}

// D4: CONTRA UN RENGLÓN SE DEVUELVE LO QUE VALE ESE RENGLÓN.
func TestARefundAgainstALineIsCappedByTheLine(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_d4", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord, lines := twoLineOrder(t, ctx, st, orders, "d4", "60", "40", cajero)
	charge(t, ctx, orders, ord, efectivo, "100", cajero)

	// Sin monto = «todo lo que queda» de ese renglón: $60, no los $100 del pedido.
	queda, err := orders.PorDevolver(ctx, ord, &lines[0])
	if err != nil {
		t.Fatal(err)
	}
	if !queda.Equal(decimal.RequireFromString("60")) {
		t.Fatalf("por devolver del renglón de $60 = %s, quiere 60", queda)
	}
	if err := orders.Devolver(ctx, app.DevolucionCmd{OrderID: ord, LineID: &lines[0], Monto: decimal.RequireFromString("61"),
		Motivo: "platillo frío", ActorID: cajero}); !errors.Is(err, domain.ErrDevolucionExcede) {
		t.Fatalf("devolver 61 de un renglón de 60: err = %v, quiere ErrDevolucionExcede", err)
	}
	refundLine(t, ctx, orders, ord, lines[0], "60", cajero)
	if err := orders.Devolver(ctx, app.DevolucionCmd{OrderID: ord, LineID: &lines[0], Monto: decimal.RequireFromString("1"),
		Motivo: "otra vez", ActorID: cajero}); !errors.Is(err, domain.ErrDevolucionExcede) {
		t.Fatalf("devolver otra vez el mismo renglón: err = %v, quiere ErrDevolucionExcede", err)
	}

	// La cuenta entera ya devuelta deja en cero a cualquier renglón.
	refund(t, ctx, orders, ord, nil, "40", cajero)
	if err := orders.Devolver(ctx, app.DevolucionCmd{OrderID: ord, LineID: &lines[1], Monto: decimal.RequireFromString("1"),
		Motivo: "y éste", ActorID: cajero}); !errors.Is(err, domain.ErrDevolucionExcede) {
		t.Fatalf("devolver un renglón de un pedido ya devuelto entero: err = %v, quiere ErrDevolucionExcede", err)
	}
	if total := refundedTotal(t, st, ord); !total.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("se devolvieron %s de 100 cobrados", total)
	}

	// Un renglón de OTRO pedido no existe para éste.
	otro, otras := twoLineOrder(t, ctx, st, orders, "d4_otro", "10", "10", cajero)
	charge(t, ctx, orders, otro, efectivo, "20", cajero)
	if err := orders.Devolver(ctx, app.DevolucionCmd{OrderID: otro, LineID: &lines[1], Monto: decimal.RequireFromString("1"),
		Motivo: "renglón ajeno", ActorID: cajero}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("devolver contra un renglón de otro pedido: err = %v, quiere ErrNotFound", err)
	}
	_ = otras
}

// D18: UN PEDIDO REEMBOLSADO POR EL FLUJO VIEJO NO SE VUELVE A DEVOLVER.
//
// `Refund` marcaba el pedido sin escribir el libro de devoluciones, así que para el libro ese pedido
// no tenía nada devuelto y se podía devolver completo otra vez, con otra salida de caja.
func TestARefundedOrderFromTheOldFlowIsNotRefundedAgain(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_d18", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord := pedidoCobradoParcial(t, ctx, st, orders, "d18", "220", "220", cajero, efectivo, false)
	if err := orders.Refund(ctx, ord, cajero, "flujo viejo"); err != nil {
		t.Fatalf("Refund: %v", err)
	}
	before := salidasDeCaja(t, st)
	err := orders.Devolver(ctx, app.DevolucionCmd{OrderID: ord, Monto: decimal.RequireFromString("220"), Motivo: "otra vez", ActorID: cajero})
	if !errors.Is(err, domain.ErrRefundOnRefundedOrder) {
		t.Fatalf("devolver un pedido reembolsado: err = %v, quiere ErrRefundOnRefundedOrder", err)
	}
	if !salidasDeCaja(t, st).Equal(before) {
		t.Fatal("salió dinero del cajón por un pedido que ya se había reembolsado")
	}
}

// D2: DEVOLVER UN PAGO YA DEVUELTO EN PARTE SE RECHAZA.
//
// $100 en efectivo, «Devolver» $40 y luego «Devolver pago» de los $100: salían $140 por un pedido
// de $100 y el pedido volvía a deber $100.
func TestVoidingAPaymentWithRefundsIsRejected(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_d2", "gerente")
	efectivo := paymentMethodID(t, st, "Efectivo")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")
	abrirCajaPrincipal(t, st, cajero)

	prod := makeProduct(t, st, "Pago d2", decimal.RequireFromString("100"), false)
	ord := crearPedidoSimple(t, ctx, orders, prod, cajero)
	pay := charge(t, ctx, orders, ord, efectivo, "100", cajero)
	refund(t, ctx, orders, ord, nil, "40", cajero)

	if _, err := orders.VoidPayment(ctx, ord, pay, cajero, "Se cobró de más"); !errors.Is(err, domain.ErrPaymentHasRefunds) {
		t.Fatalf("devolver un pago con $40 ya devueltos: err = %v, quiere ErrPaymentHasRefunds", err)
	}

	// Una devolución por OTRO medio no estorba: el pago en efectivo se puede devolver.
	ord2 := crearPedidoSimple(t, ctx, orders, prod, cajero)
	cash := charge(t, ctx, orders, ord2, efectivo, "50", cajero)
	charge(t, ctx, orders, ord2, tarjeta, "50", cajero)
	refund(t, ctx, orders, ord2, nil, "50", cajero) // sale primero del efectivo...
	if _, err := orders.VoidPayment(ctx, ord2, cash, cajero, "Se cobró de más"); !errors.Is(err, domain.ErrPaymentHasRefunds) {
		t.Fatalf("el efectivo ya devuelto: err = %v, quiere ErrPaymentHasRefunds", err)
	}
	ord3 := crearPedidoSimple(t, ctx, orders, prod, cajero)
	card := charge(t, ctx, orders, ord3, tarjeta, "50", cajero)
	cash3 := charge(t, ctx, orders, ord3, efectivo, "50", cajero)
	refund(t, ctx, orders, ord3, nil, "50", cajero) // ...aquí sale de la tarjeta, que entró primero
	if _, err := orders.VoidPayment(ctx, ord3, cash3, cajero, "Se cobró de más"); err != nil {
		t.Fatalf("devolver el pago en efectivo con la devolución por tarjeta: %v", err)
	}
	_ = card
}

// ---- helpers ----

func charge(t *testing.T, ctx context.Context, svc *app.OrdersService, order int64, method int16, amount string, actor int64) int64 {
	t.Helper()
	res, err := svc.Charge(ctx, app.ChargeCmd{OrderID: order, MethodID: method, Amount: decimal.RequireFromString(amount), ActorID: actor})
	if err != nil {
		t.Fatalf("Charge(%s): %v", amount, err)
	}
	return res.PaymentID
}

func refund(t *testing.T, ctx context.Context, svc *app.OrdersService, order int64, line *int64, amount string, actor int64) {
	t.Helper()
	if err := svc.Devolver(ctx, app.DevolucionCmd{OrderID: order, LineID: line, Monto: decimal.RequireFromString(amount),
		Motivo: "prueba", ActorID: actor}); err != nil {
		t.Fatalf("Devolver(%s): %v", amount, err)
	}
}

func refundLine(t *testing.T, ctx context.Context, svc *app.OrdersService, order, line int64, amount string, actor int64) {
	t.Helper()
	refund(t, ctx, svc, order, &line, amount, actor)
}

func refundedByMethod(t *testing.T, st *store.Store, order int64) map[int16]decimal.Decimal {
	t.Helper()
	rows, err := st.Pool.Query(context.Background(),
		`select payment_method_id, sum(amount) from order_refunds where order_id = $1 group by 1`, order)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int16]decimal.Decimal{}
	for rows.Next() {
		var m int16
		var a decimal.Decimal
		if err := rows.Scan(&m, &a); err != nil {
			t.Fatal(err)
		}
		out[m] = a
	}
	return out
}

func refundedTotal(t *testing.T, st *store.Store, order int64) decimal.Decimal {
	t.Helper()
	var total decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(),
		`select coalesce(sum(amount), 0) from order_refunds where order_id = $1`, order).Scan(&total); err != nil {
		t.Fatal(err)
	}
	return total
}

// twoLineOrder deja un pedido vivo de dos renglones (pasan por cocina) y devuelve sus ids en orden.
func twoLineOrder(t *testing.T, ctx context.Context, st *store.Store, svc *app.OrdersService, suffix, priceA, priceB string, actor int64) (int64, []int64) {
	t.Helper()
	a := makeProduct(t, st, "A "+suffix, decimal.RequireFromString(priceA), false)
	b := makeProduct(t, st, "B "+suffix, decimal.RequireFromString(priceB), false)
	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: actor,
		Lines: []domain.OrderLineInput{
			{ProductID: a, Qty: decimal.RequireFromString("1")},
			{ProductID: b, Qty: decimal.RequireFromString("1")},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	rows, err := st.Pool.Query(ctx, `select id from order_lines where order_id = $1 order by id`, ord.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ord.ID, ids
}

// concurrently corre n llamadas a la vez, soltándolas juntas, y devuelve sus errores.
func concurrently(n int, f func(i int) error) []error {
	errs := make([]error, n)
	var ready, done sync.WaitGroup
	start := make(chan struct{})
	ready.Add(n)
	done.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer done.Done()
			ready.Done()
			<-start
			errs[i] = f(i)
		}(i)
	}
	ready.Wait()
	close(start)
	done.Wait()
	return errs
}
