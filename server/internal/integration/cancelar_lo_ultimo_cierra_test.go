//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// pedidoDeVarios deja un pedido de mostrador abierto, sin cobrar, con un renglón por producto.
// Los productos pasan por cocina (needs_prep por omisión), así que cobrarlo NO lo cierra solo: es
// la forma del pedido del incidente.
func pedidoDeVarios(t *testing.T, st *store.Store, svc *app.OrdersService, sufijo string, n int) (*app.OrderView, int64, int16) {
	t.Helper()
	cajero := makeUser(t, st, "cajero_"+sufijo, "cajero")
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)
	return crearPedidoDeVarios(t, st, svc, cajero, sufijo, n), cajero, efectivo
}

// crearPedidoDeVarios crea el pedido con la caja ya abierta, para las pruebas que crean varios.
func crearPedidoDeVarios(t *testing.T, st *store.Store, svc *app.OrdersService, cajero int64, sufijo string, n int) *app.OrderView {
	t.Helper()
	lineas := make([]domain.OrderLineInput, 0, n)
	for i := range n {
		prod := makeProduct(t, st, "Plato "+sufijo+" "+itoa(i), pesos("50"), false)
		lineas = append(lineas, domain.OrderLineInput{ProductID: prod, Qty: pesos("1")})
	}
	ord, err := svc.Create(context.Background(), app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero, Lines: lineas,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(ord.Lines) != n {
		t.Fatalf("el pedido quedó con %d renglones, quiere %d", len(ord.Lines), n)
	}
	return ord
}

// estadoYCierre lee el estado y si el pedido tiene completed_at directo de la base: es lo que
// leen el corte y la barra, no lo que devuelve el servicio.
func estadoYCierre(t *testing.T, st *store.Store, orderID int64) (string, bool) {
	t.Helper()
	var estado string
	var cerrado bool
	if err := st.Pool.QueryRow(context.Background(),
		`select status::text, completed_at is not null from orders where id = $1`, orderID).Scan(&estado, &cerrado); err != nil {
		t.Fatalf("leer el pedido %d: %v", orderID, err)
	}
	return estado, cerrado
}

// EL INCIDENTE: un pedido que nada podía cerrar y que bloqueaba el corte de caja.
//
// Un pedido de mostrador creció por lotes, se le entregó un renglón y el resto se canceló uno por
// uno. Entregar un renglón cierra el pedido cuando ya no falta nada (DeliverLine →
// cerrarSiYaSeEntregoTodo), pero CANCELAR el último pendiente no lo hacía: el pedido se quedaba
// `abierta` con todo lo vivo entregado y pagado. El tablero decía «Todo entregado» y escondía
// «Entregar todo», y cancelarlo completo da 409 porque ya soltó comida. Sin salida desde la
// pantalla.
//
// Corre bajo el rol de la aplicación: el cierre escribe `orders` desde un camino que antes no lo
// hacía, y un grant que faltara aparecería en producción como 42501 en el último toque.
func TestCancellingTheLastPendingLineClosesTheOrder(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	ownerSvc := app.NewOrdersService(st, clock)
	ord, cajero, efectivo := pedidoDeVarios(t, st, ownerSvc, "incidente", 5)

	appSt := appRoleStore(t)
	tctx, release, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	defer release()
	svc := app.NewOrdersService(appSt, clock)

	// Más renglones en dos lotes, como en el pedido real.
	extra := func(nombre string) domain.OrderLineInput {
		return domain.OrderLineInput{ProductID: makeProduct(t, st, nombre, pesos("30"), false), Qty: pesos("1")}
	}
	for _, lote := range [][]domain.OrderLineInput{
		{extra("Lote1 a"), extra("Lote1 b")},
		{extra("Lote2 a"), extra("Lote2 b"), extra("Lote2 c")},
	} {
		if _, err := svc.AddLines(tctx, ord.ID, lote, cajero, uuid.New()); err != nil {
			t.Fatalf("AddLines: %v", err)
		}
	}
	vista, err := svc.Detail(tctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(vista.Lines) != 10 {
		t.Fatalf("el pedido tiene %d renglones, quiere 10", len(vista.Lines))
	}
	ids := make([]int64, 0, len(vista.Lines))
	for _, l := range vista.Lines {
		ids = append(ids, l.ID)
	}

	// Se cancelan dos, se entrega uno.
	for _, id := range ids[:2] {
		if _, err := svc.CancelarRenglon(tctx, ord.ID, id, cajero, "se equivocó"); err != nil {
			t.Fatalf("cancelar renglón %d: %v", id, err)
		}
	}
	entregado := ids[2]
	if err := svc.DeliverLine(tctx, ord.ID, entregado, pesos("1")); err != nil {
		t.Fatalf("entregar renglón %d: %v", entregado, err)
	}

	// Cancelar el pedido completo rebota: ya soltó comida. Es el 409 del incidente, y es correcto.
	err = svc.CancelarConDevolucion(tctx, app.CancelacionCmd{CardFolio: "F-1", OrderID: ord.ID, Motivo: "se fue", ActorID: cajero})
	if !errors.Is(err, domain.ErrCancelarConEntregas) {
		t.Fatalf("cancelar el pedido con un renglón entregado = %v, quiere ErrCancelarConEntregas", err)
	}

	// Se cancela todo lo demás. Al cancelar el ÚLTIMO pendiente ya no falta nada por entregar.
	for _, id := range ids[3:] {
		if _, err := svc.CancelarRenglon(tctx, ord.ID, id, cajero, "ya no lo quiso"); err != nil {
			t.Fatalf("cancelar renglón %d: %v", id, err)
		}
	}
	if estado, cerrado := estadoYCierre(t, st, ord.ID); estado != domain.StatusEntregada || !cerrado {
		t.Fatalf("tras cancelar el último pendiente: estado=%s completed_at=%v, quiere entregada con completed_at. "+
			"Es el pedido del incidente: todo lo vivo entregado y nada que lo cierre — bloquea el corte de caja", estado, cerrado)
	}

	// Se cobra lo que queda. Cobrar un pedido entregado con saldo es el camino normal del «por
	// cobrar», y no lo reabre.
	vista, err = svc.Detail(tctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if !vista.Outstanding.Equal(pesos("50")) {
		t.Fatalf("falta por cobrar %s, quiere 50 (solo el renglón entregado)", vista.Outstanding)
	}
	if _, err := svc.Charge(tctx, app.ChargeCmd{
		OrderID: ord.ID, MethodID: efectivo, Amount: vista.Outstanding, ActorID: cajero,
	}); err != nil {
		t.Fatalf("cobrar el saldo: %v", err)
	}
	if estado, _ := estadoYCierre(t, st, ord.ID); estado != domain.StatusEntregada {
		t.Fatalf("tras cobrar: estado=%s, quiere entregada", estado)
	}

	// Doble tap sobre el último renglón cancelado: no-op, no error, y el pedido sigue cerrado.
	if _, err := svc.CancelarRenglon(tctx, ord.ID, ids[len(ids)-1], cajero, "doble tap"); err != nil {
		t.Fatalf("volver a cancelar un renglón ya cancelado = %v, quiere nil (idempotente)", err)
	}
	if estado, _ := estadoYCierre(t, st, ord.ID); estado != domain.StatusEntregada {
		t.Fatalf("tras el doble tap: estado=%s, quiere entregada", estado)
	}
}

// Si se cancela TODO y nada se entregó, el pedido NO se marca entregado: nadie recibió nada, y
// cerrarlo así convertiría una cancelación renglón a renglón en una venta en el corte (la regla
// vive en domain.TodoEntregado). Tampoco queda atorado: como no soltó comida, se cancela completo.
func TestCancellingEveryLineLeavesTheOrderOpenToBeCancelled(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero, _ := pedidoDeVarios(t, st, svc, "todo_cancelado", 3)

	for _, l := range ord.Lines {
		if _, err := svc.CancelarRenglon(ctx, ord.ID, l.ID, cajero, "se equivocó"); err != nil {
			t.Fatalf("cancelar renglón %d: %v", l.ID, err)
		}
	}
	if estado, cerrado := estadoYCierre(t, st, ord.ID); estado != domain.StatusAbierta || cerrado {
		t.Fatalf("todo cancelado sin entregas: estado=%s completed_at=%v, quiere abierta sin cerrar — "+
			"marcarlo entregado lo contaría como venta", estado, cerrado)
	}
	if err := svc.CancelarConDevolucion(ctx, app.CancelacionCmd{CardFolio: "F-1", OrderID: ord.ID, Motivo: "se fue", ActorID: cajero}); err != nil {
		t.Fatalf("cancelar el pedido vacío = %v, quiere nil: si no, queda atorado igual que el del incidente", err)
	}
	if estado, _ := estadoYCierre(t, st, ord.ID); estado != domain.StatusCancelada {
		t.Fatalf("estado=%s, quiere cancelada", estado)
	}
}

// Una entrega PARCIAL en otro renglón sigue siendo comida pendiente: cancelar el resto no cierra el
// pedido, y entregar lo que falta sí.
func TestCancellingWithAPartialDeliveryElsewhereKeepsTheOrderOpen(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_parcial_cancela", "cajero")
	alitas := makeProduct(t, st, "Alitas parcial cancela", pesos("40"), false)
	papas := makeProduct(t, st, "Papas parcial cancela", pesos("60"), false)
	abrirCajaPrincipal(t, st, cajero)
	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: alitas, Qty: pesos("5")}, {ProductID: papas, Qty: pesos("1")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	alitasID, papasID := ord.Lines[0].ID, ord.Lines[1].ID

	if err := svc.DeliverLine(ctx, ord.ID, alitasID, pesos("3")); err != nil {
		t.Fatalf("entregar 3 de 5: %v", err)
	}
	if _, err := svc.CancelarRenglon(ctx, ord.ID, papasID, cajero, "no hay papas"); err != nil {
		t.Fatalf("cancelar papas: %v", err)
	}
	if estado, _ := estadoYCierre(t, st, ord.ID); estado != domain.StatusAbierta {
		t.Fatalf("faltan 2 alitas y el pedido quedó %s, quiere abierta", estado)
	}
	if err := svc.DeliverLine(ctx, ord.ID, alitasID, pesos("2")); err != nil {
		t.Fatalf("entregar las 2 que faltaban: %v", err)
	}
	if estado, cerrado := estadoYCierre(t, st, ord.ID); estado != domain.StatusEntregada || !cerrado {
		t.Fatalf("estado=%s completed_at=%v, quiere entregada", estado, cerrado)
	}
}

// El pedido en `lista` también se cierra: es el otro estado vivo desde el que se cancela un
// renglón.
func TestCancellingTheLastPendingLineClosesAReadyOrder(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, cajero, _ := pedidoDeVarios(t, st, svc, "lista", 2)

	if err := svc.DeliverLine(ctx, ord.ID, ord.Lines[0].ID, pesos("1")); err != nil {
		t.Fatalf("entregar: %v", err)
	}
	if err := svc.SetStatus(ctx, ord.ID, domain.StatusLista); err != nil {
		t.Fatalf("marcar lista: %v", err)
	}
	if _, err := svc.CancelarRenglon(ctx, ord.ID, ord.Lines[1].ID, cajero, "ya no lo quiso"); err != nil {
		t.Fatalf("cancelar: %v", err)
	}
	if estado, cerrado := estadoYCierre(t, st, ord.ID); estado != domain.StatusEntregada || !cerrado {
		t.Fatalf("estado=%s completed_at=%v, quiere entregada", estado, cerrado)
	}
}

// Dos personas a la vez: una entrega el penúltimo renglón y otra cancela el último. Cada una, por
// separado, ve al otro renglón pendiente; si no se serializan sobre el pedido, ninguna lo cierra y
// el pedido del incidente vuelve por la puerta de la concurrencia. Tampoco puede tronar por
// interbloqueo: entregar toma el pedido y luego los renglones, y cancelar tiene que tomarlos en
// el mismo orden.
func TestConcurrentDeliverAndCancelStillCloseTheOrder(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_carrera", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	for i := range 15 {
		ord := crearPedidoDeVarios(t, st, svc, cajero, "carrera_"+itoa(i), 2)
		var wg sync.WaitGroup
		var errEntregar, errCancelar error
		wg.Add(2)
		go func() {
			defer wg.Done()
			errEntregar = svc.DeliverLine(ctx, ord.ID, ord.Lines[0].ID, pesos("1"))
		}()
		go func() {
			defer wg.Done()
			_, errCancelar = svc.CancelarRenglon(ctx, ord.ID, ord.Lines[1].ID, cajero, "ya no lo quiso")
		}()
		wg.Wait()
		if errEntregar != nil || errCancelar != nil {
			t.Fatalf("vuelta %d: entregar=%v cancelar=%v, quiere los dos sin error", i, errEntregar, errCancelar)
		}
		if estado, cerrado := estadoYCierre(t, st, ord.ID); estado != domain.StatusEntregada || !cerrado {
			t.Fatalf("vuelta %d: estado=%s completed_at=%v, quiere entregada: las dos transacciones se cruzaron y ninguna cerró", i, estado, cerrado)
		}
	}
}
