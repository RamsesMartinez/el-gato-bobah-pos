package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// CashRetiro es lo que se lleva alguien al cerrar: lo contado menos el fondo que se deja (spec 035).
// No es una salida del negocio ni entra al esperado del cajón: se registra DESPUÉS de contar, así
// que restarlo del neto reportaría un sobrante igual al retiro. Toda suma de movimientos lo excluye.
const CashRetiro = "retiro"

var errFloatOverCounted = fmt.Errorf("%w: el fondo que se deja no puede ser mayor a lo contado", ErrValidation)

// Withdrawal es el retiro al cierre: lo contado menos el fondo que se deja, a centavos.
func Withdrawal(counted, floatLeft decimal.Decimal) (decimal.Decimal, error) {
	left := Round2(floatLeft)
	if !ValidMoney(left, true) {
		return decimal.Zero, fmt.Errorf("%w: el fondo que se deja no es una cantidad válida", ErrValidation)
	}
	if left.GreaterThan(Round2(counted)) {
		return decimal.Zero, errFloatOverCounted
	}
	return Round2(counted).Sub(left), nil
}

// ResolveFloatLeft decide cuánto se queda en el cajón. Lo que manda quien cierra gana y se valida;
// sin cifra, el fondo fijo de la caja. El fijo se recorta a lo contado en vez de rechazarse: quien
// no mandó cifra es una tableta con el front anterior, y rebotarle el cierre durante la subida la
// deja sin poder cerrar. Sin fijo ni cifra se deja todo lo contado, como antes de la spec 035.
func ResolveFloatLeft(sent, fixed *decimal.Decimal, counted decimal.Decimal) (decimal.Decimal, error) {
	if sent != nil {
		left := Round2(*sent)
		if _, err := Withdrawal(counted, left); err != nil {
			return decimal.Zero, err
		}
		return left, nil
	}
	if fixed != nil {
		return decimal.Min(Round2(*fixed), Round2(counted)), nil
	}
	return Round2(counted), nil
}
