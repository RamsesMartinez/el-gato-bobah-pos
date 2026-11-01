package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/shopspring/decimal"
)

// Editar un gasto ya guardado (spec 053): las reglas que deciden el importe, el estado de pago, qué
// pasa cuando llega menos y cómo se revierte lo que entró. Todo puro: el servicio las aplica dentro
// de su transacción.

var (
	// ErrDecisionRequired: la vista previa pidió una decisión (conteo, dinero, lo que falta) y la
	// confirmación llegó sin ella.
	ErrDecisionRequired = fmt.Errorf("%w: falta decidir lo que marca la vista previa", ErrValidation)
	// ErrExpenseEditStale: desde la vista previa cambió algo que cambia el resultado (otra edición,
	// una entrega, una venta del mismo artículo). Se confirma solo lo que se vio.
	ErrExpenseEditStale = fmt.Errorf("%w: el gasto cambió desde la vista previa; revisa de nuevo", ErrConflict)
)

// ExpenseAmountAfterEdit es el importe después de una edición o una entrega (FR-009): el mayor entre
// lo que quedaba del importe (menos lo borrado y lo ajustado a lo recibido, más lo pagado por lo que
// llegó de más) y la suma de los renglones vivos. Así lo agregado ocupa primero lo que el importe
// tenía sin desglosar y solo lo que excede lo sube. Un resultado en cero se rechaza: quitar el gasto
// entero es anularlo.
func ExpenseAmountAfterEdit(before, removed, adjusted, extra, liveIncluded decimal.Decimal) (decimal.Decimal, error) {
	left := before.Sub(removed).Sub(adjusted).Add(extra)
	out := Round2(decimal.Max(left, liveIncluded))
	if !out.IsPositive() {
		return decimal.Zero, fmt.Errorf("%w: el gasto quedaría en $0; para quitarlo entero, anúlalo", ErrValidation)
	}
	if !ValidMoney(out, false) {
		return decimal.Zero, fmt.Errorf("%w: el importe del gasto pasaría del máximo", ErrValidation)
	}
	return out, nil
}

// PaymentStatusAfterEdit es el estado de pago después de una acción (FR-011). Solo se mueve si cambió
// el importe. Un gasto pagado sin ningún pago registrado es de antes de que existieran los pagos: su
// dinero sí salió, y volverlo deuda por editarlo lo pondría en cuentas por pagar sin deberse.
func PaymentStatusAfterEdit(current string, amountChanged bool, paid, amount decimal.Decimal, hasPayments bool) string {
	if !amountChanged || current == ExpenseCancelada {
		return current
	}
	if current == ExpensePagada && !hasPayments {
		return ExpensePagada
	}
	if paid.GreaterThanOrEqual(amount) {
		return ExpensePagada
	}
	return ExpensePendiente
}

// Shortfall es qué pasa con lo que no llegó de un renglón (FR-017).
type Shortfall string

const (
	// ShortfallPending: lo que falta sigue por llegar.
	ShortfallPending Shortfall = "pending"
	// ShortfallAdjust: lo que falta ya no llega y el importe baja a lo recibido.
	ShortfallAdjust Shortfall = "adjust"
)

// ParseShortfall acepta solo las dos salidas que existen.
func ParseShortfall(s string) (Shortfall, error) {
	switch v := Shortfall(s); v {
	case ShortfallPending, ShortfallAdjust:
		return v, nil
	}
	return "", fmt.Errorf("%w: qué hacer con lo que faltó es desconocido (%q)", ErrValidation, s)
}

// ShortfallDefault es la salida preseleccionada: lo que se compra por pieza o por empaque llega o no
// llega, así que lo que falta sigue pendiente; lo que se pesa o se mide casi siempre se cobró por lo
// que llegó.
func ShortfallDefault(buyKind string, byPresentation bool) Shortfall {
	if byPresentation || buyKind == "pieza" {
		return ShortfallPending
	}
	return ShortfallAdjust
}

// AdjustLineToReceived cierra un renglón en lo que llegó, con el mismo precio por unidad (FR-018).
func AdjustLineToReceived(qty, received, amount decimal.Decimal) (newQty, newAmount decimal.Decimal) {
	if !qty.IsPositive() {
		return received, amount
	}
	return Round4(received), Round2(amount.Mul(received).Div(qty))
}

// CountDecision es qué hacer cuando un conteo cerrado ya contó lo que esta acción movería (FR-022).
type CountDecision string

const (
	// CountAdjust: el movimiento mueve las existencias.
	CountAdjust CountDecision = "adjust"
	// CountKeep: el movimiento se registra y otro igual y contrario deja las existencias como las dejó
	// el conteo.
	CountKeep CountDecision = "keep"
)

// ParseCountDecision acepta solo las dos decisiones que existen.
func ParseCountDecision(s string) (CountDecision, error) {
	switch v := CountDecision(s); v {
	case CountAdjust, CountKeep:
		return v, nil
	}
	return "", fmt.Errorf("%w: decisión de conteo desconocida (%q)", ErrValidation, s)
}

// MoneyDecision es qué hacer cuando el importe sube por encima de lo pagado (FR-012).
type MoneyDecision string

const (
	// MoneyPay: después de confirmar se registra un pago con el saldo.
	MoneyPay MoneyDecision = "pay"
	// MoneyLeave: el gasto queda pendiente con su saldo.
	MoneyLeave MoneyDecision = "leave"
)

// ParseMoneyDecision acepta solo las dos decisiones que existen.
func ParseMoneyDecision(s string) (MoneyDecision, error) {
	switch v := MoneyDecision(s); v {
	case MoneyPay, MoneyLeave:
		return v, nil
	}
	return "", fmt.Errorf("%w: decisión de pago desconocida (%q)", ErrValidation, s)
}

// ValidDeliveryDate: una entrega no llega antes de la compra ni en el futuro (FR-015). Las tres son
// días del negocio.
func ValidDeliveryDate(day, expenseDate, today time.Time) error {
	if day.After(today) {
		return fmt.Errorf("%w: la entrega no puede ser de un día que no ha pasado", ErrValidation)
	}
	if day.Before(expenseDate) {
		return fmt.Errorf("%w: la entrega no puede ser anterior a la fecha del gasto", ErrValidation)
	}
	return nil
}

// DeliveryDay es lo que un renglón sumó al almacén en un día de entrega, y el último instante en que
// se registró algo de ese día.
type DeliveryDay struct {
	Day          time.Time
	Qty          decimal.Decimal
	RegisteredAt time.Time
}

// PlanDeliveryReversal devuelve los contra-movimientos de un renglón que se borra: uno por día de
// entrega, con su fecha, por lo que ese día dejó. Un día negativo (una corrección hecha después) se
// descuenta de las entregas más recientes primero. Nunca devuelve una cantidad positiva: un borrado
// que suma sería una compra que nadie hizo. La suma es exactamente −(lo aplicado) cuando lo aplicado
// es positivo.
func PlanDeliveryReversal(days []DeliveryDay) []DeliveryDay {
	pos := []DeliveryDay{}
	debt := decimal.Zero
	for _, x := range days {
		if x.Qty.IsPositive() {
			pos = append(pos, x)
		} else {
			debt = debt.Add(x.Qty.Neg())
		}
	}
	slices.SortStableFunc(pos, func(a, b DeliveryDay) int { return a.Day.Compare(b.Day) })
	for i := len(pos) - 1; i >= 0 && debt.IsPositive(); i-- {
		take := decimal.Min(pos[i].Qty, debt)
		pos[i].Qty = pos[i].Qty.Sub(take)
		debt = debt.Sub(take)
	}
	out := []DeliveryDay{}
	for _, x := range pos {
		if x.Qty.IsPositive() {
			out = append(out, DeliveryDay{Day: x.Day, Qty: Round4(x.Qty.Neg()), RegisteredAt: x.RegisteredAt})
		}
	}
	return out
}

// CountPosteriorCutoff es el instante después del cual un conteo ya vio lo que movió una entrega del
// día `day` registrada en `registeredAt` (R5): el conteo vio la mercancía si contó después de ese día,
// y vio el movimiento si contó después de registrarlo. Recibir hoy corta en ahora, así que ningún
// conteo cerrado queda después.
func CountPosteriorCutoff(registeredAt, day time.Time, loc *time.Location) time.Time {
	y, m, dd := day.Date()
	endOfDay := time.Date(y, m, dd, 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	if registeredAt.Before(endOfDay) {
		return registeredAt
	}
	return endOfDay
}

// PreviewFingerprint es la huella de una vista previa (R2): la confirmación manda la que vio y el
// servidor la recalcula. JSON ordena las llaves de los mapas, así que la huella no depende del orden.
func PreviewFingerprint(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		// Una vista previa siempre se serializa (la misma que se responde); si no, ninguna huella
		// coincide y la confirmación se rechaza en vez de escribir algo que no se vio.
		return "invalid"
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
