package domain

import "github.com/shopspring/decimal"

// CountReference es lo que el servidor sabe de un artículo para decidir si una cifra del conteo es
// sospechosa. Nunca sale a la pantalla: el conteo es a ciegas.
type CountReference struct {
	Expected    decimal.Decimal   // lo que el sistema cree que hay, en unidad base
	LastCounted *decimal.Decimal  // el último conteo cerrado del artículo, si hay
	PackQtys    []decimal.Decimal // lo que trae cada empaque activo y no aproximado
	BaseKind    string            // masa | volumen | longitud | pieza
}

// countCaps son los topes por clase de unidad para un artículo sin ninguna otra referencia, en su
// unidad base: 50 kg, 50 L, 50 m y 5,000 piezas.
var countCaps = map[string]decimal.Decimal{
	"masa":     decimal.NewFromInt(50_000),
	"volumen":  decimal.NewFromInt(50_000),
	"longitud": decimal.NewFromInt(5_000),
	"pieza":    decimal.NewFromInt(5_000),
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
	if limit, ok := countCaps[ref.BaseKind]; ok {
		return captured.GreaterThan(limit)
	}
	return false
}
