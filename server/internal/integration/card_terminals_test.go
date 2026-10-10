//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

func branchOfPrincipal(t *testing.T, st *store.Store) int64 {
	t.Helper()
	var b int64
	if err := st.Pool.QueryRow(context.Background(),
		`select branch_id from cash_registers where is_primary and is_active limit 1`).Scan(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

func terminalDelPago(t *testing.T, st *store.Store, orderID int64) (*int64, *string) {
	t.Helper()
	var id *int64
	var name *string
	if err := st.Pool.QueryRow(context.Background(),
		`select card_terminal_id, card_terminal_name from order_payments where order_id = $1`, orderID).Scan(&id, &name); err != nil {
		t.Fatal(err)
	}
	return id, name
}

// TODO COBRO CON TARJETA GUARDA SU TERMINAL (punto 8). Con una sola terminal en la sucursal no hay
// nada que elegir; con varias, manda la del usuario; sin ella, se pide (EB-28, EB-29, EB-31).
func TestCardChargeRecordsTerminal(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	terms := app.NewTerminalsService(st)
	cajero := makeUser(t, st, "cajero_terminal", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	branch := branchOfPrincipal(t, st)
	debito := paymentMethodID(t, st, "Tarjeta débito")

	cobrar := func(nombre string, terminal *int64) (int64, error) {
		prod := makeProduct(t, st, nombre, dec("100"), false)
		ord := crearPedidoSimple(t, ctx, orders, prod, cajero)
		_, err := orders.Charge(ctx, app.ChargeCmd{OrderID: ord, MethodID: debito, Amount: dec("100"), ActorID: cajero, TerminalID: terminal})
		return ord, err
	}

	// Una sola terminal (la que nace con el negocio): se usa sola.
	ord, err := cobrar("Tarjeta una terminal", nil)
	if err != nil {
		t.Fatal(err)
	}
	if id, name := terminalDelPago(t, st, ord); id == nil || name == nil || *name != "Terminal" {
		t.Fatalf("el cobro no guardó la terminal: %v %v", id, name)
	}

	// Dos terminales y sin la del usuario: se pide.
	hey, err := terms.Create(ctx, branch, "Hey")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cobrar("Tarjeta sin elegir", nil); !errors.Is(err, domain.ErrCardTerminalRequired) {
		t.Fatalf("con dos terminales y sin elegir: err = %v", err)
	}
	// La del usuario llega sola.
	if err := terms.SetUserDefault(ctx, cajero, &hey.ID); err != nil {
		t.Fatal(err)
	}
	ord, err = cobrar("Tarjeta por omision", nil)
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := terminalDelPago(t, st, ord); id == nil || *id != hey.ID {
		t.Fatalf("no usó la terminal del usuario: %v", id)
	}
	// Archivada, la del usuario se ignora y se vuelve a pedir.
	if err := terms.Archive(ctx, hey.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := cobrar("Tarjeta archivada", nil); err != nil {
		t.Fatalf("con la del usuario archivada y una sola activa, debe usar la activa: %v", err)
	}
	// Efectivo no lleva terminal.
	prod := makeProduct(t, st, "Efectivo sin terminal", dec("50"), false)
	o := crearPedidoSimple(t, ctx, orders, prod, cajero)
	if _, err := orders.Charge(ctx, app.ChargeCmd{OrderID: o, MethodID: paymentMethodID(t, st, "Efectivo"), Amount: dec("50"), ActorID: cajero}); err != nil {
		t.Fatal(err)
	}
	if id, _ := terminalDelPago(t, st, o); id != nil {
		t.Fatal("un cobro en efectivo guardó terminal")
	}
}

// Una terminal de otra empresa no se puede usar para cobrar (EB-28).
func TestCardTerminalsIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	branch := branchOfPrincipal(t, st)
	if _, err := app.NewTerminalsService(st).Create(ctx, branch, "Solo de la dueña"); err != nil {
		t.Fatal(err)
	}
	otra := makeCompany(t, st, "otra-terminales")
	inTheThreeCases(t, defaultCompanyID, otra, func(t *testing.T, as *store.Store, ctx context.Context) {
		lista, err := app.NewTerminalsService(as).List(ctx)
		if err != nil {
			return
		}
		for _, x := range lista {
			if x.Name == "Solo de la dueña" {
				t.Fatal("se vio una terminal de otra empresa")
			}
		}
	})
}

// ARQUEO POR TERMINAL (punto 9): con el modo de la sucursal en «por terminal», el cierre pide el
// total de cada terminal que cobró y guarda la diferencia; el modo es el que había al abrir (EB-33).
func TestPerTerminalCountAtClose(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)
	terms := app.NewTerminalsService(st)
	cajero := makeUser(t, st, "cajero_arqueo_terminal", "cajero")
	branch := branchOfPrincipal(t, st)
	reg := principalRegister(t, st)
	if err := terms.SetCardCountMode(ctx, branch, "per_terminal"); err != nil {
		t.Fatal(err)
	}
	cero := dec("0")
	if _, err := back.OpenSession(ctx, reg, app.AperturaCmd{Total: &cero, Motivo: "prueba"}, cajero); err != nil {
		t.Fatal(err)
	}
	// Cambiar el modo con la caja abierta no cambia este turno.
	if err := terms.SetCardCountMode(ctx, branch, "auto"); err != nil {
		t.Fatal(err)
	}
	prod := makeProduct(t, st, "Tarjeta arqueo", dec("250"), false)
	ord := crearPedidoSimple(t, ctx, orders, prod, cajero)
	if _, err := orders.Charge(ctx, app.ChargeCmd{OrderID: ord, MethodID: paymentMethodID(t, st, "Tarjeta débito"), Amount: dec("250"), ActorID: cajero}); err != nil {
		t.Fatal(err)
	}
	entregarPendientes(t, st)
	var term int64
	_ = st.Pool.QueryRow(ctx, `select card_terminal_id from order_payments where order_id = $1`, ord).Scan(&term)

	if _, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &cero, Motivo: "prueba"}); !errors.Is(err, domain.ErrTerminalCountRequired) {
		t.Fatalf("cerrar sin el total de la terminal: err = %v", err)
	}
	v, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &cero, Motivo: "prueba",
		TerminalCounts: map[int64]decimal.Decimal{term: dec("240")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.TerminalCounts) != 1 || !v.TerminalCounts[0].Difference.Equal(dec("-10")) {
		t.Fatalf("diferencia por terminal = %+v; quería -10", v.TerminalCounts)
	}
}

// sinArqueoPorTerminal deja todas las sucursales en arqueo automático de tarjeta, para los casos
// que prueban otra cosa y cierran turnos con cobros con tarjeta.
func sinArqueoPorTerminal(t *testing.T, st *store.Store) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(), `update branches set card_count_mode = 'auto'`); err != nil {
		t.Fatal(err)
	}
}

// Una devolución con tarjeta en el mismo turno baja lo esperado de su terminal: sin eso el arqueo por
// terminal marcaba un faltante igual a lo devuelto, que la terminal ya regresó.
func TestCardRefundLowersTerminalExpected(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_term_dev", "admin")
	reg := principalRegister(t, st)
	if err := app.NewTerminalsService(st).SetCardCountMode(ctx, branchOfPrincipal(t, st), "per_terminal"); err != nil {
		t.Fatal(err)
	}
	cero := dec("0")
	if _, err := back.OpenSession(ctx, reg, app.AperturaCmd{Total: &cero, Motivo: "prueba"}, cajero); err != nil {
		t.Fatal(err)
	}
	ord := pedidoCobradoParcial(t, ctx, st, orders, "term dev", "300", "300", cajero, paymentMethodID(t, st, "Tarjeta débito"), false)
	if err := orders.Devolver(ctx, app.DevolucionCmd{OrderID: ord, Monto: dec("100"), Motivo: "prueba", ActorID: cajero, CardFolio: "F9"}); err != nil {
		t.Fatal(err)
	}
	entregarPendientes(t, st)
	var term int64
	_ = st.Pool.QueryRow(ctx, `select card_terminal_id from order_payments where order_id = $1`, ord).Scan(&term)
	v, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &cero, Motivo: "prueba",
		TerminalCounts: map[int64]decimal.Decimal{term: dec("200")}})
	if err != nil {
		t.Fatal(err)
	}
	if !v.TerminalCounts[0].Difference.IsZero() {
		t.Fatalf("DEVOLUCIÓN NO DESCONTADA de la terminal: esperado %s, diferencia %s", v.TerminalCounts[0].Expected, v.TerminalCounts[0].Difference)
	}
}
