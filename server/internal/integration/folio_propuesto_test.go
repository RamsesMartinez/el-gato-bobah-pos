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
	ord, err := svc.Create(context.Background(), app.CreateOrderCmd{
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
func TestElNombreQueProponeLaPantallaEsElQueSeGuarda(t *testing.T) {
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_prop", "cajero")
	prod := makeProduct(t, st, "Café prop", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord, err := svc.Create(context.Background(), app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero, FolioName: "Ajolote",
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("50")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ord.FolioName != "Ajolote" {
		t.Fatalf("folio = %q, quiere Ajolote", ord.FolioName)
	}
}

// Dos cuentas abiertas a la vez pueden proponer el mismo animal; la pantalla no sabe qué se usó
// hoy. El segundo lleva su vuelta —conserva el animal, que es lo que el cliente ya oyó— y NO
// desplaza al primero.
func TestDosCuentasConElMismoAnimalNoChocan(t *testing.T) {
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_choque", "cajero")
	prod := makeProduct(t, st, "Café choque", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	primero := ventaConFolio(t, svc, cajero, prod, efectivo, "Tejón")
	segundo := ventaConFolio(t, svc, cajero, prod, efectivo, "Tejón")
	tercero := ventaConFolio(t, svc, cajero, prod, efectivo, "Tejón")

	if primero.FolioName != "Tejón" {
		t.Errorf("el primero = %q, quiere Tejón", primero.FolioName)
	}
	if segundo.FolioName != "Tejón 2" {
		t.Errorf("el segundo = %q, quiere Tejón 2", segundo.FolioName)
	}
	if tercero.FolioName != "Tejón 3" {
		t.Errorf("el tercero = %q, quiere Tejón 3", tercero.FolioName)
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
