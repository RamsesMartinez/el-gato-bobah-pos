//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

func paymentMethodID(t *testing.T, st *store.Store, name string) int16 {
	t.Helper()
	var id int16
	if err := st.Pool.QueryRow(context.Background(),
		`select id from payment_methods where name = $1`, name).Scan(&id); err != nil {
		t.Fatalf("paymentMethodID(%s): %v", name, err)
	}
	return id
}

// registerID resuelve el id de una caja sembrada (0026): "Caja principal" (primaria) / "Caja fuerte".
func registerID(t *testing.T, st *store.Store, name string) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(),
		`select id from cash_registers where name = $1`, name).Scan(&id); err != nil {
		t.Fatalf("registerID(%s): %v", name, err)
	}
	return id
}

// Un método marcado auto_declare cierra SIEMPRE con declarado = esperado del servidor: el
// valor que el cliente mande al cerrar caja para ese método se ignora. Cubre de punta a
// punta (DB → servicio → persistencia) el control central de domain.ResolveDeclared, que
// evita que un front comprometido/con bug subdeclare un método que nunca requirió conteo.
func TestCloseSessionAutoDeclareIgnoresClientValue(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordersSvc := app.NewOrdersService(st, clock)
	backoffice := app.NewBackofficeService(st, clock)

	cashier := makeUser(t, st, "cajero_auto", "cajero")
	prod := makeProduct(t, st, "Café", decimal.RequireFromString("80"), false)
	cardID := paymentMethodID(t, st, "Tarjeta débito")

	if _, err := backoffice.SetPaymentMethodAutoDeclare(ctx, int(cardID), true); err != nil {
		t.Fatalf("SetPaymentMethodAutoDeclare: %v", err)
	}
	primaryID := registerID(t, st, "Caja principal")
	if _, err := backoffice.OpenSession(ctx, primaryID, decimal.Zero, cashier); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	// Venta de 80 con tarjeta (método auto_declare).
	if _, err := ordersSvc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cashier,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:    []app.PaymentInput{{MethodID: cardID, Amount: decimal.RequireFromString("80")}},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Cierre con un declarado FALSEADO (1, muy por debajo del esperado) para ese método.
	declared := map[int]decimal.Decimal{int(cardID): decimal.RequireFromString("1")}
	sess, err := backoffice.CloseSession(ctx, primaryID, cashier, declared, "")
	if err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	var got *app.MethodTotal
	for i := range sess.Totals {
		if sess.Totals[i].MethodID == int(cardID) {
			got = &sess.Totals[i]
		}
	}
	if got == nil {
		t.Fatal("no hay total para el método de tarjeta en la respuesta")
	}
	if !got.Declared.Equal(decimal.RequireFromString("80")) {
		t.Fatalf("declared = %s, want 80 (esperado del servidor, no el 1 falseado por el cliente)", got.Declared)
	}
	if !got.Difference.IsZero() {
		t.Fatalf("difference = %s, want 0", got.Difference)
	}

	// Lo persistido en DB también refleja el esperado, no el valor falseado del cliente.
	var declaredDB, expectedDB decimal.Decimal
	if err := st.Pool.QueryRow(ctx,
		`select declared, expected from register_session_totals where session_id = $1 and payment_method_id = $2`,
		sess.ID, cardID).Scan(&declaredDB, &expectedDB); err != nil {
		t.Fatalf("leer register_session_totals: %v", err)
	}
	if !declaredDB.Equal(decimal.RequireFromString("80")) {
		t.Fatalf("declared persistido = %s, want 80", declaredDB)
	}
}

// Efectivo es justo el método que exige conteo físico: no debe poder marcarse auto_declare,
// o el corte de caja perdería la única forma de detectar un faltante de efectivo.
func TestSetPaymentMethodAutoDeclareRejectsCashDrawer(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)

	cashID := paymentMethodID(t, st, "Efectivo")

	_, err := backoffice.SetPaymentMethodAutoDeclare(ctx, int(cashID), true)
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("SetPaymentMethodAutoDeclare(Efectivo, true) = %v, want ErrValidation", err)
	}

	// Marcarlo false (el default) sigue permitido — el rechazo es solo al prender auto en
	// un método que afecta el cajón.
	if _, err := backoffice.SetPaymentMethodAutoDeclare(ctx, int(cashID), false); err != nil {
		t.Fatalf("SetPaymentMethodAutoDeclare(Efectivo, false): %v", err)
	}
}

// Un traspaso entre dos cajas abiertas genera, en un solo tx, la salida en la caja origen y la
// entrada en la destino, ambas ligadas al mismo cash_transfer: cada caja "detecta" el movimiento
// automáticamente y el neto es simétrico (−monto en origen, +monto en destino).
func TestTransferBetweenRegistersDetectedInBoth(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)

	cashier := makeUser(t, st, "cajero_traspaso", "cajero")
	primaryID := registerID(t, st, "Caja principal")
	safeID := registerID(t, st, "Caja fuerte")

	if _, err := backoffice.OpenSession(ctx, primaryID, decimal.RequireFromString("1000"), cashier); err != nil {
		t.Fatalf("OpenSession(principal): %v", err)
	}
	if _, err := backoffice.OpenSession(ctx, safeID, decimal.Zero, cashier); err != nil {
		t.Fatalf("OpenSession(fuerte): %v", err)
	}

	if _, err := backoffice.Transfer(ctx, primaryID, safeID, decimal.RequireFromString("300"), "guardar excedente", cashier); err != nil {
		t.Fatalf("Transfer: %v", err)
	}

	// Origen: salida 300, neto −300.
	from, err := backoffice.CurrentByRegister(ctx, primaryID)
	if err != nil || from == nil {
		t.Fatalf("CurrentByRegister(principal): %v (nil=%v)", err, from == nil)
	}
	if !from.NetMovements.Equal(decimal.RequireFromString("-300")) {
		t.Fatalf("neto principal = %s, want -300", from.NetMovements)
	}
	if len(from.Movements) != 1 || from.Movements[0].Kind != domain.CashSalida || from.Movements[0].TransferID == nil {
		t.Fatalf("movimiento origen inesperado: %+v", from.Movements)
	}

	// Destino: entrada 300, neto +300.
	to, err := backoffice.CurrentByRegister(ctx, safeID)
	if err != nil || to == nil {
		t.Fatalf("CurrentByRegister(fuerte): %v (nil=%v)", err, to == nil)
	}
	if !to.NetMovements.Equal(decimal.RequireFromString("300")) {
		t.Fatalf("neto fuerte = %s, want 300", to.NetMovements)
	}
	if len(to.Movements) != 1 || to.Movements[0].Kind != domain.CashEntrada || to.Movements[0].TransferID == nil {
		t.Fatalf("movimiento destino inesperado: %+v", to.Movements)
	}
	// Ambas piernas apuntan al mismo traspaso.
	if *from.Movements[0].TransferID != *to.Movements[0].TransferID {
		t.Fatalf("las piernas del traspaso no comparten transfer_id: %d vs %d", *from.Movements[0].TransferID, *to.Movements[0].TransferID)
	}
}

// Un traspaso exige que AMBAS cajas estén abiertas: con la destino cerrada, ErrConflict y sin
// escribir ningún movimiento (nada que "detectar" a medias).
func TestTransferRequiresBothRegistersOpen(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)

	cashier := makeUser(t, st, "cajero_traspaso2", "cajero")
	primaryID := registerID(t, st, "Caja principal")
	safeID := registerID(t, st, "Caja fuerte")

	if _, err := backoffice.OpenSession(ctx, primaryID, decimal.RequireFromString("500"), cashier); err != nil {
		t.Fatalf("OpenSession(principal): %v", err)
	}
	// Caja fuerte NO abierta.
	if _, err := backoffice.Transfer(ctx, primaryID, safeID, decimal.RequireFromString("100"), "", cashier); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("Transfer con destino cerrada = %v, want ErrConflict", err)
	}
	var n int
	if err := st.Pool.QueryRow(ctx, `select count(*) from register_cash_movements`).Scan(&n); err != nil {
		t.Fatalf("count movements: %v", err)
	}
	if n != 0 {
		t.Fatalf("se escribieron %d movimientos en un traspaso fallido, want 0", n)
	}
}

// Una venta con propina entra al corte: el esperado del método incluye la propina y el resumen
// jerárquico la muestra como línea "Propinas" separada de "Ventas".
func TestTipFlowsIntoCorte(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ordersSvc := app.NewOrdersService(st, clock)
	backoffice := app.NewBackofficeService(st, clock)

	cashier := makeUser(t, st, "cajero_tip", "cajero")
	prod := makeProduct(t, st, "Latte tip", decimal.RequireFromString("100"), false)
	cashID := paymentMethodID(t, st, "Efectivo")
	primaryID := registerID(t, st, "Caja principal")

	if _, err := backoffice.OpenSession(ctx, primaryID, decimal.Zero, cashier); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	// Venta de 100 en efectivo con 15 de propina.
	if _, err := ordersSvc.Create(ctx, app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cashier,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:    []app.PaymentInput{{MethodID: cashID, Amount: decimal.RequireFromString("100"), Tip: decimal.RequireFromString("15")}},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	view, err := backoffice.CurrentByRegister(ctx, primaryID)
	if err != nil || view == nil {
		t.Fatalf("CurrentByRegister: %v (nil=%v)", err, view == nil)
	}
	// Esperado del efectivo = fondo 0 + ventas 100 + propina 15 = 115.
	var cash *app.MethodTotal
	for i := range view.Totals {
		if view.Totals[i].Name == "Efectivo" {
			cash = &view.Totals[i]
		}
	}
	if cash == nil || !cash.Expected.Equal(decimal.RequireFromString("115")) {
		t.Fatalf("esperado efectivo = %v, want 115", cash)
	}
	if !cash.Tips.Equal(decimal.RequireFromString("15")) {
		t.Fatalf("propinas efectivo = %s, want 15", cash.Tips)
	}
	// El breakdown separa Ventas 100 de Propinas 15 bajo Efectivo.
	var ventas, propinas string
	for _, m := range view.Breakdown.Ingresos {
		if m.Method != "Efectivo" {
			continue
		}
		for _, it := range m.Items {
			switch it.Concept {
			case "Ventas":
				ventas = it.Amount.String()
			case "Propinas":
				propinas = it.Amount.String()
			}
		}
	}
	if ventas != "100" || propinas != "15" {
		t.Fatalf("breakdown efectivo: ventas=%q propinas=%q, want 100 / 15", ventas, propinas)
	}
}

// El resumen del corte incluye los gastos atribuidos (sección "Gastos") y la salida de efectivo
// del gasto trae expenseId (el front la excluye de la tabla de movimientos para no duplicar).
func TestSessionSummaryIncludesExpenses(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)

	admin := makeUser(t, st, "admin_corte", "admin")
	primaryID := registerID(t, st, "Caja principal")
	cashID := paymentMethodID(t, st, "Efectivo")
	var catID int64
	if err := st.Pool.QueryRow(ctx,
		`insert into expense_categories (name, financial_group) values ('Insumos corte', 'operacional') returning id`).Scan(&catID); err != nil {
		t.Fatalf("categoria: %v", err)
	}

	sess, err := backoffice.OpenSession(ctx, primaryID, decimal.Zero, admin)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	if _, err := backoffice.CreateExpense(ctx, app.ExpenseInput{
		CategoryID: catID, Amount: decimal.RequireFromString("120"),
		Status: domain.ExpensePagada, MethodID: &cashID, RegisterID: &primaryID, UserID: admin,
	}); err != nil {
		t.Fatalf("CreateExpense: %v", err)
	}

	detail, err := backoffice.SessionDetail(ctx, sess.ID)
	if err != nil {
		t.Fatalf("SessionDetail: %v", err)
	}
	if len(detail.Expenses) != 1 || !detail.Expenses[0].Amount.Equal(decimal.RequireFromString("120")) {
		t.Fatalf("gastos del corte = %+v, want 1 de 120", detail.Expenses)
	}
	withExpense := 0
	for _, m := range detail.Movements {
		if m.ExpenseID != nil {
			withExpense++
		}
	}
	if withExpense != 1 {
		t.Fatalf("movimientos con expenseId = %d, want 1", withExpense)
	}
}

// Todo gasto PAGADO exige una caja abierta: sin caja abierta, ErrConflict (ya no hay fallback de
// petty cash). Con la caja abierta y pago en efectivo, el gasto se liga a esa sesión y genera la
// salida del cajón.
func TestExpensePaidRequiresOpenRegister(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)

	admin := makeUser(t, st, "admin_gasto", "admin")
	primaryID := registerID(t, st, "Caja principal")
	cashID := paymentMethodID(t, st, "Efectivo")
	var catID int64
	if err := st.Pool.QueryRow(ctx,
		`insert into expense_categories (name, financial_group) values ('Insumos test', 'operacional') returning id`).Scan(&catID); err != nil {
		t.Fatalf("categoria: %v", err)
	}

	in := app.ExpenseInput{
		CategoryID: catID, Amount: decimal.RequireFromString("150"),
		Status: domain.ExpensePagada, MethodID: &cashID, RegisterID: &primaryID, UserID: admin,
	}
	// Sin caja abierta → ErrConflict.
	if _, err := backoffice.CreateExpense(ctx, in); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("CreateExpense sin caja abierta = %v, want ErrConflict", err)
	}

	// Con caja abierta → ok + salida de efectivo ligada al gasto.
	if _, err := backoffice.OpenSession(ctx, primaryID, decimal.Zero, admin); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	id, err := backoffice.CreateExpense(ctx, in)
	if err != nil {
		t.Fatalf("CreateExpense con caja abierta: %v", err)
	}
	var moves int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from register_cash_movements where expense_id = $1 and kind = 'salida'`, id).Scan(&moves); err != nil {
		t.Fatalf("count movements: %v", err)
	}
	if moves != 1 {
		t.Fatalf("salidas de efectivo del gasto = %d, want 1", moves)
	}
}
