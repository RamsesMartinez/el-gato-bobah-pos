package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

var (
	// ErrTransferNeedsConfirm: algún renglón deja el origen en negativo. La pantalla lo pregunta y, si
	// quien traspasa confirma (la mercancía sí está, el sistema no la tenía), se manda de nuevo.
	ErrTransferNeedsConfirm = fmt.Errorf("%w: el origen no tiene suficiente", ErrConflict)
	// ErrTransferKeyReused: la tableta mandó otro traspaso con la clave de uno que ya existe. No es un
	// reintento (el cuerpo es distinto): se rechaza en vez de devolver el traspaso que no pidió.
	ErrTransferKeyReused = fmt.Errorf("%w: esa clave ya se usó para otro traspaso", ErrConflict)
)

// MaxTransferLines es lo más que lleva un traspaso. Uno de verdad mueve unos cuantos artículos; el tope
// existe para que un cuerpo enorme no tenga tomados los almacenes mientras inserta miles de renglones.
const MaxTransferLines = 100

// TransferLine es un renglón de traspaso: un artículo (insumo o producto con existencia propia) y la
// cantidad en su unidad base.
type TransferLine struct {
	ItemType string // "ingrediente" | "producto"
	ID       int64
	Qty      decimal.Decimal
}

// Key es el renglón de existencia que mueve.
func (l TransferLine) Key() StockKey { return StockKey{ItemType: l.ItemType, ID: l.ID} }

// ValidateTransfer revisa la forma de un traspaso y devuelve los renglones con la cantidad redondeada
// como la columna (4 decimales). Un artículo repetido se rechaza en vez de sumarse: dos renglones
// iguales en la pantalla casi siempre son un dedazo, y sumarlos en silencio movería el doble.
func ValidateTransfer(from, to int64, lines []TransferLine) ([]TransferLine, error) {
	if from <= 0 || to <= 0 {
		return nil, fmt.Errorf("%w: falta el almacén de origen o de destino", ErrValidation)
	}
	if from == to {
		return nil, fmt.Errorf("%w: el origen y el destino son el mismo almacén", ErrValidation)
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("%w: el traspaso no lleva artículos", ErrValidation)
	}
	if len(lines) > MaxTransferLines {
		return nil, fmt.Errorf("%w: un traspaso lleva hasta %d artículos", ErrValidation, MaxTransferLines)
	}
	seen := map[StockKey]bool{}
	out := make([]TransferLine, 0, len(lines))
	for _, l := range lines {
		if l.ItemType != "ingrediente" && l.ItemType != "producto" {
			return nil, fmt.Errorf("%w: tipo de artículo desconocido", ErrValidation)
		}
		if l.ID <= 0 {
			return nil, fmt.Errorf("%w: falta el artículo", ErrValidation)
		}
		q := Round4(l.Qty)
		if !q.IsPositive() || !ValidQty(q, MaxStockQty, false) {
			return nil, fmt.Errorf("%w: cantidad inválida", ErrValidation)
		}
		if seen[l.Key()] {
			return nil, fmt.Errorf("%w: el mismo artículo va dos veces", ErrValidation)
		}
		seen[l.Key()] = true
		out = append(out, TransferLine{ItemType: l.ItemType, ID: l.ID, Qty: q})
	}
	return out, nil
}

// TransferDuplicates rechaza el mismo artículo dos veces. Se vuelve a revisar después de seguir las
// uniones de insumos: dos renglones distintos (A y B) son el mismo si A se unió a B.
func TransferDuplicates(lines []TransferLine) error {
	seen := map[StockKey]bool{}
	for _, l := range lines {
		if seen[l.Key()] {
			return fmt.Errorf("%w: el mismo artículo va dos veces", ErrValidation)
		}
		seen[l.Key()] = true
	}
	return nil
}

// TransferShort es un renglón que el origen no alcanza a cubrir, con lo que hay.
type TransferShort struct {
	ItemType  string
	ID        int64
	Available decimal.Decimal
}

// TransferShortfalls son los renglones que dejarían el origen en negativo. Un artículo sin fila de
// existencia vale cero.
func TransferShortfalls(lines []TransferLine, onHand map[StockKey]decimal.Decimal) []TransferShort {
	var out []TransferShort
	for _, l := range lines {
		have := onHand[l.Key()]
		if have.LessThan(l.Qty) {
			out = append(out, TransferShort{ItemType: l.ItemType, ID: l.ID, Available: have})
		}
	}
	return out
}

// TransferNeedsConfirm lleva los renglones que no alcanzan, para que la pantalla los señale.
type TransferNeedsConfirm struct{ Lines []TransferShort }

func (e TransferNeedsConfirm) Error() string {
	return "el origen no tiene suficiente de algunos artículos: confirma si de verdad están ahí"
}
func (e TransferNeedsConfirm) Unwrap() error { return ErrTransferNeedsConfirm }

// CheckTransferStock pide confirmar cuando algún renglón deja el origen en negativo.
func CheckTransferStock(lines []TransferLine, onHand map[StockKey]decimal.Decimal, confirmed bool) error {
	if confirmed {
		return nil
	}
	if short := TransferShortfalls(lines, onHand); len(short) > 0 {
		return TransferNeedsConfirm{Lines: short}
	}
	return nil
}
