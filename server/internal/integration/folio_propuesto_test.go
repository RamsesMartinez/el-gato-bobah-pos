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

func ventaConFolio(t *testing.T, svc *app.OrdersService, cajero, prod int64, metodo int16, folio string) *app.OrderView {
	t.Helper()
	ord, err := crearYCobrar(t, context.Background(), svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero, FolioName: folio,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: metodo, Amount: decimal.RequireFromString("50")}},
	})
	if err != nil {
		t.Fatalf("Create(folio=%q): %v", folio, err)
	}
	return ord
}

// La pantalla le pone nombre a la cuenta al abrirla, para que el operador lo vea desde el primer
// producto. Ese nombre tiene que ser EL MISMO que acaba en el ticket: si cambiara al cobrar, el
// operador ya le habría dicho otro al cliente.
//
// Se propone un nombre DEL ESQUEMA del negocio. El servidor solo honra los de su propia lista: uno
// de fuera vendría de una tableta con la lista de otro esquema, y honrarlo dejaría el ticket con un
// nombre que la bolsa no conoce y que volvería a salir en la misma vuelta.
func TestElNombreQueProponeLaPantallaEsElQueSeGuarda(t *testing.T) {
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_prop", "cajero")
	prod := makeProduct(t, st, "Café prop", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	propuesto := domain.NombresDelEsquema(domain.EsquemaPorDefecto)[0]
	ord, err := crearYCobrar(t, context.Background(), svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero, FolioName: propuesto,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("50")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ord.FolioName != propuesto {
		t.Fatalf("folio = %q, quiere %q", ord.FolioName, propuesto)
	}
}

// DOS CUENTAS QUE PROPONEN EL MISMO NOMBRE: LA SEGUNDA SE VA A OTRO, NO A "Persa 2".
//
// Antes se conservaba el nombre y se numeraba, con el argumento de que el cliente ya lo había oído.
// El dueño lo revirtió: "Persa 2" con el primer Persa todavía en la plancha es cómo se entrega el
// pedido equivocado, y mientras quede un nombre sin usar en la bolsa hay uno mejor que un número.
//
// El numerado sigue existiendo, pero como ÚLTIMA red: solo cuando el día ya pasó del largo de la
// lista y no queda nada fresco. Eso lo cubre el unitario del dominio.
func TestDosCuentasConElMismoNombreSeVanACaminosDistintos(t *testing.T) {
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_choque", "cajero")
	prod := makeProduct(t, st, "Café choque", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	lista := domain.NombresDelEsquema(domain.EsquemaPorDefecto)
	repetido := lista[0]
	primero := ventaConFolio(t, svc, cajero, prod, efectivo, repetido)
	segundo := ventaConFolio(t, svc, cajero, prod, efectivo, repetido)
	tercero := ventaConFolio(t, svc, cajero, prod, efectivo, repetido)

	// El primero SÍ se queda con lo que propuso la pantalla: es lo que el operador ya dijo.
	if primero.FolioName != repetido {
		t.Errorf("el primero = %q, quiere %q: la propuesta libre tiene que respetarse", primero.FolioName, repetido)
	}
	for i, o := range []*app.OrderView{segundo, tercero} {
		if o.FolioName == repetido+" 2" || o.FolioName == repetido+" 3" {
			t.Errorf("el %d° se llamó %q: con nombres libres en la bolsa, numerar no es la salida",
				i+2, o.FolioName)
		}
		if !contieneNombre(lista, o.FolioName) {
			t.Errorf("el %d° se llamó %q, que no está en la lista del esquema", i+2, o.FolioName)
		}
	}
	if segundo.FolioName == tercero.FolioName {
		t.Errorf("el segundo y el tercero se llaman igual (%q)", segundo.FolioName)
	}
}

// El nombre se imprime en el ticket del cliente y en la comanda. Sin el filtro, un cliente de la
// API podría meter cualquier texto en un papel que lleva el nombre del negocio.
func TestUnFolioConBasuraNoLlegaAlPapel(t *testing.T) {
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_basura", "cajero")
	prod := makeProduct(t, st, "Café basura", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	for _, basura := range []string{"MESA GRATIS!!", "<script>", "Tigre\nGRATIS", "x"} {
		ord := ventaConFolio(t, svc, cajero, prod, efectivo, basura)
		if ord.FolioName == basura {
			t.Errorf("se guardó el folio crudo %q", basura)
		}
		// Y no se queda sin nombre: cae al que reparte el servidor.
		if ord.FolioName == "" {
			t.Errorf("con folio %q el pedido quedó sin nombre", basura)
		}
	}
}

// Sin propuesta —clientes de API, tests— el servidor reparte el suyo, como antes.
func TestSinPropuestaElServidorRepiteSuNombre(t *testing.T) {
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_sinprop", "cajero")
	prod := makeProduct(t, st, "Café sinprop", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord := ventaConFolio(t, svc, cajero, prod, efectivo, "")
	if ord.FolioName == "" {
		t.Fatal("sin propuesta el pedido quedó sin nombre")
	}
}
