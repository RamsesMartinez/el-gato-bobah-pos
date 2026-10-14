package domain

import (
	"cmp"
	"slices"

	"github.com/shopspring/decimal"
)

// CountLine es una cifra capturada en un conteo: lo contado y lo que el sistema tenía en ese
// momento, los dos en la unidad base del insumo.
type CountLine struct {
	IngredientID int64
	Counted      decimal.Decimal
	System       decimal.Decimal
}

// CountAdjustment es el ajuste que deja una cifra al cerrar el conteo.
type CountAdjustment struct {
	IngredientID int64
	Counted      decimal.Decimal
	System       decimal.Decimal
	Delta        decimal.Decimal
}

// PlanCountClose calcula los ajustes de un conteo: lo contado menos lo que había AL CAPTURAR, no
// al cerrar. Lo vendido después de capturar ya restó de lo guardado, así que el resultado es «lo
// contado menos lo vendido desde que se contó», sin detener las ventas mientras se cuenta (R5).
//
// Un renglón sin diferencia no deja movimiento. Los ajustes van en orden de insumo: dos escrituras
// que tocan los mismos insumos los toman en el mismo orden y no se esperan en ciclo.
func PlanCountClose(lines []CountLine) []CountAdjustment {
	out := make([]CountAdjustment, 0, len(lines))
	for _, l := range lines {
		d := Round4(l.Counted.Sub(l.System))
		if d.IsZero() {
			continue
		}
		out = append(out, CountAdjustment{IngredientID: l.IngredientID, Counted: l.Counted, System: l.System, Delta: d})
	}
	slices.SortFunc(out, func(a, b CountAdjustment) int { return cmp.Compare(a.IngredientID, b.IngredientID) })
	return out
}

// ValidStockQtyOrZero acepta una cantidad del almacén que puede valer cero: lo contado («no hay») o
// lo que llegó («todavía no llega»). ValidQty rechaza el cero a propósito —una venta de 0 piezas es
// un error—, y usarla aquí convertía esos dos casos normales en un 400.
func ValidStockQtyOrZero(q decimal.Decimal) bool {
	return q.IsZero() || ValidQty(q, MaxStockQty, false)
}
