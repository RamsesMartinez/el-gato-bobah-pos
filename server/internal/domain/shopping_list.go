package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// La lista de compra (spec 084, historia 8): cuánto conviene comprar y por qué, en una línea. La misma
// regla sirve a la pantalla y al correo de la mañana.

// DefaultCoverageDays son los días que debe alcanzar lo comprado si el artículo no dice otra cosa
// (decisión del dueño).
const DefaultCoverageDays = 14

// Pack es un empaque de compra con su contenido en la unidad base y su costo por unidad base en la
// última compra (nulo si nunca se compró).
type Pack struct {
	Name      string
	QtyInBase decimal.Decimal
	UnitCost  *decimal.Decimal
}

// PurchaseInput es lo que se sabe de un artículo para sugerir su compra.
type PurchaseInput struct {
	DailyUse     *decimal.Decimal // consumo diario medido en la unidad base
	Measured     bool             // hay consumo suficiente para creerle
	OnHand       decimal.Decimal
	Unit         string // código de la unidad base
	CoverageDays int    // 0 = DefaultCoverageDays
	Packs        []Pack
}

// PurchaseSuggestion es lo que se sugiere comprar.
type PurchaseSuggestion struct {
	Packs    int             `json:"packs"`
	PackName string          `json:"packName"`
	Qty      decimal.Decimal `json:"qty"`
}

// PurchaseAdvice es la sugerencia (o su ausencia) con el porqué.
type PurchaseAdvice struct {
	Suggested  *PurchaseSuggestion `json:"suggested"`
	Why        string              `json:"why"`
	NeedsCount bool                `json:"needsCount"`
}

// SuggestPurchase calcula consumo diario × días − existencia, redondeado hacia arriba a empaques
// enteros del de menor costo por unidad (sin costos, el más chico). Sin consumo medido no sugiere: una
// cantidad inventada es peor que ninguna. Con la existencia en negativo pide conteo antes de comprar.
func SuggestPurchase(in PurchaseInput) PurchaseAdvice {
	days := in.CoverageDays
	if days <= 0 {
		days = DefaultCoverageDays
	}
	if in.OnHand.IsNegative() {
		return PurchaseAdvice{NeedsCount: true, Why: "la existencia está en negativo: pide un conteo antes de comprar"}
	}
	if !in.Measured || in.DailyUse == nil || !in.DailyUse.IsPositive() {
		return PurchaseAdvice{Why: "sin consumo medido: no está en ninguna receta o casi no se ha vendido"}
	}
	need := in.DailyUse.Mul(decimal.NewFromInt(int64(days))).Sub(in.OnHand)
	if !need.IsPositive() {
		lasts := in.OnHand.Div(*in.DailyUse).Floor()
		return PurchaseAdvice{Why: fmt.Sprintf("alcanza para %s días", lasts.String())}
	}
	why := fmt.Sprintf("se usan ~%s al día, quedan %s, objetivo %d días", HumanQty(*in.DailyUse, in.Unit), HumanQty(in.OnHand, in.Unit), days)
	pack := cheapestPack(in.Packs)
	if pack == nil {
		return PurchaseAdvice{Suggested: &PurchaseSuggestion{Qty: need.Ceil()}, Why: why}
	}
	n := need.Div(pack.QtyInBase).Ceil()
	return PurchaseAdvice{Suggested: &PurchaseSuggestion{Packs: int(n.IntPart()), PackName: pack.Name, Qty: n.Mul(pack.QtyInBase)}, Why: why}
}

func cheapestPack(packs []Pack) *Pack {
	var best *Pack
	for i := range packs {
		p := &packs[i]
		if !p.QtyInBase.IsPositive() {
			continue
		}
		switch {
		case best == nil:
			best = p
		case p.UnitCost != nil && (best.UnitCost == nil || p.UnitCost.LessThan(*best.UnitCost)):
			best = p
		case p.UnitCost == nil && best.UnitCost == nil && p.QtyInBase.LessThan(best.QtyInBase):
			best = p
		}
	}
	return best
}

// HumanQty dice una cantidad en la unidad con que la piensa quien compra: 1200 g son 1.2 kg.
func HumanQty(q decimal.Decimal, unit string) string {
	big := map[string]string{"g": "kg", "ml": "L"}
	switch unit {
	case "pieza":
		n := q.Round(2)
		if n.Equal(decimal.NewFromInt(1)) {
			return "1 pza"
		}
		return n.String() + " pzas"
	case "cm":
		if q.Abs().GreaterThanOrEqual(decimal.NewFromInt(100)) {
			return q.Div(decimal.NewFromInt(100)).Round(2).String() + " m"
		}
	case "g", "ml":
		if q.Abs().GreaterThanOrEqual(decimal.NewFromInt(1000)) {
			return q.Div(decimal.NewFromInt(1000)).Round(2).String() + " " + big[unit]
		}
	}
	return q.Round(2).String() + " " + unit
}
