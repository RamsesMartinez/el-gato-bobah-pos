//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// UN PEDIDO ABIERTO SE VE HASTA QUE ALGUIEN LO CIERRE, SIN IMPORTAR DE QUÉ DÍA SEA.
//
// Es el mecanismo con el que se limpia el rezago: hay once pedidos abiertos desde julio en la cuenta
// de pruebas que nadie ve y por lo tanto nadie cierra. Mostrarlos obliga a resolverlos, y al
// resolverlos salen solos.
//
// Antes la lista se ataba a la fecha del turno abierto —el arreglo parcial de la feature 005— y
// antes de eso al día del servidor, que la vaciaba a las 18:00 locales.
func TestUnPedidoAbiertoDeOtroDiaSigueEnLaLista(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	hoy := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_viejo", "cajero")
	prod := makeProduct(t, st, "Café viejo", decimal.RequireFromString("100"), false)
	abrirCajaPrincipal(t, st, cajero)

	ord, err := hoy.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("crear el pedido viejo: %v", err)
	}
	// Se lo envejece a mano: `Create` hereda la fecha del TURNO, así que crear con un reloj viejo no
	// alcanza — el turno seguiría siendo el de hoy. Esto es la forma que tiene un pedido de julio en
	// producción: su fecha quedó en el día de su turno y nadie lo cerró.
	haceDosMeses := fixedNow.Add(-60 * 24 * time.Hour)
	if _, err := st.Pool.Exec(ctx,
		`update orders set business_date = $2 where id = $1`, ord.ID, haceDosMeses); err != nil {
		t.Fatalf("envejecer el pedido: %v", err)
	}

	fila := app.NewAccountsService(st, hoy)
	lista, err := fila.Live(ctx, false)
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	var encontrado *app.AccountItem
	for i := range lista.Items {
		if lista.Items[i].OrderID != nil && *lista.Items[i].OrderID == ord.ID {
			encontrado = &lista.Items[i]
		}
	}
	if encontrado == nil {
		t.Fatal("el pedido abierto de hace dos meses no aparece: nadie lo ve y por lo tanto nadie lo cierra")
	}
	// Y la pantalla tiene que poder distinguirlo del trabajo de hoy, o el rezago se confunde con lo
	// que sí es de este turno.
	if encontrado.BusinessDate == nil || *encontrado.BusinessDate == "" {
		t.Error("la fila no dice de qué día es: el rezago se confundiría con los pedidos de hoy")
	}

	// Al entregarlo SIN cobrar no sale: desde la 030 la deuda de otro día se queda en la fila, en «De
	// días anteriores» (caso 3 del lienzo: esa deuda solo se veía en el arqueo). Al cobrarlo, sí sale.
	if err := hoy.DeliverAll(ctx, ord.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}
	tras, err := fila.Live(ctx, false)
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	var debe *app.AccountItem
	for i := range tras.Items {
		if o := tras.Items[i]; o.OrderID != nil && *o.OrderID == ord.ID {
			debe = &tras.Items[i]
		}
	}
	if debe == nil || debe.Group != domain.GroupPreviousDays {
		t.Fatalf("el pedido entregado que debe, de hace dos meses, no está en «De días anteriores»: %+v", debe)
	}
	if _, err := hoy.Charge(ctx, app.ChargeCmd{OrderID: ord.ID, MethodID: paymentMethodID(t, st, "Efectivo"),
		Amount: decimal.RequireFromString("100"), ActorID: cajero}); err != nil {
		t.Fatalf("Charge: %v", err)
	}
	cerrado, err := fila.Live(ctx, false)
	if err != nil {
		t.Fatalf("Live: %v", err)
	}
	for _, o := range cerrado.Items {
		if o.OrderID != nil && *o.OrderID == ord.ID {
			t.Error("el pedido entregado y cobrado sigue en la fila: está cerrado")
		}
	}
}
