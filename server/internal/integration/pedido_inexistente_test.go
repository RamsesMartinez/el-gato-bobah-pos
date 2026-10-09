//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// UN PEDIDO QUE NO EXISTE ES 404, NO 500.
//
// `load` devolvía el `pgx.ErrNoRows` crudo y `httpapi.Error` no lo reconoce, así que consultar un id
// inexistente contestaba "Error interno del servidor". Un 500 dice que el servidor se rompió y manda
// a revisar logs; aquí lo único que pasó es que ese pedido no está.
//
// Se volvió visible al quitar la ruta `/orders/unpaid`: cualquier cliente que siguiera llamándola
// caía en `/orders/{id}` y recibía un 500 en vez de un error que se entiende.
func TestConsultarUnPedidoQueNoExisteEsNoEncontrado(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	_, err := svc.Detail(ctx, 99999999)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Detail de un id inexistente = %v, quiere ErrNotFound: un 500 manda a revisar logs por un pedido que simplemente no está", err)
	}
}
