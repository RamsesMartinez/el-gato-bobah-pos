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
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// pedidoDeAlitas deja un pedido abierto con un renglón de 5 y otro de 2, que es la forma del caso
// real: comida que sale por tandas.
func pedidoDeAlitas(t *testing.T, st *store.Store, svc *app.OrdersService, sufijo string) (*app.OrderView, int64, int64) {
	t.Helper()
	ctx := context.Background()
	cajero := makeUser(t, st, "cajero_"+sufijo, "cajero")
	alitas := makeProduct(t, st, "Alitas "+sufijo, decimal.RequireFromString("200"), false)
	papas := makeProduct(t, st, "Papas "+sufijo, decimal.RequireFromString("60"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	ord, err := svc.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{
			{ProductID: alitas, Qty: decimal.RequireFromString("5")},
			{ProductID: papas, Qty: decimal.RequireFromString("2")},
		},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("1120")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(ord.Lines) != 2 {
		t.Fatalf("el pedido quedó con %d renglones, quiere 2", len(ord.Lines))
	}
	return ord, ord.Lines[0].ID, ord.Lines[1].ID
}

// EL CASO QUE MOTIVA LA FEATURE: de 5 alitas salen 3 y las otras 2 siguen en la freidora. El
// pedido no puede cerrarse todavía, y lo entregado no se puede perder.
func TestSalenTresDeCincoAlitasYElPedidoSigueAbierto(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, alitas, _ := pedidoDeAlitas(t, st, svc, "parcial")

	if err := svc.DeliverLine(ctx, ord.ID, alitas, decimal.RequireFromString("3")); err != nil {
		t.Fatalf("entregar 3 de 5: %v", err)
	}

	tras, err := svc.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if tras.Status != domain.StatusAbierta {
		t.Errorf("estado = %s, quiere abierta: faltan 2 alitas y las papas", tras.Status)
	}
	if got := tras.Lines[0].Delivered; !got.Equal(decimal.RequireFromString("3")) {
		t.Errorf("entregado = %s, quiere 3", got)
	}
}

// Al marcar el último producto que faltaba, el pedido se cierra SOLO. Obligar al operador a
// marcar el renglón y además el pedido es pedirle dos veces lo mismo, y la segunda se olvida: el
// pedido se quedaría abierto toda la tarde y el cierre de caja lo reclamaría al final del turno.
func TestAlEntregarLoUltimoElPedidoSeCierraSolo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, alitas, papas := pedidoDeAlitas(t, st, svc, "cierra")

	if err := svc.DeliverLine(ctx, ord.ID, alitas, decimal.RequireFromString("3")); err != nil {
		t.Fatalf("entregar 3: %v", err)
	}
	if err := svc.DeliverLine(ctx, ord.ID, papas, decimal.RequireFromString("2")); err != nil {
		t.Fatalf("entregar papas: %v", err)
	}
	medio, _ := svc.Detail(ctx, ord.ID)
	if medio.Status != domain.StatusAbierta {
		t.Fatalf("estado = %s, quiere abierta: faltan 2 alitas", medio.Status)
	}

	if err := svc.DeliverLine(ctx, ord.ID, alitas, decimal.RequireFromString("2")); err != nil {
		t.Fatalf("entregar las 2 que faltaban: %v", err)
	}
	final, _ := svc.Detail(ctx, ord.ID)
	if final.Status != domain.StatusEntregada {
		t.Errorf("estado = %s, quiere entregada: ya no falta nada", final.Status)
	}
}

// El caso común y el de un solo tap: se entrega todo junto.
func TestEntregarTodoCierraElPedidoYSusRenglones(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, _, _ := pedidoDeAlitas(t, st, svc, "todo")

	if err := svc.DeliverAll(ctx, ord.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}
	tras, _ := svc.Detail(ctx, ord.ID)
	if tras.Status != domain.StatusEntregada {
		t.Fatalf("estado = %s, quiere entregada", tras.Status)
	}
	// Los renglones tienen que quedar de acuerdo con el pedido: si no, la pantalla mostraría
	// comida pendiente de un pedido cerrado y nadie sabría cuál de los dos datos creer.
	for _, l := range tras.Lines {
		if !l.Delivered.Equal(l.Quantity) {
			t.Errorf("%s quedó en %s de %s entregado", l.ProductName, l.Delivered, l.Quantity)
		}
	}
}

// Un doble tap sobre "entregué 3" dejaría el renglón en 6 de 5 y cerraría el pedido con comida
// todavía en la freidora.
func TestNoSePuedeEntregarMasDeLoQueFalta(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, alitas, _ := pedidoDeAlitas(t, st, svc, "exceso")

	if err := svc.DeliverLine(ctx, ord.ID, alitas, decimal.RequireFromString("3")); err != nil {
		t.Fatalf("entregar 3: %v", err)
	}
	err := svc.DeliverLine(ctx, ord.ID, alitas, decimal.RequireFromString("3"))
	if !errors.Is(err, domain.ErrEntregaExcede) {
		t.Fatalf("entregar 3 mas de un renglon de 5 con 3 entregadas = %v, quiere ErrEntregaExcede", err)
	}

	tras, _ := svc.Detail(ctx, ord.ID)
	if got := tras.Lines[0].Delivered; !got.Equal(decimal.RequireFromString("3")) {
		t.Errorf("entregado quedó en %s, quiere 3: el rechazo no debe escribir", got)
	}
}

// Cancelar repone el stock de TODAS las líneas. Si algo ya salió a la calle, reponerlo le inventa
// al almacén comida que ya se comieron, y ese error no se ve hasta que falta producto.
func TestNoSeCancelaUnPedidoConProductosEntregados(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, alitas, _ := pedidoDeAlitas(t, st, svc, "cancelar")
	cajero := makeUser(t, st, "cajero_cancela", "cajero")

	if err := svc.DeliverLine(ctx, ord.ID, alitas, decimal.RequireFromString("1")); err != nil {
		t.Fatalf("entregar 1: %v", err)
	}
	err := svc.Cancel(ctx, ord.ID, cajero, "el cliente cambió de opinión")
	if !errors.Is(err, domain.ErrCancelarConEntregas) {
		t.Fatalf("cancelar con 1 alita entregada = %v, quiere ErrCancelarConEntregas", err)
	}

	tras, _ := svc.Detail(ctx, ord.ID)
	if tras.Status != domain.StatusAbierta {
		t.Errorf("estado = %s: el pedido no debió cancelarse", tras.Status)
	}
}

// Un pedido del que no ha salido nada sí se cancela: es el flujo de siempre y no se rompe.
func TestUnPedidoSinEntregasSiSeCancela(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, _, _ := pedidoDeAlitas(t, st, svc, "cancelable")
	cajero := makeUser(t, st, "cajero_ok", "cajero")

	if err := svc.Cancel(ctx, ord.ID, cajero, "se equivocó de pedido"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	tras, _ := svc.Detail(ctx, ord.ID)
	if tras.Status != domain.StatusCancelada {
		t.Errorf("estado = %s, quiere cancelada", tras.Status)
	}
}

// El tablero necesita el avance para pintar "3 de 5 entregados" sin traerse los renglones de cada
// tarjeta. Si contara los cancelados, una tarjeta diría que falta comida que nadie va a hacer.
func TestElTableroTraeElAvanceDeEntrega(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, _, papas := pedidoDeAlitas(t, st, svc, "avance")

	if err := svc.DeliverLine(ctx, ord.ID, papas, decimal.RequireFromString("2")); err != nil {
		t.Fatalf("entregar papas: %v", err)
	}
	board, err := svc.Board(ctx)
	if err != nil {
		t.Fatalf("Board: %v", err)
	}
	var vista *app.BoardOrder
	for i := range board {
		if board[i].ID == ord.ID {
			vista = &board[i]
		}
	}
	if vista == nil {
		t.Fatal("el pedido no salió en el tablero")
	}
	if len(vista.Lines) != 2 {
		t.Fatalf("el tablero trajo %d renglones, quiere 2", len(vista.Lines))
	}
	// El tablero los trae DESPLEGADOS con lo que falta de cada uno: es lo que el operador vino a
	// leer, no algo que deba destapar con un tap.
	var completos int
	for _, l := range vista.Lines {
		if l.Delivered.GreaterThanOrEqual(l.Qty) {
			completos++
		}
	}
	if completos != 1 {
		t.Errorf("avance = %d de %d completos, quiere 1 de 2", completos, len(vista.Lines))
	}
	// Y con su nombre, o la tarjeta no sirve para preparar nada.
	for _, l := range vista.Lines {
		if l.Name == "" {
			t.Error("un renglón del tablero llegó sin nombre de producto")
		}
	}
}

// Cada pedido nace con su nombre para cantarlo en cocina, y ese nombre se GUARDA: es lo que va en
// el ticket y lo que el cliente usa para pedir su factura.
func TestCadaPedidoNaceConSuNombre(t *testing.T) {
	st := newTestStore(t)
	svc := app.NewOrdersService(st, clock)
	ord, _, _ := pedidoDeAlitas(t, st, svc, "folio")

	if ord.FolioName == "" {
		t.Fatal("el pedido nació sin nombre de folio")
	}
	tras, err := svc.Detail(context.Background(), ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if tras.FolioName != ord.FolioName {
		t.Errorf("al releer el folio es %q y al crear era %q", tras.FolioName, ord.FolioName)
	}
}

// Un pedido reembolsado SALIÓ de la cocina: solo se reembolsa lo que ya se entregó, y por eso
// reembolsar no repone stock. Sus renglones tienen que decirlo.
//
// La migración 0045 los dejó en cero —afirmando lo contrario de lo que pasó— y 0048 lo corrige en
// lo histórico; esto fija el comportamiento hacia adelante para que no vuelva a abrirse el hueco.
func TestUnPedidoReembolsadoTieneSusRenglonesEntregados(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	ord, _, _ := pedidoDeAlitas(t, st, svc, "reembolso")
	gerente := makeUser(t, st, "gerente_reemb", "gerente")

	if err := svc.DeliverAll(ctx, ord.ID); err != nil {
		t.Fatalf("DeliverAll: %v", err)
	}
	if err := svc.Refund(ctx, ord.ID, gerente, "producto mal"); err != nil {
		t.Fatalf("Refund: %v", err)
	}

	tras, err := svc.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	for _, l := range tras.Lines {
		if !l.Delivered.Equal(l.Quantity) {
			t.Errorf("%s quedó en %s de %s entregado: esa comida sí salió", l.ProductName, l.Delivered, l.Quantity)
		}
	}
}
