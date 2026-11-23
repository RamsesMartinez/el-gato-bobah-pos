package domain

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

// ErrOutOfStock: lo que se pide de un producto con «vender sin existencia» apagado no alcanza con
// la existencia del lugar donde se vende (spec 092).
var ErrOutOfStock = errors.New("se agotó")

// OutOfStock nombra el producto que se agotó y cuántas piezas quedan de él, para que la pantalla
// lo diga sin escarbar el texto.
type OutOfStock struct {
	ProductID int64
	Name      string
	Left      decimal.Decimal
}

func (e OutOfStock) Error() string {
	if e.Left.IsPositive() {
		return fmt.Sprintf("Se agotó %s: queda %s", e.Name, e.Left)
	}
	return fmt.Sprintf("Se agotó %s: no queda", e.Name)
}

func (e OutOfStock) Unwrap() error { return ErrOutOfStock }

// StockKey es un renglón de existencia: un insumo o un producto con existencia propia.
type StockKey struct {
	ItemType string // "ingrediente" | "producto", como stock_levels.item_type
	ID       int64
}

// StockLevels es la existencia del lugar donde se vende. Un renglón que no está vale cero: la fila
// de existencia nace con el primer movimiento, y lo que nunca entró no hay.
type StockLevels map[StockKey]decimal.Decimal

// StockCheckLine es un renglón de venta de un producto con «vender sin existencia» apagado.
type StockCheckLine struct {
	ProductID int64
	Name      string
	Qty       decimal.Decimal
}

// keyOf es el renglón de existencia de lo que sale del almacén.
func keyOf(sd StockDelta) StockKey {
	if sd.ProductID != nil {
		return StockKey{ItemType: "producto", ID: *sd.ProductID}
	}
	return StockKey{ItemType: "ingrediente", ID: *sd.IngredientID}
}

// perUnit es lo que una pieza del producto, sin extras, saca de cada renglón. Los extras no cuentan:
// dependen de lo que elige el cliente, no del producto.
func perUnit(g StockGraph, productID int64) map[StockKey]decimal.Decimal {
	deltas, _ := g.ExpandSale(SaleLine{ProductID: productID, Qty: decimal.NewFromInt(1)})
	out := map[StockKey]decimal.Decimal{}
	for _, sd := range deltas {
		k := keyOf(sd)
		out[k] = out[k].Add(sd.Qty)
	}
	return out
}

// SoldOut dice si con la existencia de hoy no alcanza para una pieza del producto (sin extras). Lo
// que no descuenta nada nunca se agota. Quien llama ya sabe que el producto tiene «vender sin
// existencia» apagado.
func SoldOut(g StockGraph, levels StockLevels, productID int64) bool {
	for k, need := range perUnit(g, productID) {
		if levels[k].LessThan(need) {
			return true
		}
	}
	return false
}

// StockNeeds suma lo que piden los renglones de cada renglón de existencia: es lo que el chequeo de
// la venta tiene que bloquear antes de comparar.
func StockNeeds(g StockGraph, lines []StockCheckLine) map[StockKey]decimal.Decimal {
	out := map[StockKey]decimal.Decimal{}
	for _, l := range lines {
		for k, q := range perUnit(g, l.ProductID) {
			out[k] = out[k].Add(q.Mul(l.Qty))
		}
	}
	return out
}

// CheckStock rechaza la venta si lo que piden los renglones, sumados, supera la existencia de algún
// renglón de almacén. Nombra al primer producto (en el orden de la cuenta) que pide de ese renglón,
// con cuántas piezas suyas quedan.
func CheckStock(g StockGraph, levels StockLevels, lines []StockCheckLine) error {
	needs := StockNeeds(g, lines)
	for _, l := range lines {
		unit := perUnit(g, l.ProductID)
		for k := range unit {
			if !levels[k].LessThan(needs[k]) {
				continue
			}
			return OutOfStock{ProductID: l.ProductID, Name: l.Name, Left: piecesLeft(unit, levels)}
		}
	}
	return nil
}

// piecesLeft es cuántas piezas enteras del producto alcanzan con la existencia, nunca negativo.
func piecesLeft(unit map[StockKey]decimal.Decimal, levels StockLevels) decimal.Decimal {
	var left *decimal.Decimal
	for k, need := range unit {
		if !need.IsPositive() {
			continue
		}
		n := levels[k].Div(need).Floor()
		if n.IsNegative() {
			n = decimal.Zero
		}
		if left == nil || n.LessThan(*left) {
			left = &n
		}
	}
	if left == nil {
		return decimal.Zero
	}
	return *left
}
