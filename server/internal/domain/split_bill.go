package domain

import (
	"fmt"
	"slices"
	"strings"

	"github.com/shopspring/decimal"
)

// MaxSplitParts es en cuántas personas se reparte como máximo: más allá, repartir a mano deja de
// tener sentido y se cobra por monto. Vive aquí y no en un check de la base para poder moverlo sin
// migración.
const MaxSplitParts = 12

// SelectionLine es un renglón vivo del pedido tal como lo necesita el cálculo de una selección.
type SelectionLine struct {
	LineID int64
	Qty    decimal.Decimal
	// UnitPrice es el precio de una pieza con sus modificadores.
	UnitPrice decimal.Decimal
	// CoveredQty son las piezas que ya cubre algún pago vivo.
	CoveredQty decimal.Decimal
}

// SelectedPieces es cuántas piezas de un renglón paga esta persona.
type SelectedPieces struct {
	LineID int64
	Qty    decimal.Decimal
}

// SelectionInput es todo lo que decide el monto de un cobro por productos.
type SelectionInput struct {
	Lines     []SelectionLine
	Selection []SelectedPieces
	// AllRemaining cubre todas las piezas aún sin cubrir, en lugar de Selection.
	AllRemaining bool
	Discount     decimal.Decimal
	Subtotal     decimal.Decimal
	Shipping     decimal.Decimal
	// Outstanding es lo que falta del pedido (PorCobrar).
	Outstanding decimal.Decimal
	// PaidWithoutProducts es lo que se cobró por monto, sin elegir productos. Solo sirve para
	// decirle a quien opera por qué la selección no cabe.
	PaidWithoutProducts decimal.Decimal
}

// CoveredLine es lo que un pago cubre de un renglón: cuántas piezas y cuánto dinero.
type CoveredLine struct {
	LineID int64
	Qty    decimal.Decimal
	Amount decimal.Decimal
}

// SelectionResult es el monto del pago y su cobertura por renglón. Lines nunca es nil.
type SelectionResult struct {
	Amount decimal.Decimal
	Lines  []CoveredLine
}

// SelectionAmount calcula cuánto se cobra por una selección de piezas y cuánto de ese monto
// corresponde a cada renglón.
//
// Las reglas, y el peso que cada una evita contar dos veces:
//   - el descuento del pedido se reparte en proporción al bruto: sin eso la suma de los pagos
//     pasaría del total por el monto del descuento;
//   - si la selección cubre todas las piezas aún sin cubrir, cobra exactamente lo que falta —el
//     envío y el centavo del redondeo los absorbe el último pago—, porque si no quedaría saldo sin
//     producto con que cobrarlo;
//   - lo que dice cada renglón suma exacto el monto del pago menos el envío: el residuo de redondear
//     renglón por renglón cae en el último, y si antes hubo pagos por monto (se cobra menos que el
//     bruto) cada renglón se escala en proporción. Si no, el reporte por producto no cuadra con el
//     corte.
func SelectionAmount(in SelectionInput) (SelectionResult, error) {
	picked, err := piecesToCharge(in)
	if err != nil {
		return SelectionResult{}, err
	}
	if !in.Outstanding.IsPositive() {
		return SelectionResult{}, ErrPedidoYaPagado
	}
	if len(picked) == 0 {
		// Solo llega aquí con AllRemaining: no queda pieza sin cubrir pero sí saldo (se devolvió un
		// pago por monto). Se cobra el saldo sin cobertura; rechazarlo dejaría el pedido sin salida.
		return SelectionResult{Amount: in.Outstanding, Lines: []CoveredLine{}}, nil
	}

	gross := decimal.Zero
	nets := make([]decimal.Decimal, len(picked))
	for i, p := range picked {
		g := p.Qty.Mul(p.line.UnitPrice)
		gross = gross.Add(g)
		nets[i] = Round2(g.Sub(discountShare(g, in.Discount, in.Subtotal)))
	}
	amount := Round2(gross).Sub(Round2(discountShare(gross, in.Discount, in.Subtotal)))
	linesTarget := amount

	if coversEverythingLeft(in.Lines, picked) {
		amount = in.Outstanding
		// El envío no es un producto: se queda fuera de la cobertura de los renglones.
		linesTarget = decimal.Max(decimal.Zero, amount.Sub(in.Shipping))
	} else if amount.GreaterThan(in.Outstanding) {
		return SelectionResult{}, Reword(ErrCobroExcede, fmt.Sprintf(
			"Ya se cobraron $%s sin elegir productos. Esta selección pasa de lo que falta: usa «Todo lo que falta»",
			Round2(in.PaidWithoutProducts).StringFixed(2)))
	}

	return SelectionResult{Amount: amount, Lines: spreadOverLines(picked, nets, linesTarget)}, nil
}

type pickedLine struct {
	line SelectionLine
	Qty  decimal.Decimal
}

// piecesToCharge resuelve la selección (o «todo lo que falta») a renglones del pedido, en el orden
// del pedido, y rechaza piezas que no existen o ya están cubiertas.
func piecesToCharge(in SelectionInput) ([]pickedLine, error) {
	if in.AllRemaining {
		out := []pickedLine{}
		for _, l := range in.Lines {
			if free := l.Qty.Sub(l.CoveredQty); free.IsPositive() {
				out = append(out, pickedLine{line: l, Qty: free})
			}
		}
		return out, nil
	}
	if len(in.Selection) == 0 {
		return nil, ErrEmptySelection
	}
	// El mismo renglón dos veces se suma: así no se cuela como dos selecciones que caben cada una.
	want := map[int64]decimal.Decimal{}
	for _, s := range in.Selection {
		if !s.Qty.IsPositive() {
			return nil, ErrEmptySelection
		}
		want[s.LineID] = want[s.LineID].Add(s.Qty)
	}
	out := []pickedLine{}
	for _, l := range in.Lines {
		q, ok := want[l.LineID]
		if !ok {
			continue
		}
		delete(want, l.LineID)
		if q.GreaterThan(l.Qty) {
			return nil, ErrEmptySelection
		}
		if q.GreaterThan(l.Qty.Sub(l.CoveredQty)) {
			return nil, ErrPieceAlreadyPaid
		}
		out = append(out, pickedLine{line: l, Qty: q})
	}
	if len(want) > 0 {
		return nil, fmt.Errorf("%w: Ese producto ya no está en el pedido", ErrNotFound)
	}
	return out, nil
}

// discountShare es la parte del descuento del pedido que le toca a un bruto, sin redondear.
func discountShare(gross, discount, subtotal decimal.Decimal) decimal.Decimal {
	if !discount.IsPositive() || !subtotal.IsPositive() {
		return decimal.Zero
	}
	return gross.Mul(discount).Div(subtotal)
}

// coversEverythingLeft dice si con esta selección no queda pieza sin cubrir en el pedido.
func coversEverythingLeft(lines []SelectionLine, picked []pickedLine) bool {
	for _, l := range lines {
		free := l.Qty.Sub(l.CoveredQty)
		if !free.IsPositive() {
			continue
		}
		i := slices.IndexFunc(picked, func(p pickedLine) bool { return p.line.LineID == l.LineID })
		if i < 0 || picked[i].Qty.LessThan(free) {
			return false
		}
	}
	return true
}

// spreadOverLines reparte target entre los renglones: cada uno su neto, o su neto escalado si
// target es menor que la suma, y el residuo en el último para que la suma dé target exacto.
func spreadOverLines(picked []pickedLine, nets []decimal.Decimal, target decimal.Decimal) []CoveredLine {
	sumNets := decimal.Sum(decimal.Zero, nets...)
	out := make([]CoveredLine, len(picked))
	assigned := decimal.Zero
	for i, p := range picked {
		amt := nets[i]
		if target.LessThan(sumNets) && sumNets.IsPositive() {
			amt = Round2(nets[i].Mul(target).Div(sumNets))
		}
		if i == len(picked)-1 {
			amt = target.Sub(assigned)
		}
		assigned = assigned.Add(amt)
		out[i] = CoveredLine{LineID: p.line.LineID, Qty: p.Qty, Amount: amt}
	}
	return out
}

// SplitParts reparte lo que falta entre `of` personas, con el residuo en la última.
//
// `falta/of` redondeado no suma lo que falta: $100 entre tres son tres de $33.33 = $99.99, y ese
// centavo quedaría sin nadie que lo pague. Rechaza que alguna parte quede en cero: cobrar $0 no es
// cobrar.
func SplitParts(outstanding decimal.Decimal, of int) ([]decimal.Decimal, error) {
	if of < 2 || of > MaxSplitParts {
		return nil, fmt.Errorf("%w: se reparte entre 2 y %d personas", ErrValidation, MaxSplitParts)
	}
	total := Round2(outstanding)
	part := total.Mul(decimal.NewFromInt(100)).Div(decimal.NewFromInt(int64(of))).Floor().Div(decimal.NewFromInt(100))
	if !part.IsPositive() {
		return nil, fmt.Errorf("%w: lo que falta no alcanza para %d partes", ErrValidation, of)
	}
	parts := make([]decimal.Decimal, of)
	for i := range parts {
		parts[i] = part
	}
	parts[of-1] = total.Sub(part.Mul(decimal.NewFromInt(int64(of - 1))))
	return parts, nil
}

// SplitPartAmount es cuánto se cobra por la parte `part` de `of`, dadas las partes ya cobradas de
// esa serie.
//
// Se calcula sobre lo que falta AHORA, entre las partes que quedan, y no una vez al empezar: entre
// una parte y otra pudo entrar un pago por monto o un producto, y unas partes fijas dejarían de
// sumar. Así la última parte que queda, sea cual sea, se lleva sola el residuo. Una parte ya
// cobrada se rechaza: es la idempotencia de este modo, que la llave del cobro no cubre si la
// pantalla la rota al cambiar de intención.
func SplitPartAmount(outstanding decimal.Decimal, part, of int, charged []int) (decimal.Decimal, error) {
	if of < 2 || of > MaxSplitParts || part < 1 || part > of {
		return decimal.Zero, fmt.Errorf("%w: esa parte no existe", ErrValidation)
	}
	if slices.Contains(charged, part) {
		return decimal.Zero, ErrSplitPartAlreadyCharged
	}
	if !outstanding.IsPositive() {
		return decimal.Zero, ErrPedidoYaPagado
	}
	left := of
	seen := map[int]bool{}
	for _, c := range charged {
		if c >= 1 && c <= of && !seen[c] {
			seen[c] = true
			left--
		}
	}
	if left == 1 {
		return Round2(outstanding), nil
	}
	parts, err := SplitParts(outstanding, left)
	if err != nil {
		return decimal.Zero, err
	}
	return parts[0], nil
}

// ChargeShape es la forma de un cobro: qué decide su monto.
type ChargeShape int

const (
	// ShapeAmount: el monto lo teclea quien cobra.
	ShapeAmount ChargeShape = iota + 1
	// ShapeLines: los productos que paga esta persona.
	ShapeLines
	// ShapeAllRemaining: todo lo que falta.
	ShapeAllRemaining
	// ShapeSplit: una parte de N entre personas.
	ShapeSplit
)

// ChargeShapeOf decide la forma de un cobro. Las formas se excluyen: si llegan dos, cualquiera que
// se respete deja a quien cobra creyendo que se respetó la otra.
func ChargeShapeOf(hasAmount, hasLines, allRemaining, hasSplit bool) (ChargeShape, error) {
	n := 0
	shape := ChargeShape(0)
	for _, s := range []struct {
		on    bool
		shape ChargeShape
	}{{hasAmount, ShapeAmount}, {hasLines, ShapeLines}, {allRemaining, ShapeAllRemaining}, {hasSplit, ShapeSplit}} {
		if s.on {
			n++
			shape = s.shape
		}
	}
	switch {
	case n > 1:
		return 0, ErrOneChargeShape
	case n == 0:
		return 0, ErrValidation
	}
	return shape, nil
}

// CoverageKey es la huella de una selección: renglón y piezas, en orden de renglón y con la escala
// de la columna. Dos envíos del mismo cobro dan la misma huella aunque la pantalla mande los
// renglones en otro orden o escriba «1» en vez de «1.00»; así un reenvío se reconoce y otra
// selección con la misma llave se rechaza.
func CoverageKey(sel []SelectedPieces) string {
	sum := map[int64]decimal.Decimal{}
	ids := []int64{}
	for _, s := range sel {
		if _, ok := sum[s.LineID]; !ok {
			ids = append(ids, s.LineID)
		}
		sum[s.LineID] = sum[s.LineID].Add(s.Qty)
	}
	slices.Sort(ids)
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d:%s", id, Round2(sum[id]).StringFixed(2))
	}
	return strings.Join(parts, ",")
}

// CoveredKey es la huella de lo que un pago ya cubrió, con la misma forma que CoverageKey.
func CoveredKey(lines []CoveredLine) string {
	sel := make([]SelectedPieces, len(lines))
	for i, l := range lines {
		sel[i] = SelectedPieces{LineID: l.LineID, Qty: l.Qty}
	}
	return CoverageKey(sel)
}
