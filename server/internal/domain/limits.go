package domain

import "math"

// Cotas defensivas para montos y cantidades. NO son reglas de negocio finas (un café real
// nunca se acerca a estos números): existen para que una entrada absurda o maliciosa falle
// como 400 en vez de desbordar el numeric de Postgres (→ 500) o corromper el ledger con
// Inf/NaN o un wrap de int16. Los topes quedan por debajo del límite de cada columna:
//   - dinero:    numeric(10,2) → tope ~10^8; MaxMoney un orden por debajo.
//   - venta:     order_lines.quantity numeric(8,2) → ~10^6; y el modificador se guarda como
//     int16 (tope 32767) → MaxOrderQty muy por debajo de ambos.
//   - stock:     stock_movements.quantity numeric(14,4) → ~10^10.
const (
	MaxMoney    = 10_000_000.0 // pesos, por monto individual (caja, gasto, pago, total)
	MaxOrderQty = 10_000.0     // unidades por línea de venta / por modificador
	MaxStockQty = 1_000_000.0  // base units por movimiento de stock
)

// ValidMoney: monto finito, no negativo y ≤ MaxMoney. allowZero=false exige > 0 (un gasto
// o pago de 0 no tiene sentido); allowZero=true admite 0 (apertura de caja vacía, método
// sin ventas al cerrar). Pásale el valor ya redondeado (Round2) para que un sub-centavo
// que redondea a 0 se rechace igual que el 0 explícito.
func ValidMoney(v float64, allowZero bool) bool {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return false
	}
	if v < 0 || v > MaxMoney {
		return false
	}
	return allowZero || v > 0
}

// ValidQty: cantidad finita, distinta de 0 y con |v| ≤ max. allowNegative admite deltas
// negativos (mermas/ajustes de stock); las líneas de venta y modificadores exigen > 0.
func ValidQty(v, max float64, allowNegative bool) bool {
	if math.IsNaN(v) || math.IsInf(v, 0) || v == 0 {
		return false
	}
	if v > max || v < -max {
		return false
	}
	return allowNegative || v > 0
}
