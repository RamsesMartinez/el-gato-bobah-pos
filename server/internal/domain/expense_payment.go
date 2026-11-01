package domain

import (
	"fmt"
	"time"
)

var (
	// ErrExpenseOutsideCount: un pago en efectivo de una caja que se cuenta no entraría a ningún
	// corte (la caja no tiene turno, o el pago es de otro día). Guardarlo así es válido —es como se
	// capturan gastos pasados— pero solo con la confirmación explícita de quien captura: en silencio,
	// el efectivo salió de una caja que se arquea y ningún arqueo lo ve.
	ErrExpenseOutsideCount = fmt.Errorf("%w: este gasto no entrará a ningún corte", ErrConflict)
	// ErrRegisterNotCounted: se intentó abrir turno en una caja que no lleva arqueo.
	ErrRegisterNotCounted = fmt.Errorf("%w: esta caja no lleva arqueo", ErrConflict)
	// ErrRegisterHasOpenShift: se intentó quitar el arqueo a una caja con turno abierto, que
	// dejaría un corte a medias.
	ErrRegisterHasOpenShift = fmt.Errorf("%w: cierra el turno de esta caja primero", ErrConflict)
	// ErrPrimaryRegisterCounts: la caja principal vende en efectivo; sin arqueo sus ventas no
	// tendrían corte.
	ErrPrimaryRegisterCounts = fmt.Errorf("%w: la caja principal siempre lleva arqueo", ErrConflict)
)

// PaymentRegister es lo que importa de una caja para decidir a dónde va un pago de gasto.
type PaymentRegister struct {
	CountsCash bool
	// OpenShiftDay es el día de negocio del turno abierto; nil si la caja está cerrada.
	OpenShiftDay *time.Time
}

// ExpensePaymentTarget dice si un pago de gasto entra al turno abierto de su caja.
//
// Efectivo entra solo si su caja se cuenta, tiene turno y el pago es del día de ese turno: un pago
// de otro día no salió del cajón que hoy se va a contar. Cualquier otro efectivo de una caja que se
// cuenta queda fuera de corte, y eso exige outsideCount. Tarjeta y transferencia no mueven el cajón:
// se atribuyen al turno abierto si lo hay, como siempre, y si no quedan fuera sin preguntar.
func ExpensePaymentTarget(isCash bool, reg *PaymentRegister, paidOn time.Time, outsideCount bool) (bool, error) {
	if reg == nil {
		if isCash {
			return false, ErrPaymentNeedsRegister
		}
		return false, nil
	}
	if !isCash {
		return reg.OpenShiftDay != nil, nil
	}
	if !reg.CountsCash {
		return false, nil
	}
	if reg.OpenShiftDay != nil && reg.OpenShiftDay.Equal(paidOn) {
		return true, nil
	}
	if outsideCount {
		return false, nil
	}
	return false, ErrExpenseOutsideCount
}
