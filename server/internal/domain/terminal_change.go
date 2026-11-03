package domain

import (
	"fmt"
	"strings"
)

// PaymentKindCard es el tipo de método (payment_methods.kind) del cobro con tarjeta: el único que
// tiene terminal.
const PaymentKindCard = "tarjeta"

// TerminalOption es una terminal activa de la sucursal entre las que se puede elegir.
type TerminalOption struct {
	ID   int64
	Name string
}

// TerminalChange es lo que se sabe de un pago, leído bajo el candado del pedido, y lo que pide la
// pantalla. From es la terminal que la pantalla vio: si ya no es la del pago, otro cambio ganó.
type TerminalChange struct {
	MethodKind  string
	OrderStatus string
	Current     *int64
	From        *int64
	To          int64
	Active      []TerminalOption
	Reason      string
}

// ErrPaymentChanged: la terminal que vio la pantalla ya no es la del pago. Sin esta comparación, el
// segundo de dos cambios simultáneos se aplicaría sobre uno que su autor nunca vio.
var ErrPaymentChanged = fmt.Errorf("%w: el pago ya cambió; vuelve a abrir la venta", ErrConflict)

// ValidateTerminalChange decide si se puede cambiar la terminal y devuelve la terminal destino
// (con el nombre de la lista de activas, nunca el de la pantalla) y el motivo sin espacios.
func ValidateTerminalChange(c TerminalChange) (TerminalOption, string, error) {
	if c.MethodKind != PaymentKindCard {
		return TerminalOption{}, "", fmt.Errorf("%w: solo un pago con tarjeta tiene terminal", ErrValidation)
	}
	if c.OrderStatus == StatusCancelada {
		return TerminalOption{}, "", fmt.Errorf("%w: la venta está cancelada", ErrConflict)
	}
	if !sameTerminal(c.Current, c.From) {
		return TerminalOption{}, "", ErrPaymentChanged
	}
	reason := strings.TrimSpace(c.Reason)
	if n := len([]rune(reason)); n < 3 || n > 200 {
		return TerminalOption{}, "", fmt.Errorf("%w: escribe el motivo (de 3 a 200 letras)", ErrValidation)
	}
	if c.Current != nil && *c.Current == c.To {
		return TerminalOption{}, "", fmt.Errorf("%w: el pago ya está en esa terminal", ErrValidation)
	}
	for _, t := range c.Active {
		if t.ID == c.To {
			return t, reason, nil
		}
	}
	return TerminalOption{}, "", fmt.Errorf("%w: esa terminal no está disponible en esta sucursal", ErrValidation)
}

func sameTerminal(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
