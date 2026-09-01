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
)

// La caja no se cierra con pedidos sin terminar.
//
// Es la regla que sostiene al corte: un pedido abierto o listo es comida que salió o va a salir y
// dinero que todavía no se decidió. Si el turno cierra con esos pendientes, el pedido se queda
// colgado del turno viejo, su venta cae en un arqueo que ya se firmó, y cuando alguien lo entregue
// al día siguiente el corte de ayer ya no puede cuadrar con nada.
//
// Cerrar es también el momento en que el operador SÍ puede resolverlos: está frente a la caja y el
// local está vacío. Media hora después, no.
func TestLaCajaNoCierraConPedidosSinTerminar(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	ordersSvc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_cierre", "cajero")
	prod := makeProduct(t, st, "Café cierre", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	principal := registerID(t, st, "Caja principal")

	if _, err := backoffice.OpenSession(ctx, principal, decimal.Zero, cajero); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	pedido, err := crearYCobrar(t, ctx, ordersSvc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("50")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	declarado := map[int]decimal.Decimal{int(efectivo): decimal.RequireFromString("50")}

	// Abierta: no cierra.
	if _, err := backoffice.CloseSession(ctx, principal, cajero, declarado, ""); !errors.Is(err, domain.ErrOpenOrders) {
		t.Fatalf("con un pedido abierto no debe cerrar, fue: %v", err)
	}

	// Lista tampoco: sigue sin entregarse.
	if err := ordersSvc.SetStatus(ctx, pedido.ID, domain.StatusLista); err != nil {
		t.Fatalf("marcar lista: %v", err)
	}
	if _, err := backoffice.CloseSession(ctx, principal, cajero, declarado, ""); !errors.Is(err, domain.ErrOpenOrders) {
		t.Fatalf("con un pedido listo no debe cerrar, fue: %v", err)
	}

	// El mensaje tiene que decir CUÁLES, o el operador queda con un error que no puede accionar.
	// Y tiene que decirlo con el NOMBRE del pedido: es lo que se lee en el tablero y lo que se
	// canta, así que un error con solo el número manda a comparar números contra una pantalla que
	// muestra animales, justo cuando se está cerrando y con prisa.
	_, err = backoffice.CloseSession(ctx, principal, cajero, declarado, "")
	msg := err.Error()
	if !contiene(msg, "#") {
		t.Fatalf("el error debe nombrar los folios pendientes, dijo: %s", msg)
	}
	if pedido.FolioName == "" {
		t.Fatal("el pedido nació sin nombre de folio")
	}
	if !contiene(msg, pedido.FolioName) {
		t.Fatalf("el error debe nombrar el pedido como %q, dijo: %s", pedido.FolioName, msg)
	}

	// Entregada: ya cierra.
	if err := ordersSvc.SetStatus(ctx, pedido.ID, domain.StatusEntregada); err != nil {
		t.Fatalf("entregar: %v", err)
	}
	if _, err := backoffice.CloseSession(ctx, principal, cajero, declarado, ""); err != nil {
		t.Fatalf("sin pendientes debe cerrar: %v", err)
	}
}

// Un pedido cancelado no bloquea: no hay nada que entregar ni que cobrar. Si bloqueara, el operador
// tendría que "terminar" algo que ya no existe y el único camino sería dejar la caja abierta.
func TestUnPedidoCanceladoNoBloqueaElCierre(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	ordersSvc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_cancel_cierre", "cajero")
	prod := makeProduct(t, st, "Café cancelado", decimal.RequireFromString("50"), false)
	principal := registerID(t, st, "Caja principal")

	if _, err := backoffice.OpenSession(ctx, principal, decimal.Zero, cajero); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	pedido, err := crearYCobrar(t, ctx, ordersSvc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := ordersSvc.Cancel(ctx, pedido.ID, cajero, "prueba"); err != nil {
		t.Fatalf("cancelar: %v", err)
	}

	if _, err := backoffice.CloseSession(ctx, principal, cajero, map[int]decimal.Decimal{}, ""); err != nil {
		t.Fatalf("un pedido cancelado no debe bloquear el cierre: %v", err)
	}
}

func contiene(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// El arqueo tiene que MOSTRAR lo que falta por entregar, no solo rebotar al presionar cerrar: el
// operador terminaba de contar el efectivo para enterarse entonces de que le faltaba sacar comida.
//
// Y la lista tiene que salir del mismo predicado que la guardia. Si se derivaran por separado, la
// pantalla podría decir "todo listo" mientras el botón rebota, y quien lo lee no tendría cómo
// saber cuál de las dos miente.
func TestElArqueoMuestraLoQueFaltaPorEntregar(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordersSvc := app.NewOrdersService(st, clock)
	backoffice := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_arqueo", "cajero")
	prod := makeProduct(t, st, "Café arqueo", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	principal := registerID(t, st, "Caja principal")
	abrirCajaPrincipal(t, st, cajero)

	// COBRADO pero sin entregar: aparece igual. Cobrado y entregado son cosas distintas, y lo que
	// impide cerrar es la comida que no ha salido.
	pedido, err := crearYCobrar(t, ctx, ordersSvc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("50")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	vista, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if len(vista.Pending) != 1 {
		t.Fatalf("el arqueo lista %d pendientes, quiere 1 (el pedido está cobrado pero no entregado)", len(vista.Pending))
	}
	if vista.Pending[0].Number != pedido.Number {
		t.Errorf("pendiente #%d, quiere #%d", vista.Pending[0].Number, pedido.Number)
	}
	if vista.Pending[0].Name != pedido.FolioName {
		t.Errorf("pendiente %q, quiere %q", vista.Pending[0].Name, pedido.FolioName)
	}

	// Al entregarlo, la lista se vacía Y el cierre deja de rebotar: las dos cosas se mueven juntas
	// porque salen del mismo predicado.
	if err := ordersSvc.DeliverAll(ctx, pedido.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}
	tras, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("Session tras entregar: %v", err)
	}
	if len(tras.Pending) != 0 {
		t.Fatalf("tras entregar quedan %d pendientes", len(tras.Pending))
	}
	declarado := map[int]decimal.Decimal{int(efectivo): decimal.RequireFromString("50")}
	if _, err := backoffice.CloseSession(ctx, principal, cajero, declarado, ""); err != nil {
		t.Fatalf("con la lista vacía el cierre debe pasar, fue: %v", err)
	}
}

// Dos personas cobrando contra el MISMO cajón: el arqueo tiene que decir cuánto cobró cada una.
//
// Es la alternativa a partir la caja en dos cuando hay dos estaciones. Dos arqueos contando el
// mismo dinero físico serían dos cifras inventadas; lo que sí se puede separar es quién cobró, y
// ese dato ya existía en received_by.
func TestElArqueoSeparaLoCobradoPorCadaPersona(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordersSvc := app.NewOrdersService(st, clock)
	backoffice := app.NewBackofficeService(st, clock)

	ana := makeUser(t, st, "ana_caja", "cajero")
	luis := makeUser(t, st, "luis_caja", "cajero")
	prod := makeProduct(t, st, "Café dos cajeros", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")
	principal := registerID(t, st, "Caja principal")
	abrirCajaPrincipal(t, st, ana)

	venta := func(quien int64, metodo int16, monto string) {
		t.Helper()
		o, err := crearYCobrar(t, ctx, ordersSvc, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: quien,
			Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
			Payments: []app.PaymentInput{{MethodID: metodo, Amount: decimal.RequireFromString(monto)}},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := ordersSvc.DeliverAll(ctx, o.ID); err != nil {
			t.Fatalf("DeliverAll: %v", err)
		}
	}
	venta(ana, efectivo, "100")
	venta(ana, efectivo, "100")
	venta(luis, efectivo, "100")
	// El no-efectivo va aparte: no está en el cajón, así que no puede explicar una diferencia.
	venta(luis, tarjeta, "100")

	vista, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	por := map[string]app.CashierTotal{}
	for _, c := range vista.Cashiers {
		por[c.Name] = c
	}
	if len(por) != 2 {
		t.Fatalf("el arqueo separa %d personas, quiere 2: %v", len(por), vista.Cashiers)
	}
	if got := por["Test ana_caja"].Cash.String(); got != "200" {
		t.Errorf("efectivo de Ana = %s, quiere 200", got)
	}
	if got := por["Test luis_caja"].Cash.String(); got != "100" {
		t.Errorf("efectivo de Luis = %s, quiere 100", got)
	}
	if got := por["Test luis_caja"].Other.String(); got != "100" {
		t.Errorf("no-efectivo de Luis = %s, quiere 100", got)
	}
	if got := por["Test ana_caja"].Other.String(); got != "0" {
		t.Errorf("no-efectivo de Ana = %s, quiere 0", got)
	}
}
