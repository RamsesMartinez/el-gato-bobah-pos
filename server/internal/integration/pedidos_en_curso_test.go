//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// LA BARRA DEL POS NO ES LA LISTA DE IMPAGOS.
//
// Son dos conjuntos y fundirlos sin pensarlo pierde uno de los dos:
//
//   - si solo se listaran los impagos, el pedido YA COBRADO y todavía en cocina no aparecería, y
//     ese es al que el cliente le pide algo más — el motivo de la feature;
//   - si solo se listaran los no terminados, desaparecería el pedido ENTREGADO y sin cobrar, que es
//     el pendiente caro: el cliente ya se fue. Es justo para lo que existía la píldora que esta
//     lista reemplaza.
//
// Este test es lo que impide que un refactor se quede con la mitad.
func TestLaListaDeEnCursoEsLaUnionDeLosDosConjuntos(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_encurso", "cajero")
	prod := makeProduct(t, st, "Café en curso", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	crear := func(pagar bool) *app.OrderView {
		t.Helper()
		cmd := app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		}
		if pagar {
			cmd.Payments = []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("100")}}
		}
		o, err := crearYCobrar(t, ctx, svc, cmd)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return o
	}

	enCocinaSinCobrar := crear(false)
	enCocinaYaCobrado := crear(true)
	entregadoSinCobrar := crear(false)
	if err := svc.DeliverAll(ctx, entregadoSinCobrar.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}
	entregadoYCobrado := crear(true)
	if err := svc.DeliverAll(ctx, entregadoYCobrado.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}
	cancelado := crear(false)
	if err := svc.Cancel(ctx, cancelado.ID, cajero, "prueba"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	lista, _, err := svc.Open(ctx, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	dentro := map[int64]app.BoardOrder{}
	for _, o := range lista {
		dentro[o.ID] = o
	}

	casos := []struct {
		nombre        string
		id            int64
		quiere        bool
		enPreparacion bool
	}{
		{"en cocina sin cobrar", enCocinaSinCobrar.ID, true, true},
		{"en cocina YA cobrado — a este es al que el cliente le pide más", enCocinaYaCobrado.ID, true, true},
		{"entregado sin cobrar — el pendiente caro, el cliente ya se fue", entregadoSinCobrar.ID, true, false},
		{"entregado y cobrado — ya no hay nada que hacerle", entregadoYCobrado.ID, false, false},
		{"cancelado — su dinero ya se decidió", cancelado.ID, false, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			o, hay := dentro[c.id]
			if hay != c.quiere {
				t.Fatalf("presente = %v, quiere %v", hay, c.quiere)
			}
			if hay && o.EnPreparacion != c.enPreparacion {
				t.Errorf("enPreparacion = %v, quiere %v: la pantalla no sabría si se le puede agregar",
					o.EnPreparacion, c.enPreparacion)
			}
		})
	}
}

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

// EL TURNO QUE CRUZA LA MEDIANOCHE NO PUEDE VACIAR LA BARRA.
//
// El pedido hereda la fecha de negocio del TURNO —así los tickets numeran corrido en una noche que
// abre a las 4pm y cierra a las 10pm, en vez de partirse a medianoche— pero la barra filtraba por la
// fecha del SERVIDOR. En cuanto el reloj cruza la medianoche con el turno abierto, los dos dejan de
// coincidir y todos los pedidos en curso desaparecen de la pantalla.
//
// El servidor corre en UTC y el local cierra a las 22:00 hora de México: la medianoche UTC cae a las
// 18:00 locales, o sea que la barra se vaciaba TODAS las noches en plena hora pico, con los pedidos
// vivos y el operador sin forma de llegar a ellos. Lo encontré verificando el despliegue en el
// ambiente de pruebas, con un turno abierto el día anterior.
func TestLaBarraSigueMostrandoElTurnoQueCruzoLaMedianoche(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// El turno abrió AYER y sigue abierto; el reloj del servidor ya es de hoy.
	ayer := fixedNow
	hoy := fixedNow.Add(26 * time.Hour)
	svcDeAyer := app.NewOrdersService(st, func() time.Time { return ayer })
	svcDeHoy := app.NewOrdersService(st, func() time.Time { return hoy })

	cajero := makeUser(t, st, "cajero_medianoche", "cajero")
	prod := makeProduct(t, st, "Café medianoche", decimal.RequireFromString("100"), false)
	abrirCajaPrincipal(t, st, cajero)

	ord, err := svcDeAyer.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("confirmar: %v", err)
	}

	lista, _, err := svcDeHoy.Open(ctx, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, o := range lista {
		if o.ID == ord.ID {
			return
		}
	}
	t.Errorf("el pedido del turno abierto desapareció de la barra al cruzar la medianoche: los pedidos siguen vivos y el operador no tiene cómo llegar a ellos")
}
