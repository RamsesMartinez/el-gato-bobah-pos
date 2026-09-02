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

// EL DEFECTO, VISTO EN EL AMBIENTE DE PRUEBAS: el POS deja de poder crear pedidos.
//
// `POST /orders` respondía 500 con "duplicate key value violates unique constraint
// orders_folio_dia". No es un error de captura ni de permisos: es el sistema negándose a vender, y
// la única salida del operador es esperar al día siguiente.
//
// La causa es el hermano que no se movió. Cuando la pantalla empezó a PROPONER el nombre —para que
// el operador lo vea desde el primer producto—, el camino en el que el servidor asigna el nombre se
// quedó con su suposición vieja: que un nombre derivado del folio numérico no puede chocar. Dejó de
// ser cierto en cuanto los nombres propuestos empezaron a ocupar lugares de esa misma lista. El
// comentario de `resolverFolio` todavía lo afirma.
func TestUnFolioOcupadoNoTumbaLaVenta(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_folio", "cajero")
	prod := makeProduct(t, st, "Café folio", decimal.RequireFromString("50"), false)
	abrirCajaPrincipal(t, st, cajero)

	nuevo := func(propuesto string) (*app.OrderView, error) {
		return svc.Create(ctx, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			FolioName: propuesto,
			Lines:     []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		})
	}

	// El primero se lo queda el servidor y nos dice qué nombre le tocó.
	primero, err := nuevo("")
	if err != nil {
		t.Fatalf("primer pedido: %v", err)
	}

	// La pantalla del segundo pedido propone EXACTAMENTE ese nombre. Es lo que pasa todos los días:
	// dos estaciones abren cuenta a la vez y la lista de animales es la misma.
	segundo, err := nuevo(primero.FolioName)
	if err != nil {
		t.Fatalf("el nombre propuesto que ya estaba usado tumbó el pedido: %v", err)
	}
	if segundo.FolioName == primero.FolioName {
		t.Fatalf("dos pedidos del día se llaman igual (%q): cantarlos en cocina deja de distinguirlos",
			segundo.FolioName)
	}

	// Y AHORA el caso que rompió el ambiente de pruebas, reproducido como ocurrió: un día con muchos
	// pedidos, mezclando los nombres que PROPONE la pantalla con los que asigna el servidor.
	//
	// La pantalla propone de la misma lista de animales, pero en el orden en que ella los reparte —no
	// conoce la baraja del día del servidor—. Así que en cuanto la lista se consume a medias, el
	// nombre que al servidor le toca por folio numérico ya lo ocupó una cuenta que la pantalla
	// bautizó. Antes eso salía como un 500 y el operador se quedaba sin poder vender.
	animales := domain.FolioNames()
	for i := 0; i < len(animales)+10; i++ {
		propuesto := ""
		if i%2 == 0 {
			propuesto = animales[i%len(animales)]
		}
		if _, err := nuevo(propuesto); err != nil {
			t.Fatalf("el pedido %d del día tumbó la venta: %v -- un choque de nombre no puede impedir "+
				"cobrar: el nombre existe para CANTAR el pedido, no para autorizarlo", i+3, err)
		}
	}
}
