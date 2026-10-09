package domain

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// NO HAY FIADOS (decisión del dueño, 2026-10-09).
//
// Un pedido de mostrador entregado y sin pagar completo bloquea el cierre de caja. Antes se listaba
// sin bloquear —«fiar es una decisión legítima del negocio»— y el dueño lo prohibió: lo que se
// entrega se cobra, o se cancela con su motivo («se fue sin pagar»). Los de plataforma no entran: los
// paga la plataforma, no el cliente en la caja.
var (
	// ErrUnpaidOrders: se quiso cerrar la caja con pedidos entregados que deben. Sentinel propio y no
	// envuelto en ErrConflict: el 409 lo decide httpapi.Error y el texto es el que lee quien opera.
	ErrUnpaidOrders = errors.New("hay pedidos entregados sin cobrar")
	// ErrCancelDeliveredWithPayments: cancelar un entregado que ya tiene cobros sacaría de Ventas un
	// dinero que sí está en el cajón. Se cobra lo que falta.
	ErrCancelDeliveredWithPayments = fmt.Errorf("%w: Este pedido ya tiene pagos; cancela lo que falta", ErrConflict)
	// ErrCancelDeliveredPaid: un entregado pagado es una venta terminada; se devuelve, no se cancela.
	ErrCancelDeliveredPaid = fmt.Errorf("%w: Este pedido ya está pagado", ErrConflict)
	// ErrWriteOffWithoutPayments: sin un solo pago no hay venta que conservar; se cancela entero.
	ErrWriteOffWithoutPayments = fmt.Errorf("%w: Este pedido no tiene pagos; cancélalo completo", ErrConflict)
	// ErrWriteOffNotDelivered: lo que falta se da por perdido solo cuando la comida ya salió.
	ErrWriteOffNotDelivered = fmt.Errorf("%w: Este pedido todavía no se entrega", ErrConflict)
	// ErrWriteOffPlatform: un pedido de plataforma lo paga la plataforma, no el cliente que se fue.
	ErrWriteOffPlatform = fmt.Errorf("%w: Los pedidos de plataforma los paga la plataforma", ErrConflict)
	// ErrWrittenOffNoCharge: lo que faltaba ya se dio por perdido; cobrarlo lo contaría dos veces.
	ErrWrittenOffNoCharge = fmt.Errorf("%w: Lo que faltaba de este pedido ya se dio por perdido", ErrConflict)
)

// «CANCELAR LO QUE FALTA» (dueño, 2026-10-09, opción A). Un entregado pagado a medias cuyo cliente se
// fue: lo pagado SE QUEDA como venta —el dinero está en el cajón— y lo que falta se da por PERDIDO
// con su motivo. Lo perdido es un concepto propio: no es cobro (no entra al Total de Ventas ni al
// esperado del corte), no es devolución (no salió dinero) y deja de ser «por cobrar». Cada peso en
// un solo lugar (constitución III).

// WriteOffRemainder dice cuánto se da por perdido: lo que falta por cobrar de un entregado con pagos.
func WriteOffRemainder(status string, total, paid decimal.Decimal) (decimal.Decimal, error) {
	if status != StatusEntregada {
		return decimal.Zero, ErrWriteOffNotDelivered
	}
	falta := PorCobrar(total, paid)
	if !falta.IsPositive() {
		return decimal.Zero, ErrCancelDeliveredPaid
	}
	if !paid.IsPositive() {
		return decimal.Zero, ErrWriteOffWithoutPayments
	}
	return falta, nil
}

// Owed es lo que un pedido todavía debe: lo que falta por cobrar menos lo que se dio por perdido. Lo
// perdido no se suma a lo pagado en ningún otro lado: solo deja de deberse.
func Owed(total, paid, writtenOff decimal.Decimal) decimal.Decimal {
	return PorCobrar(total.Sub(writtenOff), paid)
}

// OwingOrder es un pedido entregado que todavía debe.
type OwingOrder struct {
	ID     int64           `json:"id"`
	Number int             `json:"number"`
	Name   string          `json:"name"`
	Total  decimal.Decimal `json:"total"`
	Paid   decimal.Decimal `json:"paid"`
	// WrittenOff: lo que ya se dio por perdido. Un pedido con todo su resto perdido no debe nada.
	WrittenOff decimal.Decimal `json:"writtenOff"`
}

// Outstanding es lo que falta, con el MISMO predicado que da un pedido por saldado.
func (o OwingOrder) Outstanding() decimal.Decimal { return Owed(o.Total, o.Paid, o.WrittenOff) }

// NoOwingOrders falla nombrando cada pedido que debe y cuánto.
func NoOwingOrders(orders []OwingOrder) error {
	partes := []string{}
	for _, o := range orders {
		falta := o.Outstanding()
		if !falta.IsPositive() {
			continue
		}
		nombre := "#" + strconv.Itoa(o.Number)
		if o.Name != "" {
			nombre = o.Name + " (" + nombre + ")"
		}
		partes = append(partes, nombre+" debe $"+falta.StringFixed(2))
	}
	if len(partes) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s. Cóbralos o cancélalos antes de cerrar", ErrUnpaidOrders, strings.Join(partes, ", "))
}

// CanCancelDelivered decide si un pedido entregado se puede cancelar: solo si debe todo.
func CanCancelDelivered(total, paid decimal.Decimal) error {
	if paid.IsPositive() {
		if !PorCobrar(total, paid).IsPositive() {
			return ErrCancelDeliveredPaid
		}
		return ErrCancelDeliveredWithPayments
	}
	if !total.IsPositive() {
		return ErrCancelDeliveredPaid
	}
	return nil
}
