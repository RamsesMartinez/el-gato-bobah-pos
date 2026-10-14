package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// Salir de la existencia propia (spec 037, research R6). Los revendidos se modelan como receta de
// un insumo por pieza; un producto que todavía cuenta su propia existencia la entrega al cambiar de
// modo, y eso no puede perder ni inventar latas.

// TargetKind es a qué modo va el producto.
type TargetKind string

const (
	TargetItems   TargetKind = "items"
	TargetPackage TargetKind = "package"
	TargetNothing TargetKind = "nothing"
)

// ModeTarget es lo que se va a guardar. SingleIngredient solo cuando la receta lleva un insumo: con
// varios no hay equivalencia única entre una pieza del producto y lo que lleva.
type ModeTarget struct {
	Kind             TargetKind
	SingleIngredient *int64
	// QtyPerSale es cuánto del insumo, en su unidad base, sale por cada pieza del producto.
	QtyPerSale decimal.Decimal
}

// BranchLevels son las existencias bloqueadas (FOR UPDATE) de una sucursal: las del producto y, si
// la receta lleva un solo insumo, las de ese insumo.
type BranchLevels struct {
	BranchID   int64
	Product    decimal.Decimal
	Ingredient decimal.Decimal
}

// ModeChangeInput es lo que decide la transición. Los Seen son lo que mostró la pantalla al
// preguntar «¿Cuántas tienes de verdad?»; ActualStock, lo que se contestó.
type ModeChangeInput struct {
	From                 ConsumptionMode
	ProductID            int64
	Target               ModeTarget
	Levels               []BranchLevels
	ActualStock          *decimal.Decimal
	SeenProductOnHand    *decimal.Decimal
	SeenIngredientOnHand *decimal.Decimal
}

// StockAdjustment es un movimiento de ajuste a insertar: positivo entra, negativo sale.
type StockAdjustment struct {
	BranchID int64
	ItemType string // "producto" | "ingrediente", como stock_movements.item_type
	ID       int64
	Qty      decimal.Decimal
}

// ModeChange es el resultado: los ajustes y si se apaga la existencia propia.
type ModeChange struct {
	Adjustments       []StockAdjustment
	TurnOffTrackStock bool
}

// PlanModeChange decide qué ajustes se insertan al salir de la existencia propia. Los niveles
// llegan ya bloqueados, así que las cifras son las de verdad y no las que mostró la pantalla.
func PlanModeChange(in ModeChangeInput) (ModeChange, error) {
	var out ModeChange
	if in.From != ModeOwnStock {
		return out, nil
	}
	out.TurnOffTrackStock = true
	single := in.Target.Kind == TargetItems && in.Target.SingleIngredient != nil

	if !single {
		for _, l := range in.Levels {
			if !l.Product.IsZero() {
				out.Adjustments = append(out.Adjustments, StockAdjustment{BranchID: l.BranchID, ItemType: "producto", ID: in.ProductID, Qty: l.Product.Neg()})
			}
		}
		return out, nil
	}

	ing := *in.Target.SingleIngredient
	// Las sucursales donde las dos cuentas tienen existencia: ahí hay que decidir cuántas hay de
	// verdad. Sumarlas a ciegas contaría dos veces las mismas latas.
	var both []BranchLevels
	for _, l := range in.Levels {
		if !l.Product.IsZero() && !l.Ingredient.IsZero() {
			both = append(both, l)
		}
	}
	if len(both) > 1 {
		return out, fmt.Errorf("%w: el producto y el insumo tienen existencia en más de una sucursal; ajústalo con un conteo", ErrConflict)
	}
	if len(both) == 1 {
		if in.ActualStock == nil || in.SeenProductOnHand == nil || in.SeenIngredientOnHand == nil {
			return out, fmt.Errorf("%w: el insumo ya tiene existencia; falta decir cuántas hay de verdad", ErrValidation)
		}
		if !ValidQty(*in.ActualStock, MaxStockQty, false) {
			return out, fmt.Errorf("%w: la cantidad no es válida", ErrValidation)
		}
		if !in.SeenProductOnHand.Equal(both[0].Product) || !in.SeenIngredientOnHand.Equal(both[0].Ingredient) {
			return out, fmt.Errorf("%w: la existencia cambió mientras decidías; vuelve a revisar las cifras", ErrConflict)
		}
	}

	for _, l := range in.Levels {
		if l.Product.IsZero() {
			continue
		}
		out.Adjustments = append(out.Adjustments, StockAdjustment{BranchID: l.BranchID, ItemType: "producto", ID: in.ProductID, Qty: l.Product.Neg()})
		var toIngredient decimal.Decimal
		if len(both) == 1 && both[0].BranchID == l.BranchID {
			// La cifra elegida queda como existencia del insumo; el ajuste es la diferencia.
			toIngredient = Round4(*in.ActualStock).Sub(l.Ingredient)
		} else {
			toIngredient = StockQty(l.Product.Mul(in.Target.QtyPerSale).Abs())
			if l.Product.IsNegative() {
				toIngredient = toIngredient.Neg()
			}
		}
		if !toIngredient.IsZero() {
			out.Adjustments = append(out.Adjustments, StockAdjustment{BranchID: l.BranchID, ItemType: "ingrediente", ID: ing, Qty: toIngredient})
		}
	}
	return out, nil
}
