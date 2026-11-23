package domain

import (
	"fmt"
	"math"

	"github.com/shopspring/decimal"
)

// WasteSource dice de dónde salió el % de merma esperada aplicado a un renglón de receta (spec 096):
// la pantalla de costos lo muestra para que un costo con tres fuentes posibles se pueda auditar.
type WasteSource string

const (
	WasteFromArticle WasteSource = "article"
	WasteFromProduct WasteSource = "product"
	WasteFromGeneral WasteSource = "general"
	WasteNone        WasteSource = "none"
)

// ResolveExpectedWaste aplica la precedencia del #226, de lo más específico a lo más general:
// artículo capturado → producto → general → nada. Un nil es «sin captura» y cae al siguiente; un 0 es
// una decisión («aquí no se pierde nada») y se respeta: si cayera, la lata capturada en 0 a propósito
// sumaría la merma general. Dentro de un preparado se llama con product y general en nil (R2): el
// general solo aplica en la receta del producto, para que un 5 % sea 5 % y no se componga por nivel.
func ResolveExpectedWaste(article, product, general *float64) (float64, WasteSource) {
	switch {
	case article != nil:
		return *article, WasteFromArticle
	case product != nil:
		return *product, WasteFromProduct
	case general != nil:
		return *general, WasteFromGeneral
	}
	return 0, WasteNone
}

// MaxExpectedWastePct es el tope exclusivo: un 100 % duplicaría el costo y no cabe en numeric(5,2).
var MaxExpectedWastePct = decimal.NewFromInt(100)

// ValidExpectedWastePct acepta nil (sin captura) o un % de 0 a menos de 100 con dos decimales. Fuera
// de eso es ErrValidation (400) antes de tocar la columna, que lo rechazaría como 500.
func ValidExpectedWastePct(v *decimal.Decimal) error {
	if v == nil {
		return nil
	}
	if v.IsNegative() || v.GreaterThanOrEqual(MaxExpectedWastePct) {
		return fmt.Errorf("%w: la merma va de 0 a menos de 100 %%", ErrValidation)
	}
	if !v.Equal(v.Round(2)) {
		return fmt.Errorf("%w: la merma lleva a lo más dos decimales", ErrValidation)
	}
	return nil
}

// RealWasteState dice si la merma real tiene cociente o por qué no.
type RealWasteState string

const (
	RealWasteRatio   RealWasteState = "ratio"
	RealWasteNoSales RealWasteState = "no_sales"
	RealWasteNoData  RealWasteState = "no_data"
)

// RealWaste es lo mermado entre lo consumido por ventas, en %, a un decimal. Solo informa: ningún
// costo sale de aquí (spec 096). Sin consumo no hay cociente: un artículo que solo se tiró dice «sin
// ventas», nunca un porcentaje infinito.
func RealWaste(wasted, consumed float64) (*float64, RealWasteState) {
	switch {
	case consumed <= 0 && wasted <= 0:
		return nil, RealWasteNoData
	case consumed <= 0:
		return nil, RealWasteNoSales
	}
	return new(math.Round(wasted/consumed*1000) / 10), RealWasteRatio
}

// MergeEdge es una unión de artículos: una unidad base del viejo vale Factor del nuevo.
type MergeEdge struct {
	Old, New int64
	Factor   float64
}

// MergeChain devuelve, para el artículo y todos los que se le unieron (directo o en cadena), cuánto
// vale una unidad base de cada uno en la del artículo. Las ventas y mermas de antes de una unión siguen
// con el id y la unidad del origen; la merma real del destino las suma con este factor (R14 H1).
// Hasta 16 niveles, como el trigger de la 0104; un ciclo no cuelga.
func MergeChain(target int64, edges []MergeEdge) map[int64]float64 {
	out := map[int64]float64{target: 1}
	frontier := []int64{target}
	for depth := 0; depth < 16 && len(frontier) > 0; depth++ {
		var next []int64
		for _, to := range frontier {
			for _, e := range edges {
				if e.New != to {
					continue
				}
				if _, seen := out[e.Old]; seen {
					continue
				}
				out[e.Old] = out[to] * e.Factor
				next = append(next, e.Old)
			}
		}
		frontier = next
	}
	return out
}
