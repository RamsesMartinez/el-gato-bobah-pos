package domain

import "github.com/shopspring/decimal"

// Cotas defensivas para montos y cantidades. NO son reglas de negocio finas (un café real
// nunca se acerca a estos números): existen para que una entrada absurda o maliciosa falle
// como 400 en vez de desbordar el numeric de Postgres (→ 500) o corromper el ledger con un
// wrap de int16. Los topes quedan por debajo del límite de cada columna:
//   - dinero:    numeric(10,2) → tope ~10^8; MaxMoney un orden por debajo.
//   - venta:     order_lines.quantity numeric(8,2) → ~10^6; y el modificador se guarda como
//     int16 (tope 32767) → MaxOrderQty muy por debajo de ambos.
//   - stock:     stock_movements.quantity numeric(14,4) → ~10^10.
var (
	MaxMoney    = decimal.NewFromInt(10_000_000) // pesos, por monto individual (caja, gasto, pago, total)
	MaxOrderQty = decimal.NewFromInt(10_000)     // unidades por línea de venta / por modificador
	MaxStockQty = decimal.NewFromInt(1_000_000)  // base units por movimiento de stock
)

// ValidMoney: monto no negativo y ≤ MaxMoney. allowZero=false exige > 0 (un gasto o pago de
// 0 no tiene sentido); allowZero=true admite 0 (apertura de caja vacía, método sin ventas al
// cerrar). Pásale el valor ya redondeado (Round2) para que un sub-centavo que redondea a 0 se
// rechace igual que el 0 explícito. decimal es exacto → no hay NaN/Inf que filtrar.
func ValidMoney(v decimal.Decimal, allowZero bool) bool {
	if !escalaSana(v) {
		return false
	}
	if v.IsNegative() || v.GreaterThan(MaxMoney) {
		return false
	}
	if allowZero {
		return true
	}
	return v.IsPositive()
}

// ValidQty: cantidad distinta de 0 y con |v| ≤ max. allowNegative admite deltas negativos
// (mermas/ajustes de stock); las líneas de venta y modificadores exigen > 0.
func ValidQty(v, max decimal.Decimal, allowNegative bool) bool {
	if !escalaSana(v) {
		return false
	}
	if v.IsZero() || v.GreaterThan(max) || v.LessThan(max.Neg()) {
		return false
	}
	if allowNegative {
		return true
	}
	return v.IsPositive()
}

// maxExponente acota el exponente decimal que se acepta de la frontera. 20 deja lugar de sobra:
// MaxMoney tiene 8 dígitos y el stock se guarda a 4 decimales.
const maxExponente = 20

// escalaSana rechaza un número cuyo exponente sea absurdo, ANTES de que nadie lo redondee.
//
// No es teoría: `decimal.Round` calcula 10^|exponente| como big.Int, así que redondear el literal
// "1e100000000" —47 bytes de cuerpo JSON— quema ~25 segundos de CPU y 279 MiB de heap, y
// "1e2000000000" pide ~830 MB, que en el gigabyte del VPS es OOM del contenedor. El límite de 1 MiB
// del body no ayuda porque el payload es diminuto, y el ReadTimeout tampoco porque el gasto ocurre
// DESPUÉS de leer; la goroutine además sigue corriendo aunque el cliente corte.
//
// Mirar el exponente es O(1) y no escala el valor, así que la guarda es gratis. Va aquí, en las dos
// validaciones de frontera, y no en cada llamador: el mismo patrón entra por POST /orders (la
// cantidad de una línea, sin exigir rol) y por el alta de productos.
func escalaSana(v decimal.Decimal) bool {
	e := v.Exponent()
	return e <= maxExponente && e >= -maxExponente
}
