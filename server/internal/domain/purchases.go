package domain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
)

// Compras al almacén (spec 038): cómo se configura un empaque y cómo cambia el costo de un insumo
// al comprarlo.

// PresentationInput es un empaque tal como se captura: nombre, cuánto trae y en qué unidad.
type PresentationInput struct {
	Name string
	// Qty en la unidad de Unit («25» con Unit = kg). En un empaque aproximado, Qty es cuántas
	// unidades base del insumo trae una unidad de compra («por kilo ≈ 4 piezas»: Qty = 4).
	Qty  decimal.Decimal
	Unit PurchaseUnit
	// BaseKind es el tipo de unidad en que se cuenta el insumo (masa, volumen, longitud, pieza).
	BaseKind string
	// Approximate: se compra en otro tipo de unidad que la del insumo (regla 18).
	Approximate bool
	// UnitNames son los códigos y nombres del catálogo de unidades: un empaque no se llama así.
	UnitNames []string
}

// PresentationPlan es lo que se guarda de un empaque.
type PresentationPlan struct {
	Name        string
	QtyInBase   decimal.Decimal
	Approximate bool
}

// MaxPresentationName es el largo del nombre de un empaque (el check de la 0092).
const MaxPresentationName = 40

var spaces = regexp.MustCompile(`\s+`)

// NormalizeName compara nombres sin mayúsculas, acentos ni espacios de más: la misma normalización
// del índice único de la 0092 y de `IngredientNameTaken`.
func NormalizeName(s string) string {
	r := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")
	return spaces.ReplaceAllString(r.Replace(strings.ToLower(strings.TrimSpace(s))), " ")
}

// unitAliases son las formas cortas que se usan al hablar («kilo») y que el catálogo no trae.
var unitAliases = []string{"kilo", "kilos", "litros", "gramos", "metros", "piezas", "pza", "pzas"}

// PlanPresentation valida un empaque y convierte su contenido a la unidad base del insumo. Rechaza
// convertir entre tipos (litros de algo que se pesa) salvo el empaque aproximado declarado, y las
// piezas con decimales: una bolsa trae los sobres que dice su etiqueta, no 62.5.
func PlanPresentation(in PresentationInput) (PresentationPlan, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > MaxPresentationName {
		return PresentationPlan{}, fmt.Errorf("%w: el empaque necesita un nombre de 1 a %d letras", ErrValidation, MaxPresentationName)
	}
	if !in.Qty.IsPositive() || !ValidQty(in.Qty, MaxStockQty, false) {
		return PresentationPlan{}, fmt.Errorf("%w: escribe cuánto trae el empaque", ErrValidation)
	}
	sameKind := in.Unit.Kind == in.BaseKind
	var qty decimal.Decimal
	switch {
	case in.Approximate && sameKind:
		return PresentationPlan{}, fmt.Errorf("%w: un empaque del mismo tipo de unidad no es aproximado", ErrValidation)
	case in.Approximate:
		qty = Round4(in.Qty)
	case !sameKind:
		return PresentationPlan{}, fmt.Errorf("%w: el contenido va en la misma unidad que el insumo; para comprar por %s algo que se cuenta distinto, usa un empaque aproximado", ErrValidation, in.Unit.Kind)
	default:
		if !in.Unit.ToBase.IsPositive() {
			return PresentationPlan{}, fmt.Errorf("%w: unidad inválida", ErrValidation)
		}
		qty = Round4(in.Qty.Mul(in.Unit.ToBase))
	}
	if !ValidQty(qty, MaxStockQty, false) {
		return PresentationPlan{}, fmt.Errorf("%w: el empaque trae demasiado", ErrValidation)
	}
	if in.BaseKind == "pieza" && !qty.Equal(qty.Truncate(0)) {
		return PresentationPlan{}, fmt.Errorf("%w: un empaque trae piezas enteras", ErrValidation)
	}
	if !in.Approximate {
		norm := NormalizeName(name)
		for _, u := range append(append([]string{}, in.UnitNames...), unitAliases...) {
			nu := NormalizeName(u)
			if norm == nu || norm == nu+"s" {
				return PresentationPlan{}, fmt.Errorf("%w: «%s» es una unidad; comprar así es comprar suelto, no en empaque", ErrValidation, name)
			}
		}
	}
	return PresentationPlan{Name: name, QtyInBase: qty, Approximate: in.Approximate}, nil
}

// CostingMethod decide cómo cambia el costo de un insumo al comprarlo (decisión del dueño, 14-oct).
type CostingMethod string

const (
	// CostingLast: el costo de la última compra. Es el que espera quien compra.
	CostingLast CostingMethod = "last"
	// CostingWeighted: promedio ponderado de lo que había y lo que llegó.
	CostingWeighted CostingMethod = "weighted"
)

// ParseCostingMethod acepta solo los métodos que existen: un valor mal escrito no cae a un default.
func ParseCostingMethod(s string) (CostingMethod, error) {
	switch m := CostingMethod(s); m {
	case CostingLast, CostingWeighted:
		return m, nil
	}
	return "", fmt.Errorf("%w: método de costeo desconocido %q", ErrValidation, s)
}

// NextIngredientCost es el costo por unidad base de un insumo después de una compra de `qty` a
// `unitCost`. `onHand` es lo guardado en toda la empresa ANTES de la compra.
//
// Un regalo o un importe en 0 no cambian el costo: un costo en 0 es «nadie lo ha capturado» (037),
// y meterlo al promedio bajaría el costo de todo lo que lleva ese insumo. Con lo guardado en cero o
// negativo (lo heredado) no hay nada que promediar y manda la compra (FR-006b).
//
// ponytail: una corrección o una reversión no recalcula el costo hacia atrás; se arregla con la
// siguiente compra. Reconstruir el promedio exigiría reordenar la historia de compras (R4).
func NextIngredientCost(method CostingMethod, onHand, cost, qty, unitCost decimal.Decimal, gift bool) decimal.Decimal {
	if gift || !unitCost.IsPositive() || !qty.IsPositive() {
		return cost
	}
	if method != CostingWeighted || !onHand.IsPositive() {
		return unitCost.Round(6)
	}
	total := onHand.Mul(cost).Add(qty.Mul(unitCost))
	return total.Div(onHand.Add(qty)).Round(6)
}
