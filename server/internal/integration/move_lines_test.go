//go:build integration

package integration

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// moveOf arma un comando de «Pasar» con una pieza de cada posición del pedido origen.
func (s *splitTable) move(t *testing.T, to *int64, positions ...int) (*app.MoveLinesResult, error) {
	t.Helper()
	sel := make([]domain.SelectedPieces, len(positions))
	for i, p := range positions {
		sel[i] = domain.SelectedPieces{LineID: s.order.Lines[p].ID, Qty: s.order.Lines[p].Quantity}
	}
	return s.svc.MoveLines(s.ctx, app.MoveLinesCmd{ClientUUID: uuid.New(), FromOrderID: s.order.ID, ToOrderID: to, Lines: sel, ActorID: s.cashier})
}

func orderRow(t *testing.T, st *store.Store, id int64) (status string, merged *int64, session *int64, opener int64, service string, date string) {
	t.Helper()
	if err := st.Pool.QueryRow(context.Background(), `
		select status::text, merged_into_order_id, register_session_id, opened_by, service_type::text, business_date::text
		  from orders where id = $1`, id).Scan(&status, &merged, &session, &opener, &service, &date); err != nil {
		t.Fatal(err)
	}
	return
}

// LO QUE SE PASA VIAJA CON SU ESTADO DE COCINA Y SU INVENTARIO.
//
// Lo ya enviado a cocina no se vuelve a preparar (no hay comanda nueva) y las existencias no se
// mueven: el consumo viaja con el renglón. Recapturarlo, como se hizo el 2026-10-04, descontaba el
// inventario dos veces.
func TestMovedLinesKeepKitchenStateAndStock(t *testing.T) {
	st := newTestStore(t)
	s := newSplitTable(t, st, "pasa_cocina", "50", "60", "70")
	product := productOf(t, st, s.order.Lines[1].ID)
	before := existencias(t, st, product)
	var sentBefore string
	if err := st.Pool.QueryRow(context.Background(), `select enviado_a_cocina_at::text from order_lines where id = $1`, s.order.Lines[1].ID).Scan(&sentBefore); err != nil {
		t.Fatal(err)
	}
	res, err := s.move(t, nil, 1)
	if err != nil {
		t.Fatalf("MoveLines: %v", err)
	}
	if len(res.To.Lines) != 1 || res.To.Lines[0].ID != s.order.Lines[1].ID {
		t.Fatalf("el pedido nuevo tiene %+v; quiere el mismo renglón, movido y no recapturado", res.To.Lines)
	}
	var sentAfter string
	var movements int
	if err := st.Pool.QueryRow(context.Background(), `
		select (select enviado_a_cocina_at::text from order_lines where id = $1),
		       (select count(*) from stock_movements where order_line_id = $1 and order_id = $2)`,
		s.order.Lines[1].ID, res.To.ID).Scan(&sentAfter, &movements); err != nil {
		t.Fatal(err)
	}
	if sentAfter != sentBefore {
		t.Errorf("enviado a cocina %s → %s: se mandaría otra comanda", sentBefore, sentAfter)
	}
	if movements == 0 {
		t.Errorf("los movimientos del renglón no viajaron al pedido nuevo")
	}
	if got := existencias(t, st, product); !got.Equal(before) {
		t.Errorf("existencias %s → %s: pasar no mueve el almacén", before, got)
	}
	if !res.From.Total.Equal(pesos("120")) || !res.To.Total.Equal(pesos("60")) {
		t.Errorf("totales origen %s y destino %s; quiere 120 y 60", res.From.Total, res.To.Total)
	}
}

// EL PEDIDO NUEVO HEREDA EL TURNO DEL ORIGEN, Y QUIEN LOS CAPTURÓ.
//
// No pide «la caja abierta»: eso cerraría la puerta de más de una caja. Turno, día, servicio y quien
// abrió salen del origen; quien los pasó queda en el lote.
func TestTheNewOrderInheritsTheOriginShift(t *testing.T) {
	st := newTestStore(t)
	s := newSplitTable(t, st, "hereda_turno", "50", "60")
	mover := makeUser(t, st, "mesero_que_pasa", "mesero")
	res, err := s.svc.MoveLines(s.ctx, app.MoveLinesCmd{ClientUUID: uuid.New(), FromOrderID: s.order.ID,
		Lines: []domain.SelectedPieces{{LineID: s.order.Lines[1].ID, Qty: pesos("1")}}, ActorID: mover})
	if err != nil {
		t.Fatal(err)
	}
	_, _, fromSession, fromOpener, fromService, fromDate := orderRow(t, st, s.order.ID)
	_, _, toSession, toOpener, toService, toDate := orderRow(t, st, res.To.ID)
	if *toSession != *fromSession || toOpener != fromOpener || toService != fromService || toDate != fromDate {
		t.Fatalf("el nuevo: turno %d, abrió %d, %s, %s; el origen: %d, %d, %s, %s", *toSession, toOpener, toService, toDate,
			*fromSession, fromOpener, fromService, fromDate)
	}
	var movedBy int64
	if err := st.Pool.QueryRow(context.Background(), `select moved_by from order_line_move_batches where from_order_id = $1`, s.order.ID).Scan(&movedBy); err != nil {
		t.Fatal(err)
	}
	if movedBy != mover {
		t.Fatalf("el lote dice que pasó %d; quiere %d", movedBy, mover)
	}
}

// [C2] TRAS PASAR PARTE, ORIGEN Y DESTINO SE CIERRAN SOLOS SI YA NO LES FALTA NADA.
func TestAPartialMoveClosesBothOrdersWhenNothingIsLeft(t *testing.T) {
	st := newTestStore(t)
	o := newSplitTable(t, st, "cierra_los_dos", "50", "60")
	if err := o.svc.DeliverLine(o.ctx, o.order.ID, o.order.Lines[0].ID, pesos("1")); err != nil {
		t.Fatal(err)
	}
	// Se pasa lo entregado: el origen se queda con lo pendiente y el destino con lo entregado.
	res, err := o.move(t, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.To.Status != domain.StatusEntregada {
		t.Errorf("el destino recibió solo lo entregado y quedó %s; quiere entregada", res.To.Status)
	}
	if res.From.Status == domain.StatusEntregada {
		t.Errorf("al origen aún le falta un producto y quedó entregado")
	}
	if err := o.svc.DeliverLine(o.ctx, o.order.ID, o.order.Lines[1].ID, pesos("1")); err != nil {
		t.Fatal(err)
	}
	if status, _, _, _, _, _ := orderRow(t, st, o.order.ID); status != domain.StatusEntregada {
		t.Errorf("el origen, ya sin nada pendiente, quedó %s", status)
	}
}

// LOS RECHAZOS DE «PASAR», CADA UNO CON SU TEXTO.
func TestMoveRejections(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	cases := []struct {
		name string
		run  func(t *testing.T) error
		want error
	}{
		{"piezas pagadas", func(t *testing.T) error {
			s := newSplitTable(t, st, "rechazo_pagado", "50", "60")
			s.pay(t, 0)
			_, err := s.move(t, nil, 0)
			return err
		}, domain.ErrPieceAlreadyPaidToMove},
		{"todo hacia un pedido nuevo", func(t *testing.T) error {
			s := newSplitTable(t, st, "rechazo_todo_nuevo", "50", "60")
			_, err := s.move(t, nil, 0, 1)
			return err
		}, domain.ErrAlreadyItsOwnOrder},
		{"con descuento", func(t *testing.T) error {
			s := newSplitTable(t, st, "rechazo_descuento", "50", "60")
			ten := pesos("10")
			if _, err := s.svc.SetDiscount(s.ctx, app.SetDiscountCmd{OrderID: s.order.ID, Amount: &ten, Actor: s.cashier}); err != nil {
				t.Fatal(err)
			}
			_, err := s.move(t, nil, 0)
			return err
		}, domain.ErrMoveWithDiscount},
		{"destino igual al origen", func(t *testing.T) error {
			s := newSplitTable(t, st, "rechazo_mismo", "50", "60")
			_, err := s.move(t, &s.order.ID, 0)
			return err
		}, domain.ErrMoveTargetClosed},
		{"destino de otro día", func(t *testing.T) error {
			s := newSplitTable(t, st, "rechazo_otro_dia", "50", "60")
			d := newSplitTable(t, st, "rechazo_otro_dia_b", "50")
			if _, err := st.Pool.Exec(ctx, `update orders set business_date = business_date - 1 where id = $1`, d.order.ID); err != nil {
				t.Fatal(err)
			}
			_, err := s.move(t, &d.order.ID, 0)
			return err
		}, domain.ErrMoveTargetOtherShift},
		{"todo hacia uno existente con envío", func(t *testing.T) error {
			s := newSplitTable(t, st, "rechazo_envio", "50")
			d := newSplitTable(t, st, "rechazo_envio_b", "50")
			if _, err := st.Pool.Exec(ctx, `update orders set delivery_fee = 20, total = total + 20 where id = $1`, s.order.ID); err != nil {
				t.Fatal(err)
			}
			_, err := s.move(t, &d.order.ID, 0)
			return err
		}, domain.ErrMergeWithShipping},
		{"origen de un turno cerrado", func(t *testing.T) error {
			s := newSplitTable(t, st, "rechazo_turno", "50", "60")
			if _, err := st.Pool.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now() where status = 'abierta'`); err != nil {
				t.Fatal(err)
			}
			abrirCajaPrincipal(t, st, s.cashier)
			_, err := s.move(t, nil, 0)
			return err
		}, domain.ErrOrderFromClosedShiftToMove},
		{"plataforma", func(t *testing.T) error {
			s := newSplitTable(t, st, "rechazo_plataforma", "50", "60")
			didi := platformID(t, st, defaultCompanyID, "Didi")
			if _, err := st.Pool.Exec(ctx, `update orders set delivery_platform_id = $2, service_type = 'domicilio' where id = $1`, s.order.ID, didi); err != nil {
				t.Fatal(err)
			}
			_, err := s.move(t, nil, 0)
			return err
		}, domain.ErrPlatformOrderNotSplittable},
		{"dejaría el origen sobrepagado", func(t *testing.T) error {
			s := newSplitTable(t, st, "rechazo_sobrepagado", "50", "60")
			if _, err := s.svc.Charge(s.ctx, app.ChargeCmd{OrderID: s.order.ID, MethodID: s.cash, Amount: pesos("100"), ActorID: s.cashier}); err != nil {
				t.Fatal(err)
			}
			_, err := s.move(t, nil, 1)
			return err
		}, domain.ErrOrderWouldBeOverpaid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.run(t); !errors.Is(err, c.want) {
				t.Fatalf("= %v; quiere %v", err, c.want)
			}
		})
	}
}

// LA MISMA LLAVE ES EL MISMO LOTE: EL REENVÍO NO PASA DOS VECES, Y OTRO DESTINO SE RECHAZA.
func TestMoveIsIdempotentByBatch(t *testing.T) {
	st := newTestStore(t)
	s := newSplitTable(t, st, "pasa_idempotente", "50", "60", "70")
	key := uuid.New()
	cmd := app.MoveLinesCmd{ClientUUID: key, FromOrderID: s.order.ID, ActorID: s.cashier,
		Lines: []domain.SelectedPieces{{LineID: s.order.Lines[1].ID, Qty: pesos("1")}}}
	first, err := s.svc.MoveLines(s.ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.svc.MoveLines(s.ctx, cmd)
	if err != nil || again.To.ID != first.To.ID {
		t.Fatalf("reenvío = %v (destino %v); quiere el mismo pedido %d", err, again, first.To.ID)
	}
	var orders int
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from orders`).Scan(&orders); err != nil {
		t.Fatal(err)
	}
	if orders != 2 {
		t.Fatalf("hay %d pedidos; el reenvío creó otro", orders)
	}
	other := newSplitTable(t, st, "pasa_idempotente_b", "50")
	cmd.ToOrderID = &other.order.ID
	if _, err := s.svc.MoveLines(s.ctx, cmd); !errors.Is(err, domain.ErrMoveKeyMismatch) {
		t.Fatalf("misma llave, otro destino = %v; quiere «Esto ya se pasó a otro pedido»", err)
	}
}

// PASAR TODO A UN PEDIDO EXISTENTE JUNTA EL ORIGEN, SIN REPONER, Y NO ES UNA CANCELACIÓN.
//
// Caso D: se capturó en la cuenta equivocada. El origen vacío queda cancelado con el motivo fijo y
// marcado como juntado; el consumo viajó con los productos. Ni Ventas ni las ventas del turno lo
// cuentan como cancelación, y lo que se le quitó antes de juntarlo sí cuenta como producto
// cancelado.
func TestMovingEverythingMergesTheOriginAndIsNotACancellation(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	s := newSplitTable(t, st, "junta_todo", "50", "60", "70")
	d := newSplitTable(t, st, "junta_todo_destino", "40")
	// Se le quita uno antes de juntarlo: ese sí es producto cancelado.
	if _, err := s.svc.CancelarRenglon(s.ctx, s.order.ID, s.order.Lines[2].ID, s.cashier, "Ya no lo quiere"); err != nil {
		t.Fatal(err)
	}
	var moves int
	countMoves := func() int {
		if err := st.Pool.QueryRow(ctx, `select count(*) from stock_movements where movement_type <> 'venta'`).Scan(&moves); err != nil {
			t.Fatal(err)
		}
		return moves
	}
	restocksBefore := countMoves()
	res, err := s.move(t, &d.order.ID, 0, 1)
	if err != nil {
		t.Fatalf("MoveLines: %v", err)
	}
	status, merged, _, _, _, _ := orderRow(t, st, s.order.ID)
	if status != domain.StatusCancelada || merged == nil || *merged != d.order.ID {
		t.Fatalf("origen %s juntado con %v; quiere cancelada y juntado con %d", status, merged, d.order.ID)
	}
	if res.From.MergedIntoOrderID == nil || *res.From.MergedIntoOrderID != d.order.ID {
		t.Fatalf("la vista del origen dice juntado con %v", res.From.MergedIntoOrderID)
	}
	if got := countMoves(); got != restocksBefore {
		t.Fatalf("juntar repuso inventario (%d movimientos nuevos que no son venta)", got-restocksBefore)
	}
	var reason string
	if err := st.Pool.QueryRow(ctx, `select cancel_reason from orders where id = $1`, s.order.ID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "Se juntó con otro pedido" {
		t.Fatalf("motivo %q", reason)
	}

	sales := app.NewSalesService(st, clock)
	day, err := time.Parse("2006-01-02", businessDateOf(t, st, s.order.ID))
	if err != nil {
		t.Fatal(err)
	}
	summary, err := sales.Summary(ctx, domain.SalesFilter{Range: domain.Range{From: day, To: day}})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.Cancelled.Count != 0 {
		t.Errorf("Ventas cuenta %d pedidos cancelados; el juntado no es una cancelación", summary.Cancelled.Count)
	}
	if summary.CancelledLines.Count != 1 {
		t.Errorf("Ventas cuenta %d productos cancelados; quiere 1, el que se quitó antes de juntar", summary.CancelledLines.Count)
	}
	page, err := sales.List(ctx, domain.SalesFilter{Range: domain.Range{From: day, To: day}, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range page.Items {
		if row.ID == s.order.ID {
			t.Errorf("la lista de Ventas trae el pedido juntado: lista y resumen tienen que salir del mismo predicado")
		}
	}
	session := openSession(t, st)
	detail, err := app.NewBackofficeService(st, clock).SessionDetail(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	for _, sale := range detail.Sales {
		if sale.ID == s.order.ID {
			t.Errorf("las ventas del turno listan el pedido juntado (%+v)", sale)
		}
	}
	if detail.SalesCount != len(detail.Sales) {
		t.Errorf("las ventas del turno cuentan %d y listan %d: lista y conteo no salen del mismo predicado", detail.SalesCount, len(detail.Sales))
	}
}

// PASAR: AISLADO EN LOS TRES CASOS, Y SU PERMISO.
func TestMoveLinesPermissionAndIsolation(t *testing.T) {
	st := newTestStore(t)
	other := makeCompany(t, st, "ajena-pasar")
	s := newSplitTable(t, st, "pasar_aislado", "50", "60")
	before := splitBillFingerprint(t, st, s.order.ID)
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st2 *store.Store, c context.Context) {
		_, err := app.NewOrdersService(st2, clock).MoveLines(c, app.MoveLinesCmd{ClientUUID: uuid.New(), FromOrderID: s.order.ID,
			Lines: []domain.SelectedPieces{{LineID: s.order.Lines[0].ID, Qty: pesos("1")}}, ActorID: s.cashier})
		if err == nil {
			t.Fatal("pasó productos de la dueña")
		}
		if after := splitBillFingerprint(t, st, s.order.ID); after != before {
			t.Fatalf("cambió el pedido de la dueña:\nantes   %s\ndespués %s", before, after)
		}
	})

	none := func(domain.Role) []domain.Permission { return nil }
	r, token := ordersAPI(t, st, httpapi.PermissionResolver(none))
	_, tok := token("http_sin_permiso_pasar", "cajero")
	w := do(t, r, http.MethodPost, "/api/v1/orders/"+strconv.FormatInt(s.order.ID, 10)+"/lines/move", tok,
		[]byte(`{"clientUuid":"`+uuid.New().String()+`","lines":[{"lineId":`+strconv.FormatInt(s.order.Lines[0].ID, 10)+`,"qty":"1"}]}`), "application/json")
	if w.Code != http.StatusForbidden || !containsMessage(w.Body.Bytes(), "Tu usuario no puede pasar productos") {
		t.Fatalf("= %d %s; quiere 403 «Tu usuario no puede pasar productos»", w.Code, w.Body.String())
	}
}

func businessDateOf(t *testing.T, st *store.Store, orderID int64) string {
	t.Helper()
	var d string
	if err := st.Pool.QueryRow(context.Background(), `select business_date::text from orders where id = $1`, orderID).Scan(&d); err != nil {
		t.Fatal(err)
	}
	return d
}
