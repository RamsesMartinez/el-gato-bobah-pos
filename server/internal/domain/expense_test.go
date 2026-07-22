package domain

import "testing"

func TestExpenseTransitions(t *testing.T) {
	if !CanPayExpense(ExpensePendiente) {
		t.Error("pendiente debe poder pagarse")
	}
	for _, s := range []string{ExpensePagada, ExpenseCancelada, "otro"} {
		if CanPayExpense(s) {
			t.Errorf("no se debe poder pagar desde %q", s)
		}
	}
	if !CanCancelExpense(ExpensePendiente) {
		t.Error("pendiente debe poder cancelarse")
	}
	for _, s := range []string{ExpensePagada, ExpenseCancelada} {
		if CanCancelExpense(s) {
			t.Errorf("no se debe poder cancelar desde %q (terminal)", s)
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
