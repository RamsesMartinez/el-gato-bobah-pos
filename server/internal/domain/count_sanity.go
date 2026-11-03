package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// CountReference es lo que el servidor sabe de un artículo para decidir si una cifra del conteo es
// sospechosa. Nunca sale a la pantalla: el conteo es a ciegas.
type CountReference struct {
	Expected    decimal.Decimal   // lo que el sistema cree que hay, en unidad base
	LastCounted *decimal.Decimal  // el último conteo cerrado del artículo, si hay
	PackQtys    []decimal.Decimal // lo que trae cada empaque activo y no aproximado
	BaseKind    string            // masa | volumen | longitud | pieza
	Confirm     decimal.Decimal   // la cifra para confirmar de la empresa en esta clase; cero = default
}

var ten = decimal.NewFromInt(10)

// CountNeedsConfirm dice si una cifra del conteo pide confirmar antes de guardarse (FR-012).
//
// La referencia cae en cascada porque lo esperado casi nunca sirve: con el inventario heredado, la
// mayoría de los artículos está en cero o en negativo, y «10 veces lo esperado» con 0 no avisaría
// nunca. Ese fue justo el caso que la motivó: 740 escrito en un artículo en kg con existencia cero.
//
// Contar 0 nunca avisa: «no hay» es una respuesta normal.
func CountNeedsConfirm(captured decimal.Decimal, ref CountReference) bool {
	if !captured.IsPositive() {
		return false
	}
	if len(ref.PackQtys) > 0 {
		smallest := ref.PackQtys[0]
		for _, q := range ref.PackQtys[1:] {
			if q.LessThan(smallest) {
				smallest = q
			}
		}
		// Menos de 1/100 del empaque más chico: casi siempre se quiso escribir la unidad grande.
		if captured.Mul(decimal.NewFromInt(100)).LessThan(smallest) {
			return true
		}
	}
	base := decimal.Zero
	switch {
	case ref.Expected.IsPositive():
		base = ref.Expected
	case ref.LastCounted != nil && ref.LastCounted.IsPositive():
		base = *ref.LastCounted
	}
	if base.IsPositive() {
		return captured.GreaterThan(base.Mul(ten)) || captured.Mul(ten).LessThan(base)
	}
	if len(ref.PackQtys) > 0 {
		largest := ref.PackQtys[0]
		for _, q := range ref.PackQtys[1:] {
			if q.GreaterThan(largest) {
				largest = q
			}
		}
		return captured.GreaterThan(largest.Mul(ten))
	}
	// Sin otra referencia, la cifra que configuró la empresa (spec 044). Sin ella, la del default: una
	// referencia armada sin límites no deja de avisar.
	limit := ref.Confirm
	if !limit.IsPositive() {
		p, ok := DefaultCountLimits().ForKind(ref.BaseKind)
		if !ok {
			return false
		}
		limit = p.Confirm
	}
	return captured.GreaterThan(limit)
}

// ErrCountNeedsConfirm: la cifra pasa la referencia del artículo y se pide confirmarla (409
// NEEDS_CONFIRM). El mensaje no dice contra qué se comparó: el conteo es a ciegas.
var ErrCountNeedsConfirm = fmt.Errorf("%w: ¿seguro? revisa la cifra y la unidad", ErrConflict)

// PiecesFromWeight convierte lo pesado a piezas con un empaque aproximado («Kilo ≈ 400 pz»): el peso
// en la unidad de ese empaque, por las piezas que trae, redondeado a piezas enteras.
func PiecesFromWeight(weight, weightToBase, packUnitToBase, piecesPerPack decimal.Decimal) decimal.Decimal {
	if !packUnitToBase.IsPositive() {
		return decimal.Zero
	}
	return weight.Mul(weightToBase).Div(packUnitToBase).Mul(piecesPerPack).Round(0)
}
