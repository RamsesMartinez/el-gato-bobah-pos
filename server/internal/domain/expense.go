package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

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

// CanCancelExpense decide si un gasto se puede anular por error (#216): pendiente o pagado. Lo que
// cada pago permite (un corte cerrado lo impide) lo decide CancelPaymentPlan.
func CanCancelExpense(status string) bool {
	return status == ExpensePendiente || status == ExpensePagada
}

// MaxCancelReasonLen es el tope del motivo, en letras: es una frase para la bitácora, no un documento.
const MaxCancelReasonLen = 300

// ErrExpenseInClosedCount: un pago del gasto ya está en un corte cerrado. Anular reescribiría un corte
// contado; lo que procede es registrar que el dinero regresó, que es una devolución (#175).
var ErrExpenseInClosedCount = fmt.Errorf("%w: un pago de este gasto ya está en un corte cerrado. Eso ya no se anula: es una devolución del gasto", ErrConflict)

// CancelReason valida el motivo de anular un gasto: obligatorio, porque el gasto sale de los reportes
// y la bitácora es lo único que dirá por qué. Devuelve el motivo sin espacios a los lados.
func CancelReason(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%w: escribe por qué se anula el gasto", ErrValidation)
	}
	if utf8.RuneCountInString(s) > MaxCancelReasonLen {
		return "", fmt.Errorf("%w: el motivo pasa de %d letras", ErrValidation, MaxCancelReasonLen)
	}
	return s, nil
}

// PaymentCountState es dónde está un pago de gasto respecto al arqueo, leído con su caja y su turno
// ya tomados.
type PaymentCountState int

const (
	PaymentWithoutCount PaymentCountState = iota
	PaymentInOpenCount
	PaymentInClosedCount
)

// CancelPaymentAction es lo que anular un gasto le hace a uno de sus pagos. El pago nunca se borra:
// es el registro de lo que se capturó; deja de contar porque su gasto queda anulado.
type CancelPaymentAction int

const (
	// CancelPaymentKeep: el pago no está en ningún arqueo; no hay nada que mover.
	CancelPaymentKeep CancelPaymentAction = iota + 1
	// CancelPaymentLeaveOpenCount: sale del corte abierto, con su salida de caja. El corte no se ha
	// contado; dejarla haría esperar en el cajón menos dinero del que hay.
	CancelPaymentLeaveOpenCount
	// CancelPaymentDropPending: deja de esperar corte. Sin esto la siguiente apertura lo barrería
	// (el barrido ya ignora gastos anulados; esto deja el pago coherente aunque cambie el barrido).
	CancelPaymentDropPending
)

// MovesCount dice si la acción cambia lo que un corte espera en el cajón: es lo mismo que cambiar la
// casilla del arqueo, y pide el mismo permiso.
func (a CancelPaymentAction) MovesCount() bool {
	return a == CancelPaymentLeaveOpenCount || a == CancelPaymentDropPending
}

// CancelPaymentPlan decide qué pasa con un pago al anular su gasto (#216). Un corte cerrado ya se
// contó y firmó: reescribirlo dejaría su arqueo sin cuadrar, y devolver ese dinero es otra operación
// (la devolución del #175). registerLocked dice si la caja del pago se tomó antes que el gasto; un pago
// en el arqueo sin ese candado entró después de leer los pagos, y una apertura podría barrerlo a media
// anulación y dejar su salida en un corte de un gasto anulado.
func CancelPaymentPlan(state PaymentCountState, includeInCount, registerLocked bool) (CancelPaymentAction, error) {
	if state == PaymentInClosedCount {
		return 0, ErrExpenseInClosedCount
	}
	if (state == PaymentInOpenCount || includeInCount) && !registerLocked {
		return 0, fmt.Errorf("%w: la caja de este gasto cambió mientras tanto; vuelve a intentar", ErrConflict)
	}
	if state == PaymentInOpenCount {
		return CancelPaymentLeaveOpenCount, nil
	}
	if includeInCount {
		return CancelPaymentDropPending, nil
	}
	return CancelPaymentKeep, nil
}
