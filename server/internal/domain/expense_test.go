package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestExpenseTransitions(t *testing.T) {
	if !CanPayExpense(ExpensePendiente) {
		t.Error("pendiente debe poder pagarse")
	}
	for _, s := range []string{ExpensePagada, ExpenseCancelada, "otro"} {
		if CanPayExpense(s) {
			t.Errorf("no se debe poder pagar desde %q", s)
		}
	}
	for _, s := range []string{ExpensePendiente, ExpensePagada, ExpenseCancelada} {
		if !ValidExpenseStatus(s) {
			t.Errorf("%q debe ser un estado válido", s)
		}
	}
	if ValidExpenseStatus("basura") {
		t.Error("un estado desconocido debe rechazarse")
	}
}

// #216: un gasto capturado por error se anula aunque esté pagado, siempre que ningún pago esté en un
// corte cerrado (eso ya es una devolución, #175). La regla de antes dejaba la propina del 2026-11-02
// contando en los reportes porque «pagada» era terminal.
func TestCanCancelAPaidExpenseCapturedByMistake(t *testing.T) {
	for s, want := range map[string]bool{ExpensePendiente: true, ExpensePagada: true, ExpenseCancelada: false, "otro": false} {
		if got := CanCancelExpense(s); got != want {
			t.Errorf("CanCancelExpense(%q) = %v; quería %v", s, got, want)
		}
	}
}

// El motivo es obligatorio: un gasto que desaparece de los reportes sin decir por qué no se puede
// auditar. Solo espacios es vacío, y uno enorme se rechaza en vez de truncarse.
func TestCancelReason(t *testing.T) {
	for _, bad := range []string{"", "   ", "\t\n", strings.Repeat("a", MaxCancelReasonLen+1)} {
		if _, err := CancelReason(bad); !errors.Is(err, ErrValidation) {
			t.Errorf("CancelReason(%q…) = %v; quería ErrValidation", bad[:min(len(bad), 10)], err)
		}
	}
	got, err := CancelReason("  era una propina  ")
	if err != nil || got != "era una propina" {
		t.Fatalf("CancelReason recortado = %q, %v", got, err)
	}
	if _, err := CancelReason(strings.Repeat("ñ", MaxCancelReasonLen)); err != nil {
		t.Fatalf("el tope cuenta letras, no bytes: %v", err)
	}
}

// Qué le pasa a cada pago al anular: el de un corte abierto sale del corte, el que espera corte deja
// de esperarlo (si no, la siguiente apertura lo descontaría), y el de un corte cerrado lo impide. Un
// pago en el arqueo cuya caja no se tomó entró después de leer los pagos: decidir sin su candado deja
// que una apertura lo barra a media anulación (revisión de seguridad y de base del #216).
func TestCancelPaymentPlan(t *testing.T) {
	cases := []struct {
		name      string
		state     PaymentCountState
		include   bool
		regLocked bool
		want      CancelPaymentAction
		wantErr   error
	}{
		{"corte abierto", PaymentInOpenCount, true, true, CancelPaymentLeaveOpenCount, nil},
		{"corte cerrado", PaymentInClosedCount, true, true, 0, ErrExpenseInClosedCount},
		{"espera corte", PaymentWithoutCount, true, true, CancelPaymentDropPending, nil},
		{"espera corte, caja sin tomar", PaymentWithoutCount, true, false, 0, ErrConflict},
		{"corte abierto, caja sin tomar", PaymentInOpenCount, true, false, 0, ErrConflict},
		{"fuera del arqueo", PaymentWithoutCount, false, false, CancelPaymentKeep, nil},
	}
	for _, c := range cases {
		got, err := CancelPaymentPlan(c.state, c.include, c.regLocked)
		if c.wantErr != nil {
			if !errors.Is(err, c.wantErr) {
				t.Errorf("%s: err = %v; quería %v", c.name, err, c.wantErr)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: = %v, %v; quería %v", c.name, got, err, c.want)
		}
	}
	if !errors.Is(ErrExpenseInClosedCount, ErrConflict) || !strings.Contains(ErrExpenseInClosedCount.Error(), "devolución") {
		t.Fatalf("el rechazo por corte cerrado debe ser 409 y decir que eso es una devolución: %v", ErrExpenseInClosedCount)
	}
	// Sacar un pago del arqueo mueve el esperado del cajón: es lo mismo que la casilla del arqueo.
	for a, want := range map[CancelPaymentAction]bool{CancelPaymentKeep: false, CancelPaymentLeaveOpenCount: true, CancelPaymentDropPending: true} {
		if got := a.MovesCount(); got != want {
			t.Errorf("%v.MovesCount() = %v; quería %v", a, got, want)
		}
	}
}
