//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// SE DEVUELVE LO QUE ENTRÓ, NO EL PRECIO DE LISTA.
//
// `Refund` anotaba como pérdida `orders.total` sin mirar un solo cobro. Un pedido de $500 cobrado a
// medias registraba $500 de pérdida cuando solo habían entrado $300.
func TestSeDevuelveLoCobradoNoElTotalDelPedido(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordenes := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_dev_cobrado", "cajero")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	// Un pedido de $500 del que solo entraron $300.
	ord := pedidoCobradoParcial(t, ctx, st, ordenes, "dev_cobrado", "500", "300", cajero, efectivo, false)

	if err := ordenes.Devolver(ctx, app.DevolucionCmd{
		OrderID: ord, Monto: decimal.RequireFromString("500"),
		Motivo: "se equivocó el platillo", ActorID: cajero,
	}); !errors.Is(err, domain.ErrDevolucionExcede) {
		t.Fatalf("devolver 500 de un pedido con 300 cobrados: err = %v, quiere ErrDevolucionExcede", err)
	}

	// Lo que sí entró, se puede devolver.
	if err := ordenes.Devolver(ctx, app.DevolucionCmd{
		OrderID: ord, Monto: decimal.RequireFromString("300"),
		Motivo: "se equivocó el platillo", ActorID: cajero,
	}); err != nil {
		t.Fatalf("devolver lo cobrado: %v", err)
	}

	var devuelto decimal.Decimal
	if err := st.Pool.QueryRow(ctx,
		`select refund_amount from orders where id = $1`, ord).Scan(&devuelto); err != nil {
		t.Fatalf("leer refund_amount: %v", err)
	}
	if !devuelto.Equal(decimal.RequireFromString("300")) {
		t.Fatalf("refund_amount = %s, quiere 300: es lo que entró, no el total del pedido", devuelto)
	}
}

// UN PEDIDO SIN COBROS NO TIENE NADA QUE DEVOLVER.
//
// El tablero pinta "Cobrar $220" y "Reembolsar" pegados en la misma tarjeta. Tocar el segundo
// registraba $220 de pérdida por un ingreso que nunca ocurrió, y la cuenta por cobrar desaparecía
// del contador sin haberse cobrado.
func TestUnPedidoSinCobrarNoSeDevuelve(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordenes := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_dev_sincobrar", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	prod := makeProduct(t, st, "Sin cobrar dev", decimal.RequireFromString("220"), false)
	ord := crearPedidoSimple(t, ctx, ordenes, prod, cajero)

	err := ordenes.Devolver(ctx, app.DevolucionCmd{
		OrderID: ord, Monto: decimal.RequireFromString("220"), Motivo: "prueba", ActorID: cajero,
	})
	if !errors.Is(err, domain.ErrSinCobrosQueDevolver) {
		t.Fatalf("err = %v, quiere ErrSinCobrosQueDevolver", err)
	}

	var devuelto decimal.Decimal
	if err := st.Pool.QueryRow(ctx, `select refund_amount from orders where id = $1`, ord).Scan(&devuelto); err != nil {
		t.Fatalf("leer refund_amount: %v", err)
	}
	if !devuelto.IsZero() {
		t.Fatalf("se anotó una pérdida de %s por un ingreso que nunca ocurrió", devuelto)
	}
}

// LA DEVOLUCIÓN EN EFECTIVO SALE DEL CAJÓN; LA DE TARJETA NO.
//
// Ese dinero nunca estuvo en la caja: descontarlo del cajón inventaría un faltante que el cajero
// buscaría contando tres veces.
func TestSoloLaDevolucionEnEfectivoTocaElCajon(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordenes := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_dev_cajon", "cajero")
	efectivo := paymentMethodID(t, st, "Efectivo")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")
	abrirCajaPrincipal(t, st, cajero)

	enEfectivo := pedidoCobradoParcial(t, ctx, st, ordenes, "dev_cajon_efe", "100", "100", cajero, efectivo, false)
	conTarjeta := pedidoCobradoParcial(t, ctx, st, ordenes, "dev_cajon_tar", "100", "100", cajero, tarjeta, false)

	antes := salidasDeCaja(t, st)
	if err := ordenes.Devolver(ctx, app.DevolucionCmd{
		OrderID: conTarjeta, Monto: decimal.RequireFromString("100"), Motivo: "devuelta", ActorID: cajero,
	}); err != nil {
		t.Fatalf("devolver con tarjeta: %v", err)
	}
	if s := salidasDeCaja(t, st); !s.Equal(antes) {
		t.Fatalf("la devolución con tarjeta sacó %s del cajón: ese dinero nunca estuvo ahí", s.Sub(antes))
	}

	if err := ordenes.Devolver(ctx, app.DevolucionCmd{
		OrderID: enEfectivo, Monto: decimal.RequireFromString("100"), Motivo: "devuelta", ActorID: cajero,
	}); err != nil {
		t.Fatalf("devolver en efectivo: %v", err)
	}
	if s := salidasDeCaja(t, st); !s.Sub(antes).Equal(decimal.RequireFromString("100")) {
		t.Fatalf("del cajón salieron %s, quiere 100: el efectivo devuelto sí sale de la caja", s.Sub(antes))
	}
}

// CANCELAR UN PEDIDO YA COBRADO EXIGE RESOLVER EL DINERO.
//
// Cancelarlo a secas respondía 204: la venta salía de los reportes, los cobros se quedaban en la
// base y el arqueo SEGUÍA esperando ese dinero en el cajón. Devolverlo al cliente dejaba el corte
// con un faltante que ningún renglón explicaba.
func TestCancelarUnPedidoCobradoExigeLaDevolucion(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordenes := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_cancel_cobrado", "cajero")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)
	ord := pedidoCobradoParcial(t, ctx, st, ordenes, "cancel_cobrado", "275", "275", cajero, efectivo, true)

	// Sin devolución: se rechaza. Antes pasaba y dejaba el dinero sin clasificar.
	if err := ordenes.Cancel(ctx, ord, cajero, "el cliente se arrepintió"); err == nil {
		t.Fatal("cancelar un pedido con cobros SIN devolución debe rechazarse: el arqueo seguiría " +
			"esperando ese dinero en el cajón")
	}

	// Con devolución: pasa, y el cajón queda cuadrado.
	antes := salidasDeCaja(t, st)
	if err := ordenes.CancelarConDevolucion(ctx, app.CancelacionCmd{
		OrderID: ord, Motivo: "el cliente se arrepintió", ActorID: cajero, Devolver: true,
	}); err != nil {
		t.Fatalf("cancelar con devolución: %v", err)
	}
	if s := salidasDeCaja(t, st).Sub(antes); !s.Equal(decimal.RequireFromString("275")) {
		t.Fatalf("salieron %s del cajón, quiere 275: la cancelación tiene que devolver lo cobrado", s)
	}
}

// ---- helpers ----

// pedidoCobradoParcial deja un pedido cobrado por `cobrar` de un total de `precio`.
//
// `sigueEnCocina` decide si el pedido queda VIVO tras el cobro. Importa: un producto que no pasa por
// cocina cierra el pedido solo al quedar saldado, y un pedido entregado ya no se cancela —se
// devuelve—, que es justo la diferencia entre los dos casos que hay que probar.
func pedidoCobradoParcial(t *testing.T, ctx context.Context, st *store.Store, svc *app.OrdersService,
	sufijo, precio, cobrar string, cajero int64, metodo int16, sigueEnCocina bool,
) int64 {
	t.Helper()
	prod := makeProduct(t, st, "Devolucion "+sufijo, decimal.RequireFromString(precio), false)
	if !sigueEnCocina {
		if _, err := st.Pool.Exec(ctx, "update products set needs_prep = false where id = $1", prod); err != nil {
			t.Fatalf("quitar needs_prep: %v", err)
		}
	}
	ord := crearPedidoSimple(t, ctx, svc, prod, cajero)
	if _, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord, MethodID: metodo, Amount: decimal.RequireFromString(cobrar), ActorID: cajero,
	}); err != nil {
		t.Fatalf("Charge(%s): %v", sufijo, err)
	}
	return ord
}

func salidasDeCaja(t *testing.T, st *store.Store) decimal.Decimal {
	t.Helper()
	var total decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(),
		`select coalesce(sum(amount), 0) from register_cash_movements where kind = 'salida'`).Scan(&total); err != nil {
		t.Fatalf("sumar salidas de caja: %v", err)
	}
	return total
}

// crearPedidoSimple deja un pedido de un renglón, sin cobrar.
func crearPedidoSimple(t *testing.T, ctx context.Context, svc *app.OrdersService, prod, cajero int64) int64 {
	t.Helper()
	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return ord.ID
}

// EL INSUMO VUELVE SOLO SI LA COMIDA NO SE HIZO.
//
// Cancelar un renglón no existía: la columna estaba y ninguna consulta la escribía, mientras el
// error de cancelar un pedido con entregas parciales mandaba al operador a "cancela los que falten".
// La única salida practicable era marcar como entregado lo que seguía en la plancha.
func TestCancelarUnRenglonReponeSoloSiNoSalioACocina(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordenes := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_renglon", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	prod := makeProduct(t, st, "Renglon cancelable", decimal.RequireFromString("80"), true)
	ord := crearPedidoSimple(t, ctx, ordenes, prod, cajero)

	linea, enviado := primerRenglon(t, st, ord)
	if enviado {
		// El pedido nace con sus renglones ya en cocina: se desmarca para probar el camino de "no
		// salió", que es el que repone.
		if _, err := st.Pool.Exec(ctx,
			`update order_lines set enviado_a_cocina_at = null where id = $1`, linea); err != nil {
			t.Fatalf("desmarcar: %v", err)
		}
	}

	antes := existencias(t, st, prod)
	repuso, err := ordenes.CancelarRenglon(ctx, ord, linea, cajero, "el cliente lo quitó")
	if err != nil {
		t.Fatalf("cancelar renglón: %v", err)
	}
	if !repuso {
		t.Fatal("un renglón que no salió a cocina debe reponer: la comida no se hizo")
	}
	if e := existencias(t, st, prod); !e.GreaterThan(antes) {
		t.Fatalf("las existencias pasaron de %s a %s: el insumo no volvió", antes, e)
	}

	// Y el total del pedido baja, porque el cliente no paga lo que se canceló.
	var total decimal.Decimal
	if err := st.Pool.QueryRow(ctx, `select total from orders where id = $1`, ord).Scan(&total); err != nil {
		t.Fatalf("leer total: %v", err)
	}
	if !total.IsZero() {
		t.Fatalf("el total quedó en %s tras cancelar su único renglón, quiere 0", total)
	}
}

// El que YA salió a cocina baja el total igual, pero NO repone: ese insumo se consumió, y reponerlo
// inventaría existencias que no están.
func TestUnRenglonQueYaSalioACocinaNoRepone(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordenes := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_renglon_cocina", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	prod := makeProduct(t, st, "Renglon en plancha", decimal.RequireFromString("80"), true)
	ord := crearPedidoSimple(t, ctx, ordenes, prod, cajero)
	linea, _ := primerRenglon(t, st, ord)

	antes := existencias(t, st, prod)
	repuso, err := ordenes.CancelarRenglon(ctx, ord, linea, cajero, "se quemó")
	if err != nil {
		t.Fatalf("cancelar renglón: %v", err)
	}
	if repuso {
		t.Fatal("un renglón que ya salió a cocina NO debe reponer: el insumo se consumió")
	}
	if e := existencias(t, st, prod); !e.Equal(antes) {
		t.Fatalf("las existencias pasaron de %s a %s: se inventó inventario que se consumió", antes, e)
	}
}

// Un doble tap no puede reponer dos veces el mismo insumo: eso es inventar existencias.
func TestCancelarDosVecesElMismoRenglonNoReponeDosVeces(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordenes := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_renglon_doble", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	prod := makeProduct(t, st, "Renglon doble tap", decimal.RequireFromString("80"), true)
	ord := crearPedidoSimple(t, ctx, ordenes, prod, cajero)
	linea, _ := primerRenglon(t, st, ord)
	if _, err := st.Pool.Exec(ctx, `update order_lines set enviado_a_cocina_at = null where id = $1`, linea); err != nil {
		t.Fatalf("desmarcar: %v", err)
	}

	if _, err := ordenes.CancelarRenglon(ctx, ord, linea, cajero, "el cliente lo quitó"); err != nil {
		t.Fatalf("primer toque: %v", err)
	}
	tras1 := existencias(t, st, prod)
	if _, err := ordenes.CancelarRenglon(ctx, ord, linea, cajero, "el cliente lo quitó"); err != nil {
		t.Fatalf("segundo toque: %v — un doble tap no puede dar error sobre algo ya cancelado", err)
	}
	if e := existencias(t, st, prod); !e.Equal(tras1) {
		t.Fatalf("las existencias pasaron de %s a %s en el segundo toque: se repuso dos veces", tras1, e)
	}
}

func primerRenglon(t *testing.T, st *store.Store, orderID int64) (int64, bool) {
	t.Helper()
	var id int64
	var enviado *time.Time
	if err := st.Pool.QueryRow(context.Background(),
		`select id, enviado_a_cocina_at from order_lines where order_id = $1 order by id limit 1`,
		orderID).Scan(&id, &enviado); err != nil {
		t.Fatalf("leer renglón: %v", err)
	}
	return id, enviado != nil
}

func existencias(t *testing.T, st *store.Store, productID int64) decimal.Decimal {
	t.Helper()
	var v decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(),
		`select coalesce(sum(quantity), 0) from stock_movements where product_id = $1`,
		productID).Scan(&v); err != nil {
		t.Fatalf("sumar movimientos: %v", err)
	}
	return v
}

// EL REPORTE DE DEVOLUCIONES Y LO QUE SALIÓ DEL CAJÓN CUADRAN (SC-002).
//
// Son dos cifras del mismo dinero: `RefundsByDay` lee `orders.refund_amount` y el arqueo suma los
// movimientos de salida. Si divergen, una de las dos miente y quien las lee no tiene forma de saber
// cuál — el corolario del principio III.
//
// No se exige igualdad con TODO lo devuelto: solo el efectivo sale del cajón. Lo que tiene que
// cuadrar es la parte en efectivo.
func TestElReporteDeDevolucionesCuadraConLoQueSalioDelCajon(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordenes := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_cuadre", "cajero")
	efectivo := paymentMethodID(t, st, "Efectivo")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")
	abrirCajaPrincipal(t, st, cajero)

	enEfectivo := pedidoCobradoParcial(t, ctx, st, ordenes, "cuadre_efe", "300", "300", cajero, efectivo, false)
	conTarjeta := pedidoCobradoParcial(t, ctx, st, ordenes, "cuadre_tar", "200", "200", cajero, tarjeta, false)

	antes := salidasDeCaja(t, st)
	for _, o := range []int64{enEfectivo, conTarjeta} {
		monto, err := ordenes.PorDevolver(ctx, o, nil)
		if err != nil {
			t.Fatalf("PorDevolver: %v", err)
		}
		if err := ordenes.Devolver(ctx, app.DevolucionCmd{
			OrderID: o, Monto: monto, Motivo: "cuadre", ActorID: cajero,
		}); err != nil {
			t.Fatalf("devolver: %v", err)
		}
	}

	// Lo que el libro dice que salió EN EFECTIVO.
	var enLibro decimal.Decimal
	if err := st.Pool.QueryRow(ctx, `
		select coalesce(sum(r.amount), 0) from order_refunds r
		 where r.cash_movement_id is not null`).Scan(&enLibro); err != nil {
		t.Fatalf("sumar el libro: %v", err)
	}
	salio := salidasDeCaja(t, st).Sub(antes)
	if !enLibro.Equal(salio) {
		t.Fatalf("el libro dice %s de devoluciones en efectivo y del cajón salieron %s: dos cifras "+
			"del mismo dinero que no cuadran", enLibro, salio)
	}
	if !salio.Equal(decimal.RequireFromString("300")) {
		t.Fatalf("del cajón salieron %s, quiere 300: la de tarjeta no sale de la caja", salio)
	}

	// Y el reporte de devoluciones cuenta las DOS, porque las dos son ingreso que no ocurrió.
	var reportado decimal.Decimal
	if err := st.Pool.QueryRow(ctx,
		`select coalesce(sum(refund_amount), 0) from orders where id in ($1, $2)`,
		enEfectivo, conTarjeta).Scan(&reportado); err != nil {
		t.Fatalf("leer refund_amount: %v", err)
	}
	if !reportado.Equal(decimal.RequireFromString("500")) {
		t.Fatalf("el reporte dice %s, quiere 500: es lo devuelto por los dos métodos", reportado)
	}
}

// EL MENSAJE DE ERROR YA NO MANDA A UNA ACCIÓN QUE NO EXISTE (FR-008).
//
// `ErrCancelarConEntregas` dice "cancela los que falten o haz un reembolso". Cancelar un renglón NO
// existía: ninguna consulta escribía `order_lines.cancelled_at`, así que la única salida practicable
// era marcar como entregado lo que seguía en la plancha. Ahora la frase es cierta, y esto lo prueba
// haciendo lo que el mensaje dice.
func TestLoQueElErrorDeEntregaParcialDiceSePuedeHacer(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, alitas, _ := pedidoDeAlitas(t, st, svc, "mensaje_cierto")
	cajero := makeUser(t, st, "cajero_mensaje", "cajero")

	// Se entrega UN renglón: el pedido entra en entrega parcial.
	var lineaAlitas, lineaPapas int64
	if err := st.Pool.QueryRow(ctx,
		`select id from order_lines where order_id = $1 and product_id = $2`,
		ord.ID, alitas).Scan(&lineaAlitas); err != nil {
		t.Fatalf("leer renglón de alitas: %v", err)
	}
	if err := st.Pool.QueryRow(ctx,
		`select id from order_lines where order_id = $1 and id <> $2 limit 1`,
		ord.ID, lineaAlitas).Scan(&lineaPapas); err != nil {
		t.Fatalf("leer el otro renglón: %v", err)
	}
	if err := svc.DeliverLine(ctx, ord.ID, lineaAlitas, decimal.RequireFromString("5")); err != nil {
		t.Fatalf("entregar alitas: %v", err)
	}

	// El pedido entero no se cancela, y el error lo dice.
	if err := svc.Cancel(ctx, ord.ID, cajero, "prueba"); !errors.Is(err, domain.ErrCancelarConEntregas) {
		t.Fatalf("cancelar con entrega parcial = %v, quiere ErrCancelarConEntregas", err)
	}

	// Y lo que el mensaje manda a hacer, se puede hacer: cancelar el que falta.
	if _, err := svc.CancelarRenglon(ctx, ord.ID, lineaPapas, cajero, "ya no lo quiere"); err != nil {
		t.Fatalf("cancelar el renglón que falta: %v — el error manda a una acción que no funciona", err)
	}
}
