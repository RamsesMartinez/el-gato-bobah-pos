//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// LA HOJA DEL POS PIDE SOLO LO QUE SE PUEDE COBRAR, Y NO PUEDE PERDER EL PENDIENTE CARO.
//
// Medido en el ambiente de pruebas: la hoja abría con 30 renglones, 14 de ellos ya cobrados, en una
// pantalla donde caben cinco. Quien la abre viene a cobrar.
//
// El recorte es fácil de hacer mal de dos maneras, y este test cierra las dos:
//
//   - quedarse con `en preparación y sin cobrar` borra el pedido ENTREGADO sin cobrar — el caro,
//     porque el cliente ya se fue con la comida y nadie lo persigue si no aparece;
//   - filtrar en la pantalla en vez de en el servidor deja el total del encabezado sumando filas
//     que la lista no muestra. Ya pasó una vez: $2,141 en la píldora contra $1,928 en la lista.
func TestLaBarraPuedePedirSoloLoQueFaltaPorCobrar(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_porcobrar", "cajero")
	prod := makeProduct(t, st, "Café por cobrar", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	crear := func(abona string) *app.OrderView {
		t.Helper()
		cmd := app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		}
		if abona != "" {
			cmd.Payments = []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString(abona)}}
		}
		o, err := crearYCobrar(t, ctx, svc, cmd)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return o
	}

	enCocinaSinCobrar := crear("")
	enCocinaYaCobrado := crear("100")
	// Abonado a medias: sigue debiendo $40 y tiene que poder cobrarse. Un filtro escrito como
	// "sin ningún pago" lo perdería.
	enCocinaAMedias := crear("60")
	entregadoSinCobrar := crear("")
	if err := svc.DeliverAll(ctx, entregadoSinCobrar.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}

	lista, pendiente, err := svc.Open(ctx, true)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	dentro := map[int64]app.BoardOrder{}
	for _, o := range lista {
		dentro[o.ID] = o
	}

	casos := []struct {
		nombre string
		id     int64
		quiere bool
	}{
		{"en cocina sin cobrar", enCocinaSinCobrar.ID, true},
		{"en cocina abonado a medias — sigue debiendo $40", enCocinaAMedias.ID, true},
		{"entregado sin cobrar — el caro: el cliente ya se fue", entregadoSinCobrar.ID, true},
		{"en cocina YA cobrado — es lo que este filtro viene a quitar", enCocinaYaCobrado.ID, false},
	}
	for _, c := range casos {
		if _, hay := dentro[c.id]; hay != c.quiere {
			t.Errorf("%s: presente = %v, quiere %v", c.nombre, hay, c.quiere)
		}
	}

	// El encabezado y la lista salen del MISMO recorrido. Si el filtro viviera en la pantalla, este
	// total seguiría contando al pedido que la lista ya no muestra.
	suma := decimal.Zero
	for _, o := range lista {
		if !o.Outstanding.IsPositive() {
			t.Errorf("pedido %d listado con $%s por cobrar: la hoja lo pintaría con un botón de Cobrar $0",
				o.ID, o.Outstanding)
		}
		suma = suma.Add(o.Outstanding)
	}
	if !pendiente.Equal(domain.Round2(suma)) {
		t.Errorf("outstanding = %s, la lista suma %s: el encabezado y el detalle dirían cifras distintas",
			pendiente, domain.Round2(suma))
	}
}
