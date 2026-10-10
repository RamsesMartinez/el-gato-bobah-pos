//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// EL DÍA DEL GASTO ES EL DEL TURNO ABIERTO, NO EL DEL TICKET (spec 032, punto 5; EB-21, EB-22).
// Un ticket fechado la semana pasada que se paga hoy con la caja abierta es gasto de este turno; la
// fecha del ticket se guarda aparte para contabilidad.
func TestExpenseDayIsOpenShiftDayAndDocumentDateIsKeptApart(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	admin := makeUser(t, st, "admin_dia_gasto", "admin")
	var catID int64
	if err := st.Pool.QueryRow(ctx,
		`insert into expense_categories (name, financial_group) values ('Dia gasto', 'operacional') returning id`).Scan(&catID); err != nil {
		t.Fatal(err)
	}
	sess := abrirCajaPrincipal(t, st, admin)
	turno := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC) // el turno se abrió antes de medianoche, días atrás
	if _, err := st.Pool.Exec(ctx, `update register_sessions set business_date = $2 where id = $1`, sess, turno); err != nil {
		t.Fatal(err)
	}
	id, err := back.CreateExpense(ctx, app.ExpenseInput{
		ExpenseDate: "2026-07-10", // fecha del ticket
		CategoryID:  catID, Amount: dec("80"), Status: domain.ExpensePendiente, UserID: admin,
	})
	if err != nil {
		t.Fatal(err)
	}
	var dia, doc time.Time
	if err := st.Pool.QueryRow(ctx, `select expense_date, document_date from expenses where id = $1`, id).Scan(&dia, &doc); err != nil {
		t.Fatal(err)
	}
	if !dia.Equal(turno) {
		t.Fatalf("día del gasto = %s; quería el del turno abierto %s", dia.Format("2006-01-02"), turno.Format("2006-01-02"))
	}
	if doc.Format("2006-01-02") != "2026-07-10" {
		t.Fatalf("la fecha del documento se perdió: %s", doc)
	}
}

// Sin caja abierta, el día lo elige quien captura; si no elige, hoy.
func TestExpenseDayWithoutOpenShiftIsChosenOrToday(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	admin := makeUser(t, st, "admin_dia_sin_turno", "admin")
	var catID int64
	if err := st.Pool.QueryRow(ctx,
		`insert into expense_categories (name, financial_group) values ('Dia sin turno', 'operacional') returning id`).Scan(&catID); err != nil {
		t.Fatal(err)
	}
	elegido, err := back.CreateExpense(ctx, app.ExpenseInput{ExpenseDay: "2026-07-15", ExpenseDate: "2026-07-01",
		CategoryID: catID, Amount: dec("10"), Status: domain.ExpensePendiente, UserID: admin})
	if err != nil {
		t.Fatal(err)
	}
	hoy, err := back.CreateExpense(ctx, app.ExpenseInput{CategoryID: catID, Amount: dec("10"), Status: domain.ExpensePendiente, UserID: admin})
	if err != nil {
		t.Fatal(err)
	}
	var d1, d2 time.Time
	_ = st.Pool.QueryRow(ctx, `select expense_date from expenses where id = $1`, elegido).Scan(&d1)
	_ = st.Pool.QueryRow(ctx, `select expense_date from expenses where id = $1`, hoy).Scan(&d2)
	if d1.Format("2006-01-02") != "2026-07-15" || d2.Format("2006-01-02") != fixedNow.Format("2006-01-02") {
		t.Fatalf("elegido %s (quería 2026-07-15), sin elegir %s (quería hoy)", d1.Format("2006-01-02"), d2.Format("2006-01-02"))
	}
}

// Con turno abierto el día lo pone el turno: un día elegido a mano no se descarta en silencio, se
// rechaza (constitución V).
func TestExpenseDayChosenWithOpenShiftIsRejected(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	admin := makeUser(t, st, "admin_dia_rechazo", "admin")
	var catID int64
	if err := st.Pool.QueryRow(ctx,
		`insert into expense_categories (name, financial_group) values ('Dia rechazo', 'operacional') returning id`).Scan(&catID); err != nil {
		t.Fatal(err)
	}
	abrirCajaPrincipal(t, st, admin)
	if _, err := back.CreateExpense(ctx, app.ExpenseInput{ExpenseDay: "2026-07-01", CategoryID: catID, Amount: dec("10"),
		Status: domain.ExpensePendiente, UserID: admin}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("día elegido con turno abierto: err = %v", err)
	}
}
