package domain

import (
	"fmt"
	"sort"

	"github.com/shopspring/decimal"
)

// Errores de la entrega de propinas. Los mensajes los ve quien opera la caja.
var (
	ErrTipExceedsPending  = fmt.Errorf("%w: no hay tanta propina por entregar", ErrValidation)
	ErrWholePesosOnly     = fmt.Errorf("%w: la propina se entrega en pesos enteros", ErrValidation)
	ErrSplitBelowOnePeso  = fmt.Errorf("%w: no alcanza un peso entero para cada persona", ErrValidation)
	ErrDuplicateRecipient = fmt.Errorf("%w: una persona aparece dos veces en el reparto", ErrValidation)
	ErrNoRecipients       = fmt.Errorf("%w: elige al menos a una persona", ErrValidation)
	ErrTooManyRecipients  = fmt.Errorf("%w: son demasiadas personas para un solo reparto", ErrValidation)
	ErrTipsDecisionNeeded = fmt.Errorf("%w: queda propina por entregar: elige si se entrega ahora o se queda en caja", ErrValidation)
)

// MaxTipRecipients acota un reparto: cada persona es trabajo dentro de la transacción que tiene el
// turno bloqueado, y la caja no cobra mientras dura.
const MaxTipRecipients = 50

// PayoutLine is what one person receives in a tip payout.
type PayoutLine struct {
	UserID int64
	Amount decimal.Decimal
}

// SplitEven reparte el pendiente en pesos enteros por persona. Los centavos no se reparten
// (decisión del dueño, 2026-10-09): lo que no alcanza un peso por persona sigue pendiente para el
// siguiente reparto, en vez de entregarse a alguien en particular o perderse.
func SplitEven(pending decimal.Decimal, n int) (each, leftover decimal.Decimal, err error) {
	if n <= 0 {
		return decimal.Zero, decimal.Zero, ErrNoRecipients
	}
	each = pending.Div(decimal.NewFromInt(int64(n))).Floor()
	if each.LessThan(decimal.NewFromInt(1)) {
		return decimal.Zero, decimal.Zero, ErrSplitBelowOnePeso
	}
	return each, Round2(pending.Sub(each.Mul(decimal.NewFromInt(int64(n))))), nil
}

// ValidatePayout rechaza un reparto vacío, con personas repetidas, con centavos o que exceda lo
// pendiente.
func ValidatePayout(pending decimal.Decimal, lines []PayoutLine) error {
	if len(lines) == 0 {
		return ErrNoRecipients
	}
	seen := map[int64]bool{}
	total := decimal.Zero
	for _, l := range lines {
		if seen[l.UserID] {
			return ErrDuplicateRecipient
		}
		seen[l.UserID] = true
		if !ValidMoney(l.Amount, false) {
			return fmt.Errorf("%w: monto de propina inválido", ErrValidation)
		}
		if !l.Amount.Equal(l.Amount.Floor()) {
			return ErrWholePesosOnly
		}
		total = total.Add(l.Amount)
	}
	if total.GreaterThan(pending) {
		return ErrTipExceedsPending
	}
	return nil
}

// TipSource is tip money still available to hand out: one payment of the shift, or what the
// previous shift of the same register left in the drawer (Inherited, PaymentID 0).
type TipSource struct {
	PaymentID int64
	Cash      bool
	Inherited bool
	Available decimal.Decimal
}

// TipAllocation is how much of one source a payout used. PaymentID 0 = inherited.
type TipAllocation struct {
	PaymentID int64
	Amount    decimal.Decimal
}

// AllocateTipSources decide de qué cobros sale una entrega: primero lo heredado (es lo más viejo y
// ya está en el cajón), luego lo cobrado en efectivo y al final los demás medios, cada grupo en el
// orden recibido (el de cobro). Así la «propina de tarjeta pagada en efectivo» solo aparece cuando
// el efectivo de propinas ya no alcanza.
func AllocateTipSources(sources []TipSource, amount decimal.Decimal) ([]TipAllocation, error) {
	rank := func(s TipSource) int {
		switch {
		case s.Inherited:
			return 0
		case s.Cash:
			return 1
		}
		return 2
	}
	ordered := append([]TipSource(nil), sources...)
	sort.SliceStable(ordered, func(i, j int) bool { return rank(ordered[i]) < rank(ordered[j]) })
	out := []TipAllocation{}
	left := amount
	for _, s := range ordered {
		if !left.IsPositive() {
			break
		}
		if !s.Available.IsPositive() {
			continue
		}
		take := decimal.Min(s.Available, left)
		out = append(out, TipAllocation{PaymentID: s.PaymentID, Amount: take})
		left = left.Sub(take)
	}
	if left.IsPositive() {
		return nil, ErrTipExceedsPending
	}
	return out, nil
}

// PendingTips suma lo disponible de todas las fuentes.
func PendingTips(sources []TipSource) decimal.Decimal {
	t := decimal.Zero
	for _, s := range sources {
		if s.Available.IsPositive() {
			t = t.Add(s.Available)
		}
	}
	return Round2(t)
}

// TipsDecision is what the cashier chose at closing for the pending tips.
type TipsDecision string

const (
	TipsDecisionNone      TipsDecision = ""
	TipsDecisionHandOut   TipsDecision = "entregar"
	TipsDecisionKeepInBox TipsDecision = "quedan_en_caja"
)

// ParseTipsDecision acepta solo los valores conocidos; ausente es «sin decisión».
func ParseTipsDecision(s string) (TipsDecision, error) {
	switch TipsDecision(s) {
	case TipsDecisionNone, TipsDecisionHandOut, TipsDecisionKeepInBox:
		return TipsDecision(s), nil
	}
	return "", fmt.Errorf("%w: decisión de propinas desconocida", ErrValidation)
}

// TipsDecisionRequired: el cierre pregunta solo si hay al menos un peso entregable; un sobrante de
// centavos se hereda sin preguntar, porque no se puede entregar.
func TipsDecisionRequired(pending decimal.Decimal) bool {
	return pending.GreaterThanOrEqual(decimal.NewFromInt(1))
}
