//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Un pedido cuyos productos NINGUNO necesita preparación nace entregado y no pasa por el tablero.
//
// Es el refresco que el cliente toma de la nevera. Antes esto lo decidía el operador con un
// interruptor en el cobro, y equivocarse era caro: un ticket con un refresco y unas alitas marcado
// a mano escondía las alitas del tablero y nadie las preparaba. Ahora lo sabe el catálogo.
func TestUnPedidoSinNadaQuePrepararNaceEntregado(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_prep", "cajero")
	refresco := makeProduct(t, st, "Refresco nevera", decimal.RequireFromString("25"), false)
	sinPreparacion(t, st, refresco)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: refresco, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("25")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ord.Status != domain.StatusEntregada {
		t.Fatalf("estado = %s, quiere entregada", ord.Status)
	}
	// Y sus renglones nacen entregados. Si no, el pedido saldría cerrado pero contando comida
	// pendiente, y cualquier lectura de "qué falta" arrancaría mintiendo.
	for _, l := range ord.Lines {
		if !l.Delivered.Equal(l.Quantity) {
			t.Errorf("%s nació con %s de %s entregado", l.ProductName, l.Delivered, l.Quantity)
		}
	}
}

// EL CASO QUE ROMPÍA EL INTERRUPTOR: un ticket mezclado. Basta un producto que sí necesita cocina
// para que el pedido entero vaya al tablero — si no, ese producto desaparece y nadie lo prepara.
func TestUnPedidoMEZCLADOVaAlTablero(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_mezcla", "cajero")
	refresco := makeProduct(t, st, "Refresco mezcla", decimal.RequireFromString("25"), false)
	sinPreparacion(t, st, refresco)
	alitas := makeProduct(t, st, "Alitas mezcla", decimal.RequireFromString("200"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{
			{ProductID: refresco, Qty: decimal.RequireFromString("1")},
			{ProductID: alitas, Qty: decimal.RequireFromString("1")},
		},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("225")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ord.Status != domain.StatusAbierta {
		t.Fatalf("estado = %s, quiere abierta: las alitas necesitan prepararse", ord.Status)
	}
}

// El default es "sí pasa por cocina", así que el catálogo migrado se comporta exactamente igual que
// antes hasta que alguien apague un producto a propósito.
func TestElDefaultEsQueSiPasaPorCocina(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_default", "cajero")
	prod := makeProduct(t, st, "Café default", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("50")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ord.Status != domain.StatusAbierta {
		t.Fatalf("estado = %s, quiere abierta", ord.Status)
	}
}

// Un pedido sin preparación que NO se cobró completo no puede nacer entregado: sería regalar comida
// sin dejar rastro, porque el pedido nace terminado y no vuelve a aparecer en ninguna pantalla.
func TestSinPreparacionPeroSinCobrarVaAlTablero(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_sincobrar", "cajero")
	refresco := makeProduct(t, st, "Refresco fiado", decimal.RequireFromString("25"), false)
	sinPreparacion(t, st, refresco)
	abrirCajaPrincipal(t, st, cajero)

	ord, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: refresco, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ord.Status != domain.StatusAbierta {
		t.Fatalf("estado = %s, quiere abierta: no se cobró", ord.Status)
	}
}

func sinPreparacion(t *testing.T, st *store.Store, id int64) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(),
		`update products set needs_prep = false where id = $1`, id); err != nil {
		t.Fatalf("marcar sin preparación: %v", err)
	}
}
