package domain

// Estados de pago de un gasto (cuentas por pagar).
const (
	ExpensePendiente = "pendiente"
	ExpensePagada    = "pagada"
	ExpenseCancelada = "cancelada"
)

// ValidExpenseStatus indica si s es un estado conocido (rechaza basura en la frontera).
func ValidExpenseStatus(s string) bool {
	return s == ExpensePendiente || s == ExpensePagada || s == ExpenseCancelada
}

// CanPayExpense: solo una pendiente se marca pagada. Que pagada→pagada sea false hace el pago
// idempotente y evita un segundo movimiento de caja por un doble-tap.
func CanPayExpense(status string) bool {
	return status == ExpensePendiente
}

// CanCancelExpense: solo una pendiente se cancela. Una pagada es terminal (el dinero ya salió);
// revertirla sería otra operación (reembolso de gasto), fuera de este flujo a propósito. Una
// pendiente con un pago ya dentro de un corte tampoco: una edición (spec 053) puede dejar pendiente
// un gasto pagado, y anularlo dejaría su salida contada en un corte de un gasto anulado. Un pago que
// todavía espera corte no lo impide: ningún corte lo toma después (spec 050).
func CanCancelExpense(status string, hasPaymentsInCount bool) bool {
	return status == ExpensePendiente && !hasPaymentsInCount
}
