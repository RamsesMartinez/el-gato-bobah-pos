package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// Conversión de la cantidad COMPRADA a la unidad BASE del almacén.
//
// El almacén lleva todo en la base del kind (g para masa, ml para volumen, pieza para conteo),
// pero se compra en la unidad que usa el proveedor: 0.280 kg de aguacate, 4 piezas de harina
// de 432 g, 3 piezas de lechuga. Los dos primeros casos NO se resuelven igual y de ahí que
// esto exista en vez de un simple `qty * to_base`.

// ErrPackUnknown: se compró por pieza un artículo que se almacena por peso/volumen y no se
// sabe cuánto trae una pieza. No es un error del operador: es un dato que falta, y la línea
// queda pendiente hasta que se registre el formato del proveedor.
var ErrPackUnknown = fmt.Errorf("%w: falta el contenido de una unidad de compra (p. ej. 432 g por pieza) para convertir a unidad de almacén", ErrValidation)

// ErrUnitKindMismatch: se compró en una unidad de masa un artículo que se almacena en volumen
// (o al revés). Convertir exigiría una densidad que no tenemos.
var ErrUnitKindMismatch = fmt.Errorf("%w: la unidad de compra no es compatible con la unidad de almacén del artículo", ErrValidation)

// PurchaseUnit describe la unidad en que se compró frente a la unidad base del artículo.
type PurchaseUnit struct {
	Kind     string          // kind de la unidad de compra: masa | volumen | pieza
	ToBase   decimal.Decimal // factor de esa unidad a la base de su kind (kg → 1000)
	BaseKind string          // kind de la unidad base del artículo
	// PackQtyInBase: cuánto trae UNA unidad de compra expresado en la base del artículo
	// (una pieza de "Harina … 432 g" → 432). nil cuando no se conoce. Solo se usa para salvar
	// el salto pieza → masa/volumen; en los demás casos el factor de la unidad basta.
	PackQtyInBase *decimal.Decimal
}

// BaseQty convierte qty a unidad base del artículo.
//
// Tres caminos, y el tercero es la razón de ser de la función:
//  1. mismo kind (kg → g, l → ml): qty × ToBase.
//  2. pieza → pieza: qty tal cual.
//  3. pieza → masa/volumen: qty × PackQtyInBase. Aquí ToBase no sirve — "una pieza" no son N
//     gramos en general, depende del producto — y sin el formato del proveedor no se puede
//     convertir sin inventar el dato.
func BaseQty(qty decimal.Decimal, u PurchaseUnit) (decimal.Decimal, error) {
	if !qty.IsPositive() {
		return decimal.Zero, fmt.Errorf("%w: la cantidad comprada debe ser mayor a cero", ErrValidation)
	}
	switch u.Kind {
	case u.BaseKind:
		if !u.ToBase.IsPositive() {
			return decimal.Zero, fmt.Errorf("%w: factor de unidad inválido", ErrValidation)
		}
		return Round4(qty.Mul(u.ToBase)), nil

	case "pieza":
		if u.PackQtyInBase == nil || !u.PackQtyInBase.IsPositive() {
			return decimal.Zero, ErrPackUnknown
		}
		return Round4(qty.Mul(*u.PackQtyInBase)), nil

	default:
		// Comprar en kg algo que se almacena en ml (o al revés) necesitaría densidad. Antes de
		// asumir 1 g = 1 ml —cierto para el agua y falso para el aceite y la miel— es mejor
		// rechazar y que el operador corrija la unidad.
		return decimal.Zero, ErrUnitKindMismatch
	}
}

// UnitCost reparte el importe de la línea entre las unidades base recibidas, para guardar el
// costo por unidad base en el movimiento de stock (y, más adelante, alimentar el costeo).
// Devuelve 0 cuando no hay base sobre la que repartir, en vez de dividir entre cero.
func UnitCost(amount, baseQty decimal.Decimal) decimal.Decimal {
	if !baseQty.IsPositive() {
		return decimal.Zero
	}
	// 6 decimales: es la escala de stock_movements.unit_cost (numeric(12,6)). Un ingrediente
	// que cuesta $80 el kilo vale 0.08 por gramo, y redondear a centavos lo volvería 0.
	return amount.Div(baseQty).Round(6)
}
