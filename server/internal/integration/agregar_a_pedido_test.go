//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// pedidoEnCurso deja un pedido confirmado de un café, cobrado o no, y devuelve con qué agregarle.
func pedidoEnCurso(t *testing.T, st *store.Store, svc *app.OrdersService, sufijo string, pagado bool) (*app.OrderView, int64, int64) {
	ctx := context.Background()
	t.Helper()
	cajero := makeUser(t, st, "cajero_"+sufijo, "cajero")
	cafe := makeProduct(t, st, "Café "+sufijo, decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	cmd := app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: cafe, Qty: decimal.RequireFromString("1")}},
	}
	if pagado {
		cmd.Payments = []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("100")}}
	}
	ord, err := crearYCobrar(t, ctx, svc, cmd)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return ord, cafe, cajero
}

// A UN PEDIDO TERMINADO NO SE LE AGREGA, Y AGREGAR ES APPEND.
//
// Las dos propiedades de las que depende la barra de pedidos en curso:
//
//   - el chip sigue en pantalla hasta el siguiente refresco, así que la tableta que estuvo
//     suspendida puede intentar agregarle a un pedido que la otra estación ya entregó;
//   - dos agregados NO se pisan. Es lo que hace que dos estaciones puedan trabajar sobre el mismo
//     pedido sin perder una venta — la concurrencia real se ensaya a mano, porque un test de
//     goroutines aquí pasaría por el número de núcleos y no por el código.
func TestAgregarAUnPedidoEnCurso(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cafe, cajero := pedidoEnCurso(t, st, svc, "agrega", false)

	uno := []domain.OrderLineInput{{ProductID: cafe, Qty: decimal.RequireFromString("1")}}

	if _, err := svc.AddLines(ctx, ord.ID, uno, cajero); err != nil {
		t.Fatalf("primer agregado: %v", err)
	}
	if _, err := svc.AddLines(ctx, ord.ID, uno, cajero); err != nil {
		t.Fatalf("segundo agregado: %v", err)
	}
	tras, err := svc.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(tras.Lines) != 3 {
		t.Errorf("el pedido quedó con %d renglones, quiere 3: un agregado pisó al otro y se perdió una venta",
			len(tras.Lines))
	}

	// Y al pedido entregado ya no.
	if err := svc.DeliverAll(ctx, ord.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}
	err = func() error { _, e := svc.AddLines(ctx, ord.ID, uno, cajero); return e }()
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("agregar a un entregado = %v, quiere conflicto: entraría un renglón sobre un pedido que nadie va a preparar", err)
	}
	if !contieneEstado(err.Error()) {
		t.Errorf("el error no dice en qué estado quedó el pedido (%q): la tableta suspendida no se entera de qué pasó", err)
	}
}

func contieneEstado(msg string) bool {
	for _, e := range []string{"entregada", "cancelada", "reembolsada", "lista", "abierta"} {
		if strings.Contains(msg, e) {
			return true
		}
	}
	return false
}

// SI SE COBRÓ, NO PUEDE QUEDAR DEUDA ESCONDIDA.
//
// Agregarle a un pedido ya saldado es legítimo —el cliente pidió más— y deja un saldo nuevo. Lo que
// no puede pasar es que ese saldo no se vea: el pedido tiene que REAPARECER en la barra, que es
// donde el operador lo lee. Es la regla que el dueño puso cuando encontró un pedido cobrado que
// seguía apareciendo como deuda.
func TestAgregarAUnPedidoYaCobradoDejaSaldoVisible(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cafe, cajero := pedidoEnCurso(t, st, svc, "cobrado", true)

	// Saldado: no debe nada. Sigue en la barra porque sigue en cocina.
	antes := buscarEnCurso(t, svc, ord.ID)
	if !antes.Outstanding.IsZero() {
		t.Fatalf("el pedido nace debiendo %s y se cobró completo", antes.Outstanding)
	}

	if _, err := svc.AddLines(ctx, ord.ID, []domain.OrderLineInput{
		{ProductID: cafe, Qty: decimal.RequireFromString("2")},
	}, cajero); err != nil {
		t.Fatalf("AddLines: %v", err)
	}

	tras := buscarEnCurso(t, svc, ord.ID)
	if !tras.Outstanding.Equal(decimal.RequireFromString("200")) {
		t.Errorf("saldo = %s, quiere 200: el agregado a un pedido cobrado quedó como deuda invisible",
			tras.Outstanding)
	}
}

func buscarEnCurso(t *testing.T, svc *app.OrdersService, id int64) app.BoardOrder {
	t.Helper()
	lista, err := svc.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, o := range lista {
		if o.ID == id {
			return o
		}
	}
	t.Fatalf("el pedido %d no está en la barra de en curso", id)
	return app.BoardOrder{}
}
