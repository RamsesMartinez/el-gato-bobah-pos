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

// ProductUnavailable dice CUÁL producto tumbó el cobro. Nace de que el mensaje anterior era
// "producto no disponible (id 510)": el operador tiene el carrito enfrente y un número no le dice
// qué renglón quitar.
//
// Name va vacío cuando el catálogo de la empresa no conoce ese id. Ahí el nombre NO se puede
// averiguar: el producto o ya no existe, o es de otra empresa y la RLS lo esconde a propósito —
// sacarlo convertiría este error en un oráculo para leer el menú ajeno probando ids. Por eso el
// mensaje de ese caso no dice "no disponible" (que suena a que se agotó) sino que el producto no
// está en este menú, que es lo que de verdad pasa y sí es accionable.
type ProductUnavailable struct {
	ProductID int64
	Name      string
}

func (e ProductUnavailable) Error() string {
	if e.Name != "" {
		return fmt.Sprintf("%s: %s", ErrProductNotSell.Error(), e.Name)
	}
	return fmt.Sprintf("el producto ya no está en este menú (id %d)", e.ProductID)
}

// Unwrap mantiene el sentinel: httpapi.Error sigue mapeando esto a 422 por errors.Is.
func (e ProductUnavailable) Unwrap() error { return ErrProductNotSell }

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
	// MaxPerLine: cuántas veces admite esta opción en la MISMA línea. Lo configura el negocio por
	// opción (las salsas están en 2 para poder pedir dos del mismo sabor; las que no tiene sentido
	// repetir, en 1). Un 0 significa "sin configurar" y se trata como 1.
	MaxPerLine int
}

// --- Resultado priceado (snapshots que van a order_lines) ---

type BuiltOrder struct {
	Subtotal    decimal.Decimal
	DeliveryFee decimal.Decimal
	Total       decimal.Decimal
	Lines       []BuiltLine
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
			return BuiltOrder{}, ProductUnavailable{ProductID: in.ProductID}
		}
		if !p.Active {
			return BuiltOrder{}, ProductUnavailable{ProductID: p.ID, Name: p.Name}
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
			// El tope que puso el negocio por opción. Se valida aquí y no solo en la pantalla
			// porque la pantalla es espejo, no barrera: un cliente que mande 40 salsas en una
			// línea manda a cocina un ticket que el negocio nunca aceptó, y lo hace en silencio.
			tope := o.MaxPerLine
			if tope <= 0 {
				tope = 1 // 0 es "sin configurar" (el default de la columna es 1), no "ninguna"
			}
			if q > tope {
				return BuiltOrder{}, fmt.Errorf("%w: %s admite %d por línea y se pidieron %d", ErrOptionOverMax, o.Name, tope, q)
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

// ApplyDeliveryFee suma el costo de envío al total de un pedido a domicilio. El fee lo captura
// el operador en el cobro (como la propina): se valida en la frontera y NUNCA se asume para un
// pedido que no es a domicilio (ahí queda en 0, aunque el cliente mande otra cosa). allowZero:
// el envío gratis es válido. Devuelve la orden con DeliveryFee y Total ya actualizados.
func ApplyDeliveryFee(o BuiltOrder, fee decimal.Decimal, isDelivery bool) (BuiltOrder, error) {
	if !isDelivery {
		o.DeliveryFee = decimal.Zero
		return o, nil // Total ya = Subtotal, validado en BuildOrder
	}
	fee = Round2(fee)
	if !ValidMoney(fee, true) {
		return BuiltOrder{}, ErrValidation
	}
	o.DeliveryFee = fee
	o.Total = Round2(o.Subtotal.Add(fee))
	if !ValidMoney(o.Total, true) {
		return BuiltOrder{}, ErrValidation
	}
	return o, nil
}

// MetodoCorrespondeALaPlataforma dice si un método de pago puede cobrar un pedido: los dos tienen
// que apuntar a la misma plataforma, o ninguno a ninguna.
//
// Las dos direcciones importan y por motivos distintos:
//
//   - Un pedido de plataforma cobrado con el efectivo de mostrador hace que el sistema espere en el
//     cajón billetes que la plataforma pagó por transferencia: el turno cierra con FALTANTE.
//   - Un pedido de mostrador cobrado con un método de plataforma saca de la cuenta del cajón dinero
//     que sí estaba ahí: cierra con SOBRANTE.
//
// En los dos casos el operador ve un descuadre por el monto exacto y nada que lo explique. La regla
// vive aquí y no en el handler porque es aritmética de dinero, no forma del request.
func MetodoCorrespondeALaPlataforma(delMetodo, delPedido *int16) bool {
	if delMetodo == nil || delPedido == nil {
		return delMetodo == nil && delPedido == nil
	}
	return *delMetodo == *delPedido
}

// PagosCubren dice si lo pagado salda el total. Tolera un centavo de diferencia por el mismo motivo
// que la pantalla de cobro: el redondeo a dos decimales de varias líneas de pago puede dejar un
// centavo de sobra o de falta, y rechazar una venta saldada por eso deja al cliente esperando.
func PagosCubren(pagado, total decimal.Decimal) bool {
	return pagado.Sub(total).GreaterThanOrEqual(decimal.RequireFromString("-0.01"))
}
