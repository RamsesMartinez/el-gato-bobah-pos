//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// pedidoDe deja un pedido confirmado y SIN cobrar por el monto pedido, listo para dividirlo.
func pedidoDe(t *testing.T, st *store.Store, svc *app.OrdersService, sufijo, monto string) (*app.OrderView, int64, int16) {
	t.Helper()
	return pedidoDeProducto(t, st, svc, sufijo, monto, true)
}

// pedidoDeProducto deja el pedido con un producto que pasa —o no— por cocina. Lo que NO pasa por
// cocina se cierra solo al quedar saldado (una embotellada del mostrador); lo que sí, se queda en la
// barra esperando a que salga de la plancha, y esa diferencia decide qué predicado se ejercita.
func pedidoDeProducto(t *testing.T, st *store.Store, svc *app.OrdersService, sufijo, monto string, pasaPorCocina bool) (*app.OrderView, int64, int16) {
	t.Helper()
	cajero := makeUser(t, st, "cajero_"+sufijo, "cajero")
	prod := makeProduct(t, st, "Combo "+sufijo, decimal.RequireFromString(monto), false)
	if !pasaPorCocina {
		if _, err := st.Pool.Exec(context.Background(),
			`update products set needs_prep = false where id = $1`, prod); err != nil {
			t.Fatalf("quitar needs_prep: %v", err)
		}
	}
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord, err := svc.Create(context.Background(), app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return ord, cajero, efectivo
}

// EL DEFECTO: al dividir la cuenta desaparece la red que protegía al cobro completo.
//
// Con un solo pago, el doble tap lo atrapa `falta = 0`: el segundo intento rebota con
// ErrPedidoYaPagado. Al dividir, las dos mitades son INDISTINGUIBLES entre sí, así que el segundo
// envío de la primera mitad pasa todas las validaciones —cabe en lo que falta— y deja el pedido
// marcado como saldado. La tarjeta del segundo comensal nunca se cobra y nadie se entera: el
// arqueo cuadra contra los pagos que sí se registraron, y lo que se pierde es dinero que jamás
// entró, sin un renglón que lo nombre.
//
// Medido antes del arreglo: pedido de $500, dos llamadas idénticas de $250 + $50 de propina →
// pagado $500, propina $100, dos filas, pedido cerrado.
func TestUnDobleTapNoCobraDosVecesLaMismaMitad(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero, efectivo := pedidoDe(t, st, svc, "doble_tap", "500")

	mitad := app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo,
		Amount: decimal.RequireFromString("250"), Tip: decimal.RequireFromString("50"),
		ActorID: cajero, ClientUUID: uuid.New(),
	}
	if _, err := svc.Charge(ctx, mitad); err != nil {
		t.Fatalf("primer cobro: %v", err)
	}
	// El MISMO cobro otra vez: la tableta no pintó la respuesta y el operador volvió a tocar.
	if _, err := svc.Charge(ctx, mitad); err != nil {
		t.Fatalf("el reintento del mismo cobro debe ser inocuo, no un error: %v", err)
	}

	tras, err := svc.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if tras.Paid {
		t.Fatal("el doble tap sobre la PRIMERA mitad dejó el pedido saldado: " +
			"el segundo comensal ya no puede pagar y su dinero nunca entró")
	}
	pagado, propina := sumaDePagos(t, st, ord.ID)
	if !pagado.Equal(decimal.RequireFromString("250")) {
		t.Fatalf("se cobró %s y solo debió cobrarse una mitad de 250", pagado)
	}
	if !propina.Equal(decimal.RequireFromString("50")) {
		t.Fatalf("la propina se registró %s veces: el reintento la duplicó (esperaba 50, obtuve %s)",
			propina.Div(decimal.RequireFromString("50")), propina)
	}
}

// EL DEFECTO: la propina no tiene tope contra el pedido, solo contra ValidMoney (10 millones).
//
// Medido: un pedido de $250 aceptó `amount=100, tip=9999`. Esa propina entra al esperado del cajón
// (ExpectedByMethodForSession suma tip_amount) y a TipsByEmployee, así que un dedo gordo cierra el
// turno con un faltante de $9,999 que nadie sabe explicar. El tope es el total del pedido: una
// propina mayor que la cuenta entera es un error de captura mucho más seguido que un regalo, y
// cuando de verdad es un regalo se parte en dos cobros.
func TestLaPropinaNoPuedeSuperarLaCuenta(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero, efectivo := pedidoDe(t, st, svc, "propina_gorda", "250")

	_, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo,
		Amount: decimal.RequireFromString("100"), Tip: decimal.RequireFromString("9999"),
		ActorID: cajero, ClientUUID: uuid.New(),
	})
	if !errors.Is(err, domain.ErrPropinaExcede) {
		t.Fatalf("una propina de $9,999 sobre una cuenta de $250 se aceptó (err=%v); "+
			"el cajón la espera y el turno cierra con ese faltante", err)
	}
}

// EL DEFECTO: dos predicados distintos sobre la misma cifra.
//
// Dividir $100 en tres partes de $33.33 suma $99.99. `PagosCubren` tolera el centavo y CIERRA el
// pedido, pero `PorCobrar` es exacto y lo sigue reportando con $0.01 de deuda: el tablero suma ese
// centavo, y al día siguiente el pedido desaparece de la vista con la deuda abierta. Es el
// corolario del principio III — la lista y el resumen de la misma pantalla salen del mismo
// predicado — con el redondeo de por medio.
//
// Quien cierra el pedido es quien debe saldarlo: si el cobro alcanza para cerrarlo, no queda nada
// por cobrar.
func TestUnPedidoCerradoNoDejaCentavosDeDeuda(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	// Tres refrescos del mostrador: no pasan por cocina, así que el pedido se cierra solo al quedar
	// saldado. Es el caso donde el centavo se nota, porque el pedido ya no tiene por qué seguir en la
	// barra y ahí sigue.
	ord, cajero, efectivo := pedidoDeProducto(t, st, svc, "centavo", "100", false)

	tercio := decimal.RequireFromString("33.33")
	for i := 0; i < 3; i++ {
		if _, err := svc.Charge(ctx, app.ChargeCmd{
			OrderID: ord.ID, MethodID: efectivo, Amount: tercio,
			ActorID: cajero, ClientUUID: uuid.New(),
		}); err != nil {
			t.Fatalf("cobro %d de 3: %v", i+1, err)
		}
	}

	tras, err := svc.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if !tras.Paid {
		t.Fatal("tres tercios de $33.33 no saldaron un pedido de $100")
	}
	// El tablero lee `outstanding` de ListOpenOrders, no `Paid`: es la cifra que ve el operador.
	for _, o := range abiertosDelTablero(t, svc) {
		if o.ID == ord.ID {
			t.Fatalf("el pedido quedó saldado y cerrado pero la barra del POS lo sigue listando "+
				"(outstanding=%s): cierra con un predicado tolerante y decide qué mostrar con uno exacto",
				o.Outstanding)
		}
	}
}

// sumaDePagos lee order_payments directo: es la tabla que alimenta el arqueo, y lo que importa de
// estos defectos es cuántas filas quedaron ahí, no lo que la vista derive de ellas.
func sumaDePagos(t *testing.T, st *store.Store, orderID int64) (decimal.Decimal, decimal.Decimal) {
	t.Helper()
	var pagado, propina decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(),
		`select coalesce(sum(amount), 0), coalesce(sum(tip_amount), 0)
		   from order_payments where order_id = $1`, orderID).Scan(&pagado, &propina); err != nil {
		t.Fatalf("sumaDePagos: %v", err)
	}
	return pagado, propina
}

// abiertosDelTablero es lo que ve el operador en la barra del POS: la lista de la que salen la
// píldora y su total, no el campo Paid del detalle.
func abiertosDelTablero(t *testing.T, svc *app.OrdersService) []app.BoardOrder {
	t.Helper()
	items, _, err := svc.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return items
}

// EL DEFECTO QUE ESTO PREVIENE: tomar por reintento un cobro mal dirigido.
//
// La llave identifica un cobro, no un pedido. Si la misma llegara sobre OTRO pedido y se tratara
// como no-op, ese cobro real nunca entraría y el segundo pedido quedaría con un saldo que el
// operador ya cobró. Se rechaza en vez de aceptarse en silencio: quien manda la llave repetida está
// equivocado, y el error se lo dice.
func TestLaMismaLlaveEnOtroPedidoNoSeTomaComoReintento(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	uno, cajero, efectivo := pedidoDe(t, st, svc, "llave_uno", "100")

	prod := makeProduct(t, st, "Otro combo", decimal.RequireFromString("100"), false)
	dos, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("segundo pedido: %v", err)
	}

	llave := uuid.New()
	if _, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: uno.ID, MethodID: efectivo, Amount: decimal.RequireFromString("100"),
		ActorID: cajero, ClientUUID: llave,
	}); err != nil {
		t.Fatalf("cobro del primer pedido: %v", err)
	}

	_, err = svc.Charge(ctx, app.ChargeCmd{
		OrderID: dos.ID, MethodID: efectivo, Amount: decimal.RequireFromString("100"),
		ActorID: cajero, ClientUUID: llave,
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("la misma llave sobre otro pedido = %v, quiere ErrConflict; "+
			"darla por reintento dejaría ese cobro sin registrar", err)
	}
	pagado, _ := sumaDePagos(t, st, dos.ID)
	if !pagado.IsZero() {
		t.Fatalf("el segundo pedido registró %s: la llave repetida no debe cobrar nada", pagado)
	}
}

// EL DEFECTO: el detalle del pedido decía solo `paid: bool`, así que la hoja de cobro tenía que
// restar por su cuenta para saber cuánto faltaba — una vez por comensal en una cuenta dividida. Dos
// implementaciones de la misma cifra son las que dejaron a la barra del POS diciendo $2,141
// mientras su propia lista decía $1,928.
func TestElDetalleDelPedidoDiceCuantoFalta(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero, efectivo := pedidoDe(t, st, svc, "cuanto_falta", "500")

	sinCobrar, err := svc.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if !sinCobrar.Outstanding.Equal(decimal.RequireFromString("500")) {
		t.Fatalf("sin cobrar nada falta %s, quiere 500", sinCobrar.Outstanding)
	}

	if _, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: decimal.RequireFromString("200"),
		ActorID: cajero, ClientUUID: uuid.New(),
	}); err != nil {
		t.Fatalf("abono: %v", err)
	}

	medio, err := svc.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if !medio.Outstanding.Equal(decimal.RequireFromString("300")) {
		t.Fatalf("tras abonar 200 de 500 falta %s, quiere 300", medio.Outstanding)
	}
	if medio.Paid {
		t.Fatal("con 200 de 500 el detalle dice que ya está pagado")
	}

	// Y la cifra que devuelve el propio cobro es la misma que la del detalle: si divergen, la hoja
	// pinta una entre pago y pago y otra al recargar.
	res, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: decimal.RequireFromString("300"),
		ActorID: cajero, ClientUUID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("completar: %v", err)
	}
	final, err := svc.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if !res.Outstanding.Equal(final.Outstanding) || res.Paid != final.Paid {
		t.Fatalf("el cobro devolvió (falta %s, pagado %v) y el detalle dice (falta %s, pagado %v)",
			res.Outstanding, res.Paid, final.Outstanding, final.Paid)
	}
	if !res.Outstanding.IsZero() {
		t.Fatalf("saldado el pedido sigue faltando %s", res.Outstanding)
	}
}

// La propina se topa POR PAGO, no acumulada, y este test fija la decisión con su porqué.
//
// El tope existe para atrapar el dedo gordo, y un dedo gordo es UNA cifra absurda. Acumulando, el
// caso cotidiano de una cuenta chica con propina generosa rebota: $60 que pagan dos amigos y cada
// uno le deja $40 al repartidor son $80 de propina sobre $60 de cuenta. El segundo cobro fallaría
// con el dinero del cliente ya en la mano — justo lo que un tope de cordura no debe provocar.
func TestDosPropinasPlausiblesNoSeBloqueanEntreEllas(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero, efectivo := pedidoDe(t, st, svc, "propina_repartida", "60")

	for i := 0; i < 2; i++ {
		if _, err := svc.Charge(ctx, app.ChargeCmd{
			OrderID: ord.ID, MethodID: efectivo,
			Amount: decimal.RequireFromString("30"), Tip: decimal.RequireFromString("40"),
			ActorID: cajero, ClientUUID: uuid.New(),
		}); err != nil {
			t.Fatalf("cobro %d: %v; el tope de propina no puede bloquear a dos clientes generosos", i+1, err)
		}
	}
	_, propina := sumaDePagos(t, st, ord.ID)
	if !propina.Equal(decimal.RequireFromString("80")) {
		t.Fatalf("propina registrada %s, quiere 80", propina)
	}
}

// EL DEFECTO QUE ESTO CIERRA: un no-op solo es inocuo si la llamada es IDÉNTICA.
//
// El pago entra, la respuesta se pierde en la red, y el operador —"la terminal no jaló, me paga en
// efectivo"— cambia el método del renglón y vuelve a tocar. Comparando solo la llave, el servidor lo
// da por reintento: la pantalla canta cobrado, el operador mete los billetes al cajón, y el corte
// cierra esperando la tarjeta que nunca llegó sin esperar el efectivo que sí está. Descuadre por el
// mismo monto en los dos métodos a la vez, y firmado en register_session_totals.
func TestLaMismaLlaveConOtroMetodoNoPasaPorReintento(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero, efectivo := pedidoDe(t, st, svc, "llave_sellada", "250")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")

	llave := uuid.New()
	if _, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: tarjeta, Amount: decimal.RequireFromString("250"),
		ActorID: cajero, ClientUUID: llave,
	}); err != nil {
		t.Fatalf("cobro con tarjeta: %v", err)
	}

	// El mismo renglón, ahora en efectivo.
	_, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: decimal.RequireFromString("250"),
		ActorID: cajero, ClientUUID: llave,
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("la misma llave con OTRO método = %v, quiere ErrConflict; "+
			"darla por reintento deja el cajón descuadrado en los dos métodos", err)
	}

	// Y con otro monto, lo mismo: el operador corrigió la cifra antes de reintentar.
	_, err = svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: tarjeta, Amount: decimal.RequireFromString("200"),
		ActorID: cajero, ClientUUID: llave,
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("la misma llave con OTRO monto = %v, quiere ErrConflict", err)
	}

	// Y con otra propina: son $50 del cliente que nunca se registrarían.
	_, err = svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: tarjeta, Amount: decimal.RequireFromString("250"),
		Tip: decimal.RequireFromString("50"), ActorID: cajero, ClientUUID: llave,
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("la misma llave con propina distinta = %v, quiere ErrConflict", err)
	}
}

// EL DEFECTO: un cobro que YA entró tiene que poder reconocerse aunque entretanto se haya cerrado
// la caja.
//
// La sesión se leía antes que la llave, así que el reintento de un pago registrado contestaba "no
// hay caja abierta". El operador concluye que no entró, borra el renglón y lo rehace con llave
// nueva — que es el cobro doble que la llave existe para impedir.
func TestUnCobroYaRegistradoSeReconoceConLaCajaCerrada(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero, efectivo := pedidoDe(t, st, svc, "caja_cerrada", "250")

	llave := uuid.New()
	if _, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: decimal.RequireFromString("100"),
		ActorID: cajero, ClientUUID: llave,
	}); err != nil {
		t.Fatalf("cobro: %v", err)
	}
	if _, err := st.Pool.Exec(ctx,
		`update register_sessions set status = 'cerrada', closed_at = now() where status = 'abierta'`); err != nil {
		t.Fatalf("cerrar la caja: %v", err)
	}

	res, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: decimal.RequireFromString("100"),
		ActorID: cajero, ClientUUID: llave,
	})
	if err != nil {
		t.Fatalf("el reintento de un pago ya registrado devolvió %v; con la caja cerrada tiene que "+
			"seguir reconociéndose, o el operador lo rehace y cobra dos veces", err)
	}
	if !res.YaEstaba {
		t.Fatal("el reintento no vino marcado como ya registrado: la pantalla cantaría un cobro que no ocurrió")
	}
	pagado, _ := sumaDePagos(t, st, ord.ID)
	if !pagado.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("se cobró %s: el reintento con la caja cerrada registró un pago nuevo", pagado)
	}
}

// Y con la caja cerrada un cobro NUEVO sí se rechaza: la excepción es solo para lo ya registrado.
func TestSinCajaAbiertaNoSeCobraNadaNuevo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero, efectivo := pedidoDe(t, st, svc, "sin_caja", "250")

	if _, err := st.Pool.Exec(ctx,
		`update register_sessions set status = 'cerrada', closed_at = now() where status = 'abierta'`); err != nil {
		t.Fatalf("cerrar la caja: %v", err)
	}
	_, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: decimal.RequireFromString("250"),
		ActorID: cajero, ClientUUID: uuid.New(),
	})
	if !errors.Is(err, domain.ErrNoOpenRegister) {
		t.Fatalf("cobrar sin caja abierta = %v, quiere ErrNoOpenRegister", err)
	}
}

// EL DEFECTO: un método de pago desactivado seguía cobrando.
//
// GetPaymentMethod no filtraba `is_active`, así que la única barrera era que el front no lo listara
// — y una tableta encendida lleva horas con el catálogo en caché. El negocio apaga un método
// justamente para dejar de recibir por ahí; que el servidor lo siga aceptando manda ese dinero a un
// renglón del corte que nadie está contando.
func TestUnMetodoDesactivadoNoCobra(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero, efectivo := pedidoDe(t, st, svc, "metodo_apagado", "250")

	if _, err := st.Pool.Exec(ctx,
		`update payment_methods set is_active = false where id = $1`, efectivo); err != nil {
		t.Fatalf("desactivar el método: %v", err)
	}

	_, err := svc.Charge(ctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: decimal.RequireFromString("250"),
		ActorID: cajero, ClientUUID: uuid.New(),
	})
	if !errors.Is(err, domain.ErrMetodoInactivo) {
		t.Fatalf("cobrar con un método desactivado = %v, quiere ErrMetodoInactivo", err)
	}
}
