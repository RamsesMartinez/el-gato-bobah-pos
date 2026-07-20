package domain

import "math"

// Round2 redondea a 2 decimales (centavos), half-up. Se aplica en cada frontera de
// dinero para contener el drift de float64. Ceiling conocido: si la exactitud al
// centavo llega a fallar en sumas grandes, migrar las columnas numeric a centavos int64.
func Round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// Round4 redondea a 4 decimales, para cantidades de stock (columnas numeric(14,4) en base
// units: gramos/ml). El dinero usa Round2; usar Round2 aquí perdería precisión que el
// esquema sí soporta y rechazaría ajustes válidos de sub-centésima.
func Round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
