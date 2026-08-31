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
	pedido, err := ordersSvc.Create(ctx, app.CreateOrderCmd{
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
	pedido, err := ordersSvc.Create(ctx, app.CreateOrderCmd{
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
