package domain

import "math"

// Round2 redondea a 2 decimales (centavos), half-up. Se aplica en cada frontera de
// dinero para contener el drift de float64. Ceiling conocido: si la exactitud al
// centavo llega a fallar en sumas grandes, migrar las columnas numeric a centavos int64.
func Round2(v float64) float64 {
	return math.Round(v*100) / 100
}
