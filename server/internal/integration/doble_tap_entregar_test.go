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

// UN DOBLE TAP EN "ENTREGAR TODO" NO PUEDE CONTESTAR ERROR SOBRE UNA ENTREGA QUE SÍ OCURRIÓ.
//
// `SetStatus` ya tenía el no-op idempotente, con su comentario: "un doble-tap en el tablero no debe
// dar error". `DeliverAll` nació después y usa `CanTransition` a pelo, y `entregada → entregada` es
// false: el segundo tap pintaba un toast rojo sobre un pedido correctamente entregado. En una
// pantalla táctil el doble tap es el primer borde de la lista, y aquí además el botón no se apaga
// mientras la petición viaja.
func TestUnDobleTapEnEntregarTodoNoDaError(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordenes := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_doble_entrega", "cajero")
	prod := makeProduct(t, st, "Alitas doble tap", decimal.RequireFromString("100"), false)
	abrirCajaPrincipal(t, st, cajero)

	ord, err := ordenes.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := ordenes.DeliverAll(ctx, ord.ID); err != nil {
		t.Fatalf("primer toque: %v", err)
	}
	if err := ordenes.DeliverAll(ctx, ord.ID); err != nil {
		t.Fatalf("el segundo toque contestó %v sobre un pedido ya entregado: el operador ve un "+
			"error rojo por algo que salió bien", err)
	}

	// Y no se coló otra transición: sigue entregada, no cerrada ni cancelada.
	visto, err := ordenes.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if visto.Status != domain.StatusEntregada {
		t.Fatalf("estado = %s, quiere entregada", visto.Status)
	}

	// El motivo en blanco sigue siendo un rechazo: el no-op es para repetir lo mismo, no para
	// aflojar lo demás.
	if err := ordenes.Cancel(ctx, ord.ID, cajero, "   "); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("cancelar con un motivo en blanco: err = %v, quiere ErrValidation", err)
	}
}
