package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// Orígenes del costo de un producto que se pueden elegir desde el catálogo. «compra» existe en la
// base pero lo fija la captura de compras, no una persona desde la ficha del producto.
const (
	CostSourceManual = "manual"
	CostSourceRecipe = "receta"
)

// ProductCostChange es un cambio de costo de producto ya validado: manual con su monto en
// centavos, o «de su receta» sin monto (lo calcula el motor de costeo).
type ProductCostChange struct {
	Source string
	Amount decimal.Decimal
}

// NewProductCostChange validates the requested cost source and amount. A manual cost without an
// amount is rejected instead of read as zero: an emptied field would silently zero the margin.
func NewProductCostChange(source string, amount *decimal.Decimal) (ProductCostChange, error) {
	switch source {
	case CostSourceManual:
		if amount == nil {
			return ProductCostChange{}, fmt.Errorf("%w: falta el costo", ErrValidation)
		}
		v := Round2(*amount)
		if !ValidMoney(v, true) {
			return ProductCostChange{}, fmt.Errorf("%w: costo inválido", ErrValidation)
		}
		return ProductCostChange{Source: source, Amount: v}, nil
	case CostSourceRecipe:
		if amount != nil {
			return ProductCostChange{}, fmt.Errorf("%w: el costo de receta no se captura", ErrValidation)
		}
		return ProductCostChange{Source: source}, nil
	default:
		return ProductCostChange{}, fmt.Errorf("%w: origen de costo desconocido", ErrValidation)
	}
}
