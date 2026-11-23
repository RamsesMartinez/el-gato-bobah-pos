package domain

import (
	"math"

	"github.com/shopspring/decimal"
)

// ValidSelectRange dice si un mínimo y un máximo de selección caben en la columna (smallint) y tienen
// sentido: un máximo que no cabe da la vuelta y deja un grupo donde no se puede elegir nada.
func ValidSelectRange(min, max int) bool {
	return min >= 0 && max >= 1 && min <= max && max <= math.MaxInt16
}

// ValidMaxPerLine dice si el tope de un extra por renglón cabe en su columna (smallint).
func ValidMaxPerLine(n int) bool { return n >= 1 && n <= math.MaxInt16 }

// ValidPosition dice si el orden de un grupo en un producto cabe en su columna (integer).
func ValidPosition(n int) bool { return n >= 0 && n <= math.MaxInt32 }

// ValidPriceDelta dice si lo que suma un extra cabe en numeric(10,2) con centavos. Puede ser negativo
// («sin leche» resta), así que se mide su magnitud.
func ValidPriceDelta(d decimal.Decimal) bool {
	return ValidMoney(Round2(d).Abs(), true)
}
