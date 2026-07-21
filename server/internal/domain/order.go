package domain

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
)

var (
	ErrEmptyOrder     = errors.New("el pedido no tiene líneas")
	ErrProductNotSell = errors.New("producto no disponible")
	ErrOptionNotFound = errors.New("opción de modificador no encontrada")
)

// Estados de una orden. "paid" es un campo derivado, no un estado.
const (
	StatusAbierta   = "abierta"
	StatusLista     = "lista"
	StatusEntregada = "entregada"
	StatusCancelada = "cancelada"
	// Devolución de una orden YA entregada. Terminal, y distinto de cancelada: es una
	// pérdida (mercancía consumida, ingreso revertido), no un pedido que nunca se sirvió.
	StatusReembolsada = "reembolsada"
)

// CanRefund indica si una orden puede reembolsarse. Solo desde 'entregada': lo abierto/listo
// se cancela (repone stock); lo ya cancelado o reembolsado es terminal. Separado de
// CanTransition a propósito, para no reabrir el flujo normal de estados.
func CanRefund(current string) bool {
	return current == StatusEntregada
}

// CanTransition indica si una orden puede pasar de current a next. Los estados
// terminales (entregada, cancelada) no permiten más cambios y el estado nunca
// retrocede. Esto hace la cancelación idempotente (cancelada→cancelada = false,
// evita el doble-restock del doble-tap) y cierra el "void después de entregar".
func CanTransition(current, next string) bool {
	switch current {
	case StatusAbierta:
		return next == StatusLista || next == StatusEntregada || next == StatusCancelada
	case StatusLista:
		return next == StatusEntregada || next == StatusCancelada
	default: // entregada, cancelada = terminal
		return false
	}
}

// --- Entradas del cliente (solo IDs + cantidades; los precios los pone el servidor) ---

type OrderLineInput struct {
	ProductID int64
	Qty       decimal.Decimal
	Notes     string
	Modifiers []OrderModInput
}

type OrderModInput struct {
	OptionID int64
	Qty      int
	Portion  string // "A"/"B" para mitad-y-mitad; "" = aplica al producto completo
}

// --- Catálogo priceado cargado del servidor ---

type PricedProduct struct {
	ID     int64
	Name   string
	Price  decimal.Decimal
	Cost   decimal.Decimal
	Active bool
}

type PricedOption struct {
	ID         int64
	Name       string
	PriceDelta decimal.Decimal
	Cost       decimal.Decimal
	GroupTitle string
}

// --- Resultado priceado (snapshots que van a order_lines) ---

type BuiltOrder struct {
	Subtotal decimal.Decimal
	Total    decimal.Decimal
	Lines    []BuiltLine
}

type BuiltLine struct {
	ProductID      int64
	ProductName    string
	Qty            decimal.Decimal
	UnitPrice      decimal.Decimal
	ModifiersTotal decimal.Decimal // por unidad
	UnitCost       decimal.Decimal // por unidad (producto + opciones)
	LineTotal      decimal.Decimal
	Notes          string
	Modifiers      []BuiltMod
}

type BuiltMod struct {
	OptionID   int64
	GroupTitle string
	OptionName string
	Qty        int
	PriceDelta decimal.Decimal
	UnitCost   decimal.Decimal
}

// BuildOrder calcula precios, costos y totales de forma autoritativa en el servidor.
// Los precios del cliente nunca se usan.
func BuildOrder(lines []OrderLineInput, products map[int64]PricedProduct, options map[int64]PricedOption) (BuiltOrder, error) {
	if len(lines) == 0 {
		return BuiltOrder{}, ErrEmptyOrder
	}
	var out BuiltOrder
	for _, in := range lines {
		p, ok := products[in.ProductID]
		if !ok {
			return BuiltOrder{}, fmt.Errorf("%w (id %d)", ErrProductNotSell, in.ProductID)
		}
		if !p.Active {
			return BuiltOrder{}, fmt.Errorf("%w: %s", ErrProductNotSell, p.Name)
		}
		// Redondear a 2dp (order_lines.quantity es numeric(8,2)) y validar el valor ya
		// redondeado: si no, un qty como 0.001 pasa la validación pero Postgres lo coacciona
		// a 0.00 y viola el check (quantity > 0) → 500 en vez de un 400 limpio.
		qty := Round2(in.Qty)
		if !ValidQty(qty, MaxOrderQty, false) {
			return BuiltOrder{}, ErrValidation
		}
		line := BuiltLine{
			ProductID:   p.ID,
			ProductName: p.Name,
			Qty:         qty,
			UnitPrice:   p.Price,
			UnitCost:    p.Cost,
			Notes:       in.Notes,
		}
		var modsPerUnit, modCostPerUnit decimal.Decimal
		for _, m := range in.Modifiers {
			o, ok := options[m.OptionID]
			if !ok {
				return BuiltOrder{}, fmt.Errorf("%w (id %d)", ErrOptionNotFound, m.OptionID)
			}
			q := m.Qty
			if q <= 0 {
				q = 1
			}
			qd := decimal.NewFromInt(int64(q))
			// Cota superior: el modificador se persiste como int16; sin esto un Qty enorme
			// hace wrap (40000 → -25536) y corrompe el ticket sin siquiera dar 500.
			if !ValidQty(qd, MaxOrderQty, false) {
				return BuiltOrder{}, ErrValidation
			}
			modsPerUnit = modsPerUnit.Add(o.PriceDelta.Mul(qd))
			modCostPerUnit = modCostPerUnit.Add(o.Cost.Mul(qd))
			// ponytail: la mitad se refleja prefijando el nombre snapshot ("½A · BBQ").
			// Aflora en cocina/ticket sin migración. Ceiling: solo 2 mitades, sin precio
			// fraccionado por mitad ni líneas hijas (parent_line_id).
			name := o.Name
			if m.Portion == "A" || m.Portion == "B" {
				name = "½" + m.Portion + " · " + o.Name
			}
			line.Modifiers = append(line.Modifiers, BuiltMod{
				OptionID: o.ID, GroupTitle: o.GroupTitle, OptionName: name,
				Qty: q, PriceDelta: o.PriceDelta, UnitCost: o.Cost,
			})
		}
		line.ModifiersTotal = Round2(modsPerUnit)
		line.UnitCost = Round2(p.Cost.Add(modCostPerUnit))
		line.LineTotal = Round2(p.Price.Add(modsPerUnit).Mul(qty))
		out.Lines = append(out.Lines, line)
		out.Subtotal = out.Subtotal.Add(line.LineTotal)
	}
	out.Subtotal = Round2(out.Subtotal)
	out.Total = out.Subtotal // sin descuentos en MVP
	// Aunque cada Qty esté acotada, un precio de catálogo alto × muchas líneas podría
	// desbordar el total: se rechaza antes de tocar el numeric(10,2) (allowZero: un pedido
	// comped puede totalizar 0).
	if !ValidMoney(out.Total, true) {
		return BuiltOrder{}, ErrValidation
	}
	return out, nil
}
