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
	ErrCancelDeliveredWithPayments = fmt.Errorf("%w: Este pedido ya tiene pagos; cobra lo que falta", ErrConflict)
	// ErrCancelDeliveredPaid: un entregado pagado es una venta terminada; se devuelve, no se cancela.
	ErrCancelDeliveredPaid = fmt.Errorf("%w: Este pedido ya está pagado", ErrConflict)
)

// OwingOrder es un pedido entregado que todavía debe.
type OwingOrder struct {
	ID     int64           `json:"id"`
	Number int             `json:"number"`
	Name   string          `json:"name"`
	Total  decimal.Decimal `json:"total"`
	Paid   decimal.Decimal `json:"paid"`
}

// Outstanding es lo que falta, con el MISMO predicado que da un pedido por saldado.
func (o OwingOrder) Outstanding() decimal.Decimal { return PorCobrar(o.Total, o.Paid) }

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
