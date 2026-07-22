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
// revertirla sería otra operación (reembolso de gasto), fuera de este flujo a propósito.
func CanCancelExpense(status string) bool {
	return status == ExpensePendiente
}
