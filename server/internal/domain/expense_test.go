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
	if !CanCancelExpense(ExpensePendiente, false) {
		t.Error("pendiente debe poder cancelarse")
	}
	for _, s := range []string{ExpensePagada, ExpenseCancelada} {
		if CanCancelExpense(s, false) {
			t.Errorf("no se debe poder cancelar desde %q (terminal)", s)
		}
	}
	// Spec 053 (revisión A1): una edición puede dejar pendiente un gasto pagado cuya salida ya está en
	// un corte. Anularlo dejaría ese corte contando la salida de un gasto anulado: el corte y el arqueo
	// dejan de cuadrar. Anular con devolución es la entrega 2.
	if CanCancelExpense(ExpensePendiente, true) {
		t.Error("un gasto pendiente con un pago ya en un corte no debe poder anularse")
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
