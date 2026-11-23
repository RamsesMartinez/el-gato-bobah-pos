package domain

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// Caducidad por lote (spec 098). Un lote es un artículo en un almacén con una fecha de caducidad;
// lo sin fecha se deriva (existencia − lotes). El orden de consumo vive en la base
// (`apply_stock_lots`, migración 0128) para que todos los caminos que mueven existencias —también
// la versión anterior de la API durante una subida— apliquen la misma regla; aquí queda lo puro:
// validar lo que se captura y proyectar si un lote se termina antes de caducar.

// ErrLotsExceedCounted: las fechas de un artículo contado suman más que lo contado.
var ErrLotsExceedCounted = fmt.Errorf("%w: las fechas suman más de lo contado", ErrValidation)

// MaxExpiryYears acota una fecha de caducidad hacia adelante: más allá es un error de captura (un
// año tecleado de más), no un producto real de un negocio de comida.
const MaxExpiryYears = 10

// MaxCountLotsPerItem acota las fechas de un artículo contado: un estante real trae pocas, y sin tope
// una sola captura podía mandar miles e inflar la transacción.
const MaxCountLotsPerItem = 20

var minExpiry = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// ParseExpiry valida una fecha de caducidad `YYYY-MM-DD`. Una fecha pasada es válida (el conteo
// registra lo que ya caducó); la frontera de compras pide confirmarla en pantalla.
func ParseExpiry(s string, today time.Time) (time.Time, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: la fecha de caducidad va como AAAA-MM-DD", ErrValidation)
	}
	limit := time.Date(today.Year()+MaxExpiryYears, today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	if t.Before(minExpiry) || t.After(limit) {
		return time.Time{}, fmt.Errorf("%w: la fecha de caducidad no es creíble", ErrValidation)
	}
	return t, nil
}

// LotQty es una cantidad con su fecha, como la captura el conteo.
type LotQty struct {
	ExpiresOn time.Time
	Qty       decimal.Decimal
}

// ValidLotBreakdown valida el desglose de fechas de un artículo contado: cada cantidad mayor que
// cero, sin fechas repetidas (dos renglones con la misma fecha son el mismo lote) y la suma no
// pasa de lo contado; lo que falta es lo sin fecha.
func ValidLotBreakdown(counted decimal.Decimal, lots []LotQty) error {
	if len(lots) > MaxCountLotsPerItem {
		return fmt.Errorf("%w: hasta %d fechas por artículo", ErrValidation, MaxCountLotsPerItem)
	}
	seen := map[time.Time]bool{}
	sum := decimal.Zero
	for _, l := range lots {
		if !ValidQty(l.Qty, MaxStockQty, false) {
			return fmt.Errorf("%w: cada fecha lleva una cantidad mayor que cero", ErrValidation)
		}
		if seen[l.ExpiresOn] {
			return fmt.Errorf("%w: la fecha %s está repetida", ErrValidation, l.ExpiresOn.Format(time.DateOnly))
		}
		seen[l.ExpiresOn] = true
		sum = sum.Add(l.Qty)
	}
	if sum.GreaterThan(counted) {
		return ErrLotsExceedCounted
	}
	return nil
}

// DaysLeft son los días de hoy a la fecha de caducidad; negativo si ya caducó, 0 si caduca hoy.
func DaysLeft(expiresOn, today time.Time) int {
	return int(math.Round(expiresOn.Sub(today).Hours() / 24))
}

// LotStock es un lote con lo que le queda.
type LotStock struct {
	ExpiresOn time.Time
	Qty       decimal.Decimal
}

// LotProjection dice qué pasará con un lote al ritmo de consumo de hoy.
type LotProjection struct {
	Expired bool
	// FinishesInDays: en cuántos días se termina; nulo si no se termina antes de caducar.
	FinishesInDays *int
	// LeftoverAtExpiry: cuánto quedará al caducar; nulo si se termina a tiempo.
	LeftoverAtExpiry *decimal.Decimal
}

// ProjectLots simula el consumo diario contra las caducidades, en el mismo orden en que la base
// descuenta: primero lo sin fecha (si es positivo), luego los lotes buenos por fecha. Lo que queda
// de un lote al caducar ya no se usa, así que el siguiente empieza ese día y no después. Un lote
// que caduca hoy todavía se usa hoy. Sin consumo (nulo o cero), ningún lote se termina. Devuelve
// una proyección por lote, en el orden recibido.
func ProjectLots(undated decimal.Decimal, lots []LotStock, daily *decimal.Decimal, today time.Time) []LotProjection {
	out := make([]LotProjection, len(lots))
	order := make([]int, len(lots))
	for i := range lots {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return lots[order[a]].ExpiresOn.Before(lots[order[b]].ExpiresOn) })

	noUse := daily == nil || !daily.IsPositive()
	consumed := decimal.Zero // lo que el consumo ya se llevó, en unidades
	if !noUse && undated.IsPositive() {
		consumed = undated
	}
	for _, i := range order {
		l := lots[i]
		left := DaysLeft(l.ExpiresOn, today)
		if left < 0 {
			q := l.Qty
			out[i] = LotProjection{Expired: true, LeftoverAtExpiry: &q}
			continue
		}
		if noUse {
			q := l.Qty
			out[i] = LotProjection{LeftoverAtExpiry: &q}
			continue
		}
		// Hasta el fin del día de caducidad el consumo alcanza a llevarse esto en total.
		reach := daily.Mul(decimal.NewFromInt(int64(left + 1)))
		usable := decimal.Max(decimal.Zero, decimal.Min(l.Qty, reach.Sub(consumed)))
		consumed = consumed.Add(usable)
		leftover := Round4(l.Qty.Sub(usable))
		if leftover.IsPositive() {
			out[i] = LotProjection{LeftoverAtExpiry: &leftover}
			continue
		}
		days := int(math.Ceil(consumed.Div(*daily).InexactFloat64() - 1e-9))
		out[i] = LotProjection{FinishesInDays: &days}
	}
	return out
}
