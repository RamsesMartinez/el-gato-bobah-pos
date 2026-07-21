package domain

import "github.com/shopspring/decimal"

// El dinero es decimal exacto (shopspring/decimal), nunca float64: Postgres numeric es
// exacto y el escaneo va a decimal.Decimal (ver store.go). Así no hay drift ni 0.1+0.2.

// Round2 redondea a 2 decimales (centavos), half-up. Se aplica en cada frontera de dinero.
// ponytail: 2dp es correcto para las monedas soportadas hoy (MXN, USD, ambas 2 decimales).
// Si se agrega una moneda con otro exponente (JPY=0, JOD=3), redondea con Currency.Round.
func Round2(v decimal.Decimal) decimal.Decimal {
	return v.Round(2)
}

// Round4 redondea a 4 decimales, para cantidades de stock (numeric(14,4), base units:
// gramos/ml) y costos con precisión sub-centésima. El dinero usa Round2.
func Round4(v decimal.Decimal) decimal.Decimal {
	return v.Round(4)
}
