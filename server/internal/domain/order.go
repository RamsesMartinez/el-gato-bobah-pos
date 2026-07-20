package domain

import (
	"errors"
	"fmt"
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
)

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
	Qty       float64
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
	Price  float64
	Cost   float64
	Active bool
}

type PricedOption struct {
	ID         int64
	Name       string
	PriceDelta float64
	Cost       float64
	GroupTitle string
}

// --- Resultado priceado (snapshots que van a order_lines) ---

type BuiltOrder struct {
	Subtotal float64
	Total    float64
	Lines    []BuiltLine
}

type BuiltLine struct {
	ProductID      int64
	ProductName    string
	Qty            float64
	UnitPrice      float64
	ModifiersTotal float64 // por unidad
	UnitCost       float64 // por unidad (producto + opciones)
	LineTotal      float64
	Notes          string
	Modifiers      []BuiltMod
}

type BuiltMod struct {
	OptionID   int64
	GroupTitle string
	OptionName string
	Qty        int
	PriceDelta float64
	UnitCost   float64
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
		if !ValidQty(in.Qty, MaxOrderQty, false) {
			return BuiltOrder{}, ErrValidation
		}
		line := BuiltLine{
			ProductID:   p.ID,
			ProductName: p.Name,
			Qty:         in.Qty,
			UnitPrice:   p.Price,
			UnitCost:    p.Cost,
			Notes:       in.Notes,
		}
		var modsPerUnit, modCostPerUnit float64
		for _, m := range in.Modifiers {
			o, ok := options[m.OptionID]
			if !ok {
				return BuiltOrder{}, fmt.Errorf("%w (id %d)", ErrOptionNotFound, m.OptionID)
			}
			q := m.Qty
			if q <= 0 {
				q = 1
			}
			// Cota superior: el modificador se persiste como int16; sin esto un Qty enorme
			// hace wrap (40000 → -25536) y corrompe el ticket sin siquiera dar 500.
			if !ValidQty(float64(q), MaxOrderQty, false) {
				return BuiltOrder{}, ErrValidation
			}
			modsPerUnit += o.PriceDelta * float64(q)
			modCostPerUnit += o.Cost * float64(q)
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
		line.UnitCost = Round2(p.Cost + modCostPerUnit)
		line.LineTotal = Round2((p.Price + modsPerUnit) * in.Qty)
		out.Lines = append(out.Lines, line)
		out.Subtotal += line.LineTotal
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
