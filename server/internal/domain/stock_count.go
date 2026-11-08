package domain

import (
	"cmp"
	"slices"

	"github.com/shopspring/decimal"
)

// CountLine es una cifra capturada en un conteo: lo contado y lo que el sistema tenía en ese
// momento, los dos en la unidad base. ItemID es un insumo o un producto con existencia propia
// (spec 069): la regla es la misma, y el cierre la aplica a cada clase por separado.
type CountLine struct {
	ItemID  int64
	Counted decimal.Decimal
	System  decimal.Decimal
}

// CountAdjustment es el ajuste que deja una cifra al cerrar el conteo.
type CountAdjustment struct {
	ItemID  int64
	Counted decimal.Decimal
	System  decimal.Decimal
	Delta   decimal.Decimal
}

// PlanCountClose calcula los ajustes de un conteo: lo contado menos lo que había AL CAPTURAR, no
// al cerrar. Lo vendido después de capturar ya restó de lo guardado, así que el resultado es «lo
// contado menos lo vendido desde que se contó», sin detener las ventas mientras se cuenta (R5).
//
// Un renglón sin diferencia no deja movimiento. Los ajustes van en orden de id: dos escrituras que
// tocan las mismas existencias las toman en el mismo orden y no se esperan en ciclo.
func PlanCountClose(lines []CountLine) []CountAdjustment {
	out := make([]CountAdjustment, 0, len(lines))
	for _, l := range lines {
		d := Round4(l.Counted.Sub(l.System))
		if d.IsZero() {
			continue
		}
		out = append(out, CountAdjustment{ItemID: l.ItemID, Counted: l.Counted, System: l.System, Delta: d})
	}
	slices.SortFunc(out, func(a, b CountAdjustment) int { return cmp.Compare(a.ItemID, b.ItemID) })
	return out
}

// ValidStockQtyOrZero acepta una cantidad del almacén que puede valer cero: lo contado («no hay») o
// lo que llegó («todavía no llega»). ValidQty rechaza el cero a propósito —una venta de 0 piezas es
// un error—, y usarla aquí convertía esos dos casos normales en un 400. El cero también pasa por la
// guarda de escala: «0e9999» es cero y no es una cantidad que alguien teclee.
func ValidStockQtyOrZero(q decimal.Decimal) bool {
	if q.IsZero() {
		return escalaSana(q)
	}
	return ValidQty(q, MaxStockQty, false)
}

// ValidCountPieces acepta lo contado de un producto con existencia propia: piezas enteras, cero
// incluido («no hay»). Una lata no se parte; un decimal ahí es un dedazo que movería el valor del
// inventario (spec 069, R4).
func ValidCountPieces(q decimal.Decimal) bool {
	return ValidStockQtyOrZero(q) && q.Equal(q.Truncate(0))
}
