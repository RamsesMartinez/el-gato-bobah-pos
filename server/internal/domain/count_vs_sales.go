package domain

import "github.com/shopspring/decimal"

// CountVsSalesInput son las cifras de un artículo entre el conteo cerrado anterior de un almacén y
// éste (spec 096, subida 2). Todas en la unidad base y positivas: Sold es lo que las ventas
// descontaron neto de cancelaciones, ExpectedWaste lo que descontó la merma esperada, Registered la
// merma registrada, TransfersOut lo que salió a otro almacén. System es lo que el sistema tenía al
// capturar y Counted lo contado.
type CountVsSalesInput struct {
	HasPrevious   bool
	Initial       decimal.Decimal
	Entries       decimal.Decimal
	Sold          decimal.Decimal
	ExpectedWaste decimal.Decimal
	Registered    decimal.Decimal
	TransfersOut  decimal.Decimal
	System        decimal.Decimal
	Counted       decimal.Decimal
}

// CountVsSalesRow es el renglón del reporte: la diferencia y su % sobre lo vendido (nil sin ventas).
type CountVsSalesRow struct {
	CountVsSalesInput
	Diff decimal.Decimal
	Pct  *decimal.Decimal
}

// CountVsSales calcula la diferencia contra lo que el sistema tenía (lo mismo que mide el faltante del
// conteo) y la expresa como % de lo vendido, que es la cifra comparable con la merma esperada.
func CountVsSales(in CountVsSalesInput) CountVsSalesRow {
	r := CountVsSalesRow{CountVsSalesInput: in, Diff: Round4(in.Counted.Sub(in.System))}
	if in.Sold.IsPositive() {
		p := r.Diff.Div(in.Sold).Mul(decimal.NewFromInt(100)).Round(1)
		r.Pct = &p
	}
	return r
}

var (
	suggestMin   = decimal.NewFromInt(2)
	suggestMax   = decimal.NewFromInt(4)
	suggestRange = decimal.RequireFromString("1.5")
	reviewAbove  = decimal.NewFromInt(10)
)

// SuggestExpectedWaste decide, con los % de los últimos conteos de un artículo (el más reciente
// primero), si sugerir un % de merma o marcarlo para revisar. Sugiere solo con tres conteos con ventas,
// todos con faltante entre 2 y 4 % y una variación de 1.5 puntos o menos: el promedio, a medio punto.
// Marca revisar si alguno pasa de 10 % o si los tres varían más de 1.5 puntos: eso no es merma, es una
// receta mal capturada o consumo sin venta (la evidencia del #226).
func SuggestExpectedWaste(pcts []*decimal.Decimal) (*decimal.Decimal, bool) {
	var vals []decimal.Decimal
	for _, p := range pcts {
		if p != nil {
			if p.Abs().GreaterThan(reviewAbove) {
				return nil, true
			}
			vals = append(vals, *p)
		}
	}
	if len(pcts) < 3 || len(vals) < 3 {
		return nil, false
	}
	vals = vals[:3]
	lo, hi, sum := vals[0], vals[0], decimal.Zero
	for _, v := range vals {
		lo, hi, sum = decimal.Min(lo, v), decimal.Max(hi, v), sum.Add(v)
	}
	if hi.Sub(lo).GreaterThan(suggestRange) {
		return nil, true
	}
	for _, v := range vals {
		loss := v.Neg()
		if loss.LessThan(suggestMin) || loss.GreaterThan(suggestMax) {
			return nil, false
		}
	}
	s := sum.Neg().Div(decimal.NewFromInt(3)).Mul(decimal.NewFromInt(2)).Round(0).Div(decimal.NewFromInt(2))
	return &s, false
}
