package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Unir insumos (spec 056): varios insumos que en la práctica son lo mismo (dos marcas de leche) pasan
// a uno, el destino. Aquí solo las reglas; la transacción, los candados y la bitácora viven en app.

// MaxMergeSources es el tope de orígenes por unión: lo que una persona revisa en una vista previa.
const MaxMergeSources = 20

// MaxPresentationBrand es el largo de la marca de una presentación, en letras (el check de la 0104).
const MaxPresentationBrand = 40

// ValidMergeRequest rechaza una unión sin orígenes, con demasiados, con repetidos o con el destino
// entre ellos: un origen repetido traspasaría su existencia dos veces.
func ValidMergeRequest(target int64, sources []int64) error {
	if target <= 0 || len(sources) == 0 {
		return fmt.Errorf("%w: elige al menos un insumo para unir", ErrValidation)
	}
	if len(sources) > MaxMergeSources {
		return fmt.Errorf("%w: une hasta %d insumos a la vez", ErrValidation, MaxMergeSources)
	}
	seen := make(map[int64]bool, len(sources))
	for _, id := range sources {
		switch {
		case id <= 0:
			return fmt.Errorf("%w: insumo inválido", ErrValidation)
		case id == target:
			return fmt.Errorf("%w: un insumo no se une consigo mismo", ErrValidation)
		case seen[id]:
			return fmt.Errorf("%w: un insumo viene repetido", ErrValidation)
		}
		seen[id] = true
	}
	return nil
}

// MergeFactor es la equivalencia de un origen: cuánto del destino vale una unidad base del origen. Con
// la misma unidad base vale 1 y no se acepta otra (cambiaría cantidades sin que nadie lo vea); con
// unidad distinta es obligatoria.
func MergeFactor(sameUnit bool, given *decimal.Decimal) (decimal.Decimal, error) {
	one := decimal.NewFromInt(1)
	if sameUnit {
		if given != nil && !given.Equal(one) {
			return decimal.Zero, fmt.Errorf("%w: se miden igual; no lleva equivalencia", ErrValidation)
		}
		return one, nil
	}
	if given == nil || !given.IsPositive() {
		return decimal.Zero, fmt.Errorf("%w: se miden distinto; escribe cuánto equivale", ErrValidation)
	}
	if !ValidConvertedQty(*given) {
		return decimal.Zero, fmt.Errorf("%w: esa equivalencia no cabe", ErrValidation)
	}
	return *given, nil
}

// MergeRecipeLine es un renglón de receta: cantidad, cuánto vale su unidad en la base de su insumo, y
// la unidad.
type MergeRecipeLine struct {
	Qty        decimal.Decimal
	UnitToBase decimal.Decimal
	UnitID     int16
}

// MergedRecipeLine es cómo queda el renglón en el destino. Sum: se juntó con el renglón que la receta
// ya tenía del destino (el del origen se borra).
type MergedRecipeLine struct {
	Qty    decimal.Decimal
	UnitID int16
	Sum    bool
}

// PlanMergedRecipe decide el renglón del destino. Con equivalencia 1 y sin choque cambia solo de
// insumo y conserva cantidad y unidad (lo mínimo); si la receta ya lleva el destino, o hay
// equivalencia, todo se expresa en la unidad base del destino.
func PlanMergedRecipe(src MergeRecipeLine, existing *MergeRecipeLine, factor decimal.Decimal, targetBase int16) (MergedRecipeLine, error) {
	one := decimal.NewFromInt(1)
	if existing == nil && factor.Equal(one) {
		return MergedRecipeLine{Qty: src.Qty, UnitID: src.UnitID}, nil
	}
	qty := ConvertRecipeQty(src.Qty, src.UnitToBase, factor)
	out := MergedRecipeLine{UnitID: targetBase}
	if existing != nil {
		qty = Round4(qty.Add(existing.Qty.Mul(existing.UnitToBase)))
		out.Sum = true
	}
	if !qty.IsPositive() || !ValidConvertedQty(qty) {
		return MergedRecipeLine{}, fmt.Errorf("%w: con esa equivalencia una receta quedaría en cero o no cabría", ErrValidation)
	}
	out.Qty = qty
	return out, nil
}

// CostCandidate es lo que se sabe del costo de un insumo de la unión: el vigente y su última compra
// (con costo, sin regalos ni anulados). Factor convierte a la unidad del destino (1 en el destino).
type CostCandidate struct {
	ID               int64
	Factor           decimal.Decimal
	Current          decimal.Decimal
	LastPurchaseAt   *time.Time
	LastPurchaseCost *decimal.Decimal
}

// MergedCost es el costo del destino con la regla de hoy, «último precio»: el de la compra más
// reciente entre todos, convertido; sin compras, el del destino; si es cero, el primer origen con
// costo. El primero de la lista es el destino. Devuelve de qué insumo salió.
//
// ponytail: la regla configurable por empresa (promedio ponderado) es la entrega 2 del #185 y sustituye
// a esta función.
func MergedCost(cands []CostCandidate) (decimal.Decimal, int64) {
	if len(cands) == 0 {
		return decimal.Zero, 0
	}
	var best *CostCandidate
	for i := range cands {
		c := &cands[i]
		if c.LastPurchaseAt == nil || c.LastPurchaseCost == nil {
			continue
		}
		if best == nil || c.LastPurchaseAt.After(*best.LastPurchaseAt) {
			best = c
		}
	}
	if best != nil {
		return ConvertCost(*best.LastPurchaseCost, best.Factor), best.ID
	}
	if cands[0].Current.IsPositive() {
		return cands[0].Current, cands[0].ID
	}
	for _, c := range cands[1:] {
		if c.Current.IsPositive() {
			return ConvertCost(c.Current, c.Factor), c.ID
		}
	}
	return cands[0].Current, cands[0].ID
}

// MinStockSource es el mínimo de un origen y su equivalencia.
type MinStockSource struct {
	Min    *decimal.Decimal
	Factor decimal.Decimal
}

// MergeMinStock: el destino conserva su mínimo; si no tenía, toma el mayor de los orígenes, convertido.
func MergeMinStock(target *decimal.Decimal, sources []MinStockSource) *decimal.Decimal {
	if target != nil {
		return target
	}
	var best *decimal.Decimal
	for _, s := range sources {
		if s.Min == nil {
			continue
		}
		v := ConvertQty(*s.Min, s.Factor)
		if best == nil || v.GreaterThan(*best) {
			best = &v
		}
	}
	return best
}

// Acciones sobre una presentación del origen al unir.
const (
	MergeMove     = "move"     // pasa al destino conservando su id
	MergeArchive  = "archive"  // se queda archivada en el origen
	MergeConflict = "conflict" // choca con otra que trae distinto: bloquea la unión
)

// PresentationKey es una presentación activa del destino: lo que decide si otra choca con ella.
type PresentationKey struct {
	Name      string
	Brand     string
	QtyInBase decimal.Decimal
}

// MergePresentation es una presentación activa de un origen, ya convertida a la unidad del destino.
// Fits: sigue siendo válida en el destino (la app lo calcula con las unidades).
type MergePresentation struct {
	ID        int64
	Name      string
	Brand     string
	QtyInBase decimal.Decimal
	Fits      bool
}

// MergedPresentation es la decisión para una presentación del origen.
type MergedPresentation struct {
	ID        int64
	Name      string
	Brand     string
	QtyInBase decimal.Decimal
	Action    string
}

// PlanMergedPresentations decide qué pasa con cada presentación activa de un origen. Sin marca llega
// con el nombre del origen como marca: así «Caja» de la Marca B no choca con la «Caja» del destino.
// Mismo nombre, marca y contenido que una del destino (o que una que ya se movió de otro origen): es la
// misma, la del origen se archiva. Mismo nombre y marca con distinto contenido: no se adivina cuál vale.
func PlanMergedPresentations(target []PresentationKey, sources []MergePresentation, sourceName string) []MergedPresentation {
	key := func(name, brand string) string {
		return NormalizeReasonName(name) + "\x00" + NormalizeReasonName(brand)
	}
	taken := make(map[string]decimal.Decimal, len(target))
	for _, p := range target {
		taken[key(p.Name, p.Brand)] = p.QtyInBase
	}
	out := make([]MergedPresentation, 0, len(sources))
	for _, p := range sources {
		brand := strings.TrimSpace(p.Brand)
		if brand == "" {
			brand = truncateRunes(CleanArticleName(sourceName), MaxPresentationBrand)
		}
		m := MergedPresentation{ID: p.ID, Name: p.Name, Brand: brand, QtyInBase: p.QtyInBase, Action: MergeMove}
		k := key(p.Name, brand)
		switch qty, clash := taken[k]; {
		case !p.Fits:
			m.Action = MergeArchive
		case clash && qty.Equal(p.QtyInBase):
			m.Action = MergeArchive
		case clash:
			m.Action = MergeConflict
		default:
			taken[k] = p.QtyInBase
		}
		out = append(out, m)
	}
	return out
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n]))
}
