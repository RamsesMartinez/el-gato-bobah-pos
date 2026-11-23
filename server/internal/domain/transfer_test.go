package domain

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestValidateTransfer(t *testing.T) {
	ing := func(id int64, q string) TransferLine { return TransferLine{ItemType: "ingrediente", ID: id, Qty: d(q)} }
	prod := func(id int64, q string) TransferLine { return TransferLine{ItemType: "producto", ID: id, Qty: d(q)} }
	cases := []struct {
		name     string
		from, to int64
		lines    []TransferLine
		ok       bool
	}{
		{"un renglón", 1, 2, []TransferLine{ing(10, "6")}, true},
		{"insumo y producto", 1, 2, []TransferLine{ing(10, "6"), prod(3, "2")}, true},
		{"origen igual a destino", 1, 1, []TransferLine{ing(10, "6")}, false},
		{"sin renglones", 1, 2, nil, false},
		{"cantidad cero", 1, 2, []TransferLine{ing(10, "0")}, false},
		{"cantidad negativa", 1, 2, []TransferLine{ing(10, "-1")}, false},
		{"cantidad que no cabe", 1, 2, []TransferLine{ing(10, "1e12")}, false},
		{"tipo desconocido", 1, 2, []TransferLine{{ItemType: "otro", ID: 10, Qty: d("1")}}, false},
		{"sin artículo", 1, 2, []TransferLine{ing(0, "1")}, false},
		{"el mismo artículo dos veces", 1, 2, []TransferLine{ing(10, "1"), ing(10, "2")}, false},
		{"almacén sin id", 0, 2, []TransferLine{ing(10, "1")}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ValidateTransfer(c.from, c.to, c.lines)
			if (err == nil) != c.ok {
				t.Fatalf("ValidateTransfer = %v; esperaba ok=%v", err, c.ok)
			}
			if err != nil && !errors.Is(err, ErrValidation) {
				t.Fatalf("un traspaso mal formado es 400, llegó %v", err)
			}
		})
	}
	// Tope de renglones (revisión de la 094): por HTTP cabían miles en un cuerpo, cada uno tres inserts con
	// los almacenes tomados.
	many := make([]TransferLine, MaxTransferLines+1)
	for i := range many {
		many[i] = ing(int64(i+1), "1")
	}
	if _, err := ValidateTransfer(1, 2, many[:MaxTransferLines]); err != nil {
		t.Fatalf("%d renglones caben: %v", MaxTransferLines, err)
	}
	if _, err := ValidateTransfer(1, 2, many); !errors.Is(err, ErrValidation) {
		t.Fatalf("%d renglones = %v; quiere 400", MaxTransferLines+1, err)
	}
	got, _ := ValidateTransfer(1, 2, []TransferLine{ing(10, "6.123456")})
	if !got[0].Qty.Equal(d("6.1235")) {
		t.Fatalf("la cantidad se redondea a 4 decimales: %s", got[0].Qty)
	}
}

// Lo que dejaría el origen en negativo se pregunta antes; con la confirmación, pasa.
func TestTransferShortfalls(t *testing.T) {
	lines := []TransferLine{{ItemType: "ingrediente", ID: 10, Qty: d("6")}, {ItemType: "producto", ID: 3, Qty: d("2")}}
	onHand := map[StockKey]decimal.Decimal{{ItemType: "ingrediente", ID: 10}: d("20"), {ItemType: "producto", ID: 3}: d("1")}
	short := TransferShortfalls(lines, onHand)
	if len(short) != 1 || short[0].ID != 3 || !short[0].Available.Equal(d("1")) {
		t.Fatalf("faltantes = %+v; quiere solo el producto 3 con 1 disponible", short)
	}
	if err := CheckTransferStock(lines, onHand, false); !errors.Is(err, ErrTransferNeedsConfirm) {
		t.Fatalf("sin confirmar = %v", err)
	}
	if err := CheckTransferStock(lines, onHand, true); err != nil {
		t.Fatalf("confirmado = %v", err)
	}
	var nc TransferNeedsConfirm
	if !errors.As(CheckTransferStock(lines, onHand, false), &nc) || len(nc.Lines) != 1 {
		t.Fatal("el rechazo dice qué renglón no alcanza")
	}
	// Un artículo sin fila de existencia vale cero.
	if got := TransferShortfalls([]TransferLine{{ItemType: "ingrediente", ID: 99, Qty: d("1")}}, onHand); len(got) != 1 || !got[0].Available.IsZero() {
		t.Fatalf("sin existencia = %+v", got)
	}
}

// Dos insumos distintos que resultan ser el mismo después de seguir una unión (A se unió a B) son el
// mismo artículo dos veces: se dice así, no «no existe» (revisión de la 094).
func TestTransferDuplicates(t *testing.T) {
	a := TransferLine{ItemType: "ingrediente", ID: 7, Qty: d("1")}
	b := TransferLine{ItemType: "producto", ID: 7, Qty: d("1")}
	if err := TransferDuplicates([]TransferLine{a, b}); err != nil {
		t.Fatalf("insumo 7 y producto 7 son distintos: %v", err)
	}
	err := TransferDuplicates([]TransferLine{a, a})
	if !errors.Is(err, ErrValidation) || err.Error() == "" {
		t.Fatalf("el mismo insumo dos veces = %v; quiere 400", err)
	}
}
