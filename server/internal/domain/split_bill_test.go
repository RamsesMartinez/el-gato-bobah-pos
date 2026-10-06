package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

// sumCovered suma lo que cada renglón dice que pagó.
func sumCovered(ls []CoveredLine) decimal.Decimal {
	s := decimal.Zero
	for _, l := range ls {
		s = s.Add(l.Amount)
	}
	return s
}

// line arma un renglón vivo con precio por pieza (ya con modificadores) y piezas cubiertas.
func line(id int64, qty, unit, covered string) SelectionLine {
	return SelectionLine{LineID: id, Qty: d(qty), UnitPrice: d(unit), CoveredQty: d(covered)}
}

func pick(id int64, qty string) SelectedPieces { return SelectedPieces{LineID: id, Qty: d(qty)} }

// El monto de una selección lo decide el servidor, y cada caso dice qué peso se duplicaría (o se
// perdería) si la regla fallara. La invariante que se revisa en TODOS: lo que la cobertura dice que
// pagó cada renglón suma exactamente el monto del pago —salvo el envío, que no es un producto—; si
// no, la suma por producto de un reporte no cuadra contra el corte.
func TestSelectionAmount(t *testing.T) {
	// La mesa del incidente, reducida: tres bebidas de $110, $89 y $45, sin descuento ni envío.
	mesa := []SelectionLine{line(1, "1", "110", "0"), line(2, "2", "89", "0"), line(3, "1", "45", "0")}

	cases := []struct {
		name       string
		in         SelectionInput
		wantAmount string
		wantLines  map[int64]string // monto por renglón; nil = no se revisa renglón por renglón
		wantCover  string           // Σ cobertura; "" = igual al monto
	}{
		{
			// Si se cobrara el renglón entero, el cliente pagaría también la pieza de su compañero.
			name:       "una pieza",
			in:         SelectionInput{Lines: mesa, Selection: []SelectedPieces{pick(1, "1")}, Subtotal: d("333"), Outstanding: d("333")},
			wantAmount: "110", wantLines: map[int64]string{1: "110"},
		},
		{
			name:       "una de las dos piezas de un renglón",
			in:         SelectionInput{Lines: mesa, Selection: []SelectedPieces{pick(2, "1")}, Subtotal: d("333"), Outstanding: d("333")},
			wantAmount: "89", wantLines: map[int64]string{2: "89"},
		},
		{
			// Sin repartir el descuento, la suma de los pagos pasaría del total por $33.30.
			name: "descuento repartido en proporción al precio",
			in: SelectionInput{Lines: mesa, Selection: []SelectedPieces{pick(1, "1")}, Discount: d("33.30"),
				Subtotal: d("333"), Outstanding: d("299.70")},
			wantAmount: "99", wantLines: map[int64]string{1: "99"},
		},
		{
			// [A1] Tres renglones de $33.33 con descuento: redondear cada uno por su lado deja un
			// centavo que la cobertura no explica. El residuo cae en el último renglón.
			name: "el residuo de redondeo cae en el último renglón en una selección parcial",
			in: SelectionInput{
				Lines:     []SelectionLine{line(1, "1", "33.33", "0"), line(2, "1", "33.33", "0"), line(3, "1", "33.33", "0"), line(4, "1", "100", "0")},
				Selection: []SelectedPieces{pick(1, "1"), pick(2, "1"), pick(3, "1")},
				Discount:  d("10"), Subtotal: d("199.99"), Outstanding: d("189.99"),
			},
			// bruto 99.99; descuento Round2(99.99×10/199.99) = 5.00; monto 94.99
			wantAmount: "94.99", wantLines: map[int64]string{1: "31.66", 2: "31.66", 3: "31.67"},
		},
		{
			// La última selección cobra lo que falta exacto: si cobrara su bruto descontado, el
			// envío y el centavo del redondeo se quedarían debiéndose sin producto que cobrar.
			name: "cubrir todo lo pendiente cobra exactamente lo que falta, con envío",
			in: SelectionInput{
				Lines:     []SelectionLine{line(1, "1", "110", "1"), line(2, "2", "89", "0"), line(3, "1", "45", "0")},
				Selection: []SelectedPieces{pick(2, "2"), pick(3, "1")},
				Subtotal:  d("333"), Shipping: d("30"), Outstanding: d("253"),
			},
			wantAmount: "253", wantLines: map[int64]string{2: "178", 3: "45"}, wantCover: "223",
		},
		{
			// El centavo de repartir el descuento entre pagos: los dos primeros pagaron $30.00
			// cada uno y el tercero descontado daría $30.01, un centavo más de lo que falta. El
			// último lo absorbe y su renglón dice lo que de verdad entró.
			name: "cubrir todo lo pendiente absorbe el centavo en el último renglón",
			in: SelectionInput{
				Lines:     []SelectionLine{line(1, "1", "33.33", "1"), line(2, "1", "33.33", "1"), line(3, "1", "33.34", "0")},
				Selection: []SelectedPieces{pick(3, "1")},
				Discount:  d("10"), Subtotal: d("100"), Outstanding: d("30"),
			},
			wantAmount: "30", wantLines: map[int64]string{3: "30"},
		},
		{
			// [H6] Antes se cobraron $100 sin elegir productos. Cubrir todo cobra lo que falta
			// (menos que el bruto) y cada renglón paga en proporción; si cada uno llevara su bruto,
			// la cobertura diría que se cobró $100 más de lo que entró.
			name: "cubrir todo tras un pago por monto prorratea por renglón",
			in: SelectionInput{
				Lines:     mesa,
				Selection: []SelectedPieces{pick(1, "1"), pick(2, "2"), pick(3, "1")},
				Subtotal:  d("333"), Outstanding: d("233"), PaidWithoutProducts: d("100"),
			},
			// 110×233/333 = 76.97; 178×233/333 = 124.55; resto 31.48
			wantAmount: "233", wantLines: map[int64]string{1: "76.97", 2: "124.55", 3: "31.48"},
		},
		{
			// [H7] Ya no queda pieza sin cubrir pero sí saldo (se devolvió un pago por monto). No
			// es una selección vacía: si se rechazara, el pedido se quedaría debiendo sin salida.
			name: "todo lo que falta sin piezas por cubrir cobra el saldo sin cobertura",
			in: SelectionInput{
				Lines:        []SelectionLine{line(1, "1", "110", "1"), line(2, "2", "89", "2")},
				AllRemaining: true, Subtotal: d("288"), Outstanding: d("50"),
			},
			wantAmount: "50", wantLines: map[int64]string{}, wantCover: "0",
		},
		{
			name: "todo lo que falta cubre las piezas sin cubrir",
			in: SelectionInput{
				Lines:        []SelectionLine{line(1, "1", "110", "1"), line(2, "2", "89", "1"), line(3, "1", "45", "0")},
				AllRemaining: true, Subtotal: d("333"), Outstanding: d("134"),
			},
			wantAmount: "134", wantLines: map[int64]string{2: "89", 3: "45"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := SelectionAmount(c.in)
			if err != nil {
				t.Fatalf("SelectionAmount: %v", err)
			}
			if !got.Amount.Equal(d(c.wantAmount)) {
				t.Fatalf("monto = %s, quiere %s", got.Amount, c.wantAmount)
			}
			if got.Lines == nil {
				t.Fatal("Lines = nil: viajaría como null")
			}
			wantCover := c.wantCover
			if wantCover == "" {
				wantCover = c.wantAmount
			}
			if s := sumCovered(got.Lines); !s.Equal(d(wantCover)) {
				t.Fatalf("Σ cobertura = %s, quiere %s: el reporte por producto no cuadraría con el corte", s, wantCover)
			}
			if c.wantLines != nil {
				if len(got.Lines) != len(c.wantLines) {
					t.Fatalf("renglones = %+v, quiere %v", got.Lines, c.wantLines)
				}
				for _, l := range got.Lines {
					if !l.Amount.Equal(d(c.wantLines[l.LineID])) {
						t.Errorf("renglón %d = %s, quiere %s", l.LineID, l.Amount, c.wantLines[l.LineID])
					}
				}
			}
		})
	}
}

// Los rechazos. Cada uno cobraría dinero que no existe o una pieza dos veces.
func TestSelectionAmountRejections(t *testing.T) {
	mesa := []SelectionLine{line(1, "1", "110", "1"), line(2, "2", "89", "0"), line(3, "1", "45", "0")}
	cases := []struct {
		name    string
		in      SelectionInput
		want    error
		wantMsg string
	}{
		{
			// Tras $150 por monto, cobrar $178 de productos pasaría de lo que falta: el pedido
			// quedaría sobrepagado por dinero que nadie le debía al negocio.
			name: "pasa de lo que falta tras un pago por monto",
			in: SelectionInput{Lines: mesa, Selection: []SelectedPieces{pick(2, "2")}, Subtotal: d("333"),
				Outstanding: d("73"), PaidWithoutProducts: d("150")},
			want:    ErrCobroExcede,
			wantMsg: "Ya se cobraron $150.00 sin elegir productos. Esta selección pasa de lo que falta: usa «Todo lo que falta»",
		},
		{
			name:    "selección vacía",
			in:      SelectionInput{Lines: mesa, Subtotal: d("333"), Outstanding: d("223")},
			want:    ErrEmptySelection,
			wantMsg: "Elige qué productos paga",
		},
		{
			name:    "cero piezas",
			in:      SelectionInput{Lines: mesa, Selection: []SelectedPieces{pick(2, "0")}, Subtotal: d("333"), Outstanding: d("223")},
			want:    ErrEmptySelection,
			wantMsg: "Elige qué productos paga",
		},
		{
			name:    "más piezas de las que tiene el renglón",
			in:      SelectionInput{Lines: mesa, Selection: []SelectedPieces{pick(2, "3")}, Subtotal: d("333"), Outstanding: d("223")},
			want:    ErrEmptySelection,
			wantMsg: "Elige qué productos paga",
		},
		{
			// La pieza ya la pagó otra persona: cobrarla otra vez es el incidente.
			name:    "pieza ya cubierta",
			in:      SelectionInput{Lines: mesa, Selection: []SelectedPieces{pick(1, "1")}, Subtotal: d("333"), Outstanding: d("223")},
			want:    ErrPieceAlreadyPaid,
			wantMsg: "Ese producto ya se pagó",
		},
		{
			// El mismo renglón dos veces en la selección suma: no se cuela como dos pagos de uno.
			name:    "el mismo renglón repetido no esquiva el tope",
			in:      SelectionInput{Lines: mesa, Selection: []SelectedPieces{pick(3, "1"), pick(3, "1")}, Subtotal: d("333"), Outstanding: d("223")},
			want:    ErrEmptySelection,
			wantMsg: "Elige qué productos paga",
		},
		{
			name:    "renglón de otro pedido o cancelado",
			in:      SelectionInput{Lines: mesa, Selection: []SelectedPieces{pick(99, "1")}, Subtotal: d("333"), Outstanding: d("223")},
			want:    ErrNotFound,
			wantMsg: "Ese producto ya no está en el pedido",
		},
		{
			name:    "nada que cobrar",
			in:      SelectionInput{Lines: []SelectionLine{line(1, "1", "110", "1")}, AllRemaining: true, Subtotal: d("110"), Outstanding: d("0")},
			want:    ErrPedidoYaPagado,
			wantMsg: "ese pedido ya está cobrado",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := SelectionAmount(c.in)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, quiere %v", err, c.want)
			}
			if msg := stripSentinel(err.Error()); msg != c.wantMsg {
				t.Fatalf("texto = %q, quiere %q", msg, c.wantMsg)
			}
		})
	}
}

// stripSentinel imita lo que hace httpapi.Error con el texto: quien opera no ve el sentinel base.
func stripSentinel(msg string) string {
	for _, p := range []string{"conflicto: ", "datos inválidos: ", "no encontrado: "} {
		msg = strings.ReplaceAll(msg, p, "")
	}
	return msg
}

// Partes iguales sobre lo que falta: la última absorbe el centavo. Si no lo absorbiera, $100 entre
// tres dejaría $0.01 que nadie puede cobrar y que la barra seguiría sumando.
func TestSplitParts(t *testing.T) {
	cases := []struct {
		outstanding string
		of          int
		want        []string
	}{
		{"100", 3, []string{"33.33", "33.33", "33.34"}},
		{"531", 3, []string{"177", "177", "177"}},
		{"0.05", 2, []string{"0.02", "0.03"}},
		{"10", 12, []string{"0.83", "0.83", "0.83", "0.83", "0.83", "0.83", "0.83", "0.83", "0.83", "0.83", "0.83", "0.87"}},
	}
	for _, c := range cases {
		got, err := SplitParts(d(c.outstanding), c.of)
		if err != nil {
			t.Fatalf("SplitParts(%s, %d): %v", c.outstanding, c.of, err)
		}
		sum := decimal.Zero
		for i, p := range got {
			if !p.Equal(d(c.want[i])) {
				t.Errorf("SplitParts(%s, %d)[%d] = %s, quiere %s", c.outstanding, c.of, i, p, c.want[i])
			}
			sum = sum.Add(p)
		}
		if !sum.Equal(d(c.outstanding)) {
			t.Errorf("SplitParts(%s, %d) suma %s: las partes no cubren lo que falta", c.outstanding, c.of, sum)
		}
	}
	for _, bad := range []struct {
		outstanding string
		of          int
	}{{"100", 1}, {"100", 13}, {"0.05", 12}, {"0", 2}} {
		if _, err := SplitParts(d(bad.outstanding), bad.of); !errors.Is(err, ErrValidation) {
			t.Errorf("SplitParts(%s, %d) = %v, quiere ErrValidation", bad.outstanding, bad.of, err)
		}
	}
}

// Cada parte se calcula sobre lo que falta EN ESE MOMENTO, entre las partes que quedan; la última
// que queda se lleva el residuo, sin importar en qué orden se cobren.
func TestSplitPartAmount(t *testing.T) {
	cases := []struct {
		name        string
		outstanding string
		part, of    int
		charged     []int
		want        string
	}{
		{"primera de tres", "100", 1, 3, nil, "33.33"},
		{"segunda de tres", "66.67", 2, 3, []int{1}, "33.33"},
		{"la última absorbe el centavo", "33.34", 3, 3, []int{1, 2}, "33.34"},
		{"en desorden, la que queda absorbe", "33.34", 1, 3, []int{3, 2}, "33.34"},
		// Entre una parte y otra se cobró algo por monto: la parte se recalcula sobre lo vivo.
		{"recalcula sobre lo que falta", "50", 2, 3, []int{1}, "25"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := SplitPartAmount(d(c.outstanding), c.part, c.of, c.charged)
			if err != nil {
				t.Fatalf("SplitPartAmount: %v", err)
			}
			if !got.Equal(d(c.want)) {
				t.Fatalf("= %s, quiere %s", got, c.want)
			}
		})
	}

	// La misma parte dos veces: es la idempotencia de este modo, que la llave no cubre si se rota.
	if _, err := SplitPartAmount(d("66.67"), 1, 3, []int{1}); !errors.Is(err, ErrSplitPartAlreadyCharged) {
		t.Fatalf("parte ya cobrada = %v, quiere ErrSplitPartAlreadyCharged", err)
	}
	for _, bad := range []struct{ part, of int }{{1, 1}, {1, 13}, {0, 3}, {4, 3}} {
		if _, err := SplitPartAmount(d("100"), bad.part, bad.of, nil); !errors.Is(err, ErrValidation) {
			t.Errorf("SplitPartAmount(parte %d de %d) = %v, quiere ErrValidation", bad.part, bad.of, err)
		}
	}
	if _, err := SplitPartAmount(d("0"), 1, 2, nil); !errors.Is(err, ErrPedidoYaPagado) {
		t.Errorf("sin nada que falte = %v, quiere ErrPedidoYaPagado", err)
	}
}
