package domain

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// Las reglas de cancelar UN renglón, puras y sin I/O.
//
// Existían las columnas —`order_lines.cancelled_at`, `cancelled_by`, `cancel_reason`— y no existía
// la operación: ninguna consulta las escribía. Mientras tanto, el error de cancelar un pedido con
// entregas parciales mandaba al operador a "cancela los que falten", que no se podía hacer desde
// ningún lado. La única salida practicable era marcar como entregado lo que seguía en la plancha.

// ErrRenglonYaEntregado: no se cancela un renglón del que el cliente ya se llevó la comida. Lo que
// se hace con lo entregado es devolver el dinero, que es otra operación.
var ErrRenglonYaEntregado = fmt.Errorf(
	"%w: ese producto ya se entregó; lo que se devuelve es su dinero, no el renglón", ErrConflict)

// ReponeInventario dice si quitar un renglón devuelve su insumo al almacén.
//
// Lo decide el sistema y no el operador: el producto y el renglón ya saben la respuesta. Lo que no se
// prepara (`needs_prep` falso, el refresco embotellado) vuelve siempre: todo renglón nace «enviado a
// cocina», así que mirar solo la comanda hacía que nunca volviera, y cada refresco quitado era una
// merma inventada. Lo que se prepara vuelve solo si la comanda no salió; con fecha ya está en la
// plancha, y reponerlo inventariaría existencias que se consumieron.
//
// La consecuencia se ANUNCIA en pantalla antes de confirmar: quitar algo que ya se consumió baja el
// total del pedido pero no devuelve el insumo. Callarlo descuadra el almacén sin que nadie sepa por
// qué.
func ReponeInventario(needsPrep bool, enviadoACocina *time.Time) bool {
	return !needsPrep || enviadoACocina == nil
}

// PuedeCancelarRenglon decide si un renglón admite cancelarse.
//
// Dos barreras: el pedido tiene que seguir vivo —uno cancelado, reembolsado o entregado ya
// clasificó su dinero— y el renglón no puede tener nada entregado, ni siquiera a medias.
func PuedeCancelarRenglon(estadoPedido string, cantidad, entregado decimal.Decimal) error {
	if !PuedeRecibirLineas(estadoPedido) || estadoPedido == StatusEntregada {
		return fmt.Errorf("%w: un pedido %s ya no admite cambios en sus renglones", ErrConflict, estadoPedido)
	}
	if entregado.GreaterThan(decimal.Zero) {
		return ErrRenglonYaEntregado
	}
	if cantidad.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("%w: ese renglón no tiene cantidad que cancelar", ErrValidation)
	}
	return nil
}

// LinePieces son las piezas de un renglón: cuántas tiene, cuántas se entregaron y cuántas cubre
// algún pago vivo.
type LinePieces struct {
	Qty       decimal.Decimal
	Delivered decimal.Decimal
	Covered   decimal.Decimal
}

// LinePart es una de las dos mitades de un renglón partido.
type LinePart struct {
	Qty       decimal.Decimal
	Delivered decimal.Decimal
}

// LineSplit es cómo queda un renglón al sacarle piezas: lo que se queda y lo que se va.
type LineSplit struct {
	Keep LinePart
	Move LinePart
}

// MovablePieces son las piezas que pueden salir de un renglón sin tocar lo pagado: la cobertura es
// del renglón que se cobró, así que lo pagado se queda en él.
func MovablePieces(l LinePieces) decimal.Decimal {
	return decimal.Max(decimal.Zero, l.Qty.Sub(l.Covered))
}

// SplitLine decide qué se lleva el renglón nuevo al sacar k piezas de uno.
//
// Lo pendiente se va primero: es lo que se quita o se pasa a otra cuenta. Un renglón con piezas
// entregadas y pendientes a la vez no se parte por en medio de las entregadas —no hay forma de
// saber cuál de ellas se fue, y la cocina ya las dio por hechas—, así que se rechaza, salvo que se
// lleven todas juntas.
func SplitLine(l LinePieces, k decimal.Decimal) (LineSplit, error) {
	if !k.IsPositive() {
		return LineSplit{}, ErrEmptySelection
	}
	if k.GreaterThan(l.Qty) {
		return LineSplit{}, ErrTooManyPieces
	}
	if k.GreaterThan(MovablePieces(l)) {
		return LineSplit{}, ErrPieceAlreadyPaid
	}
	pending := l.Qty.Sub(l.Delivered)
	mixed := l.Delivered.IsPositive() && pending.IsPositive()
	if mixed && k.GreaterThan(pending) && k.LessThan(l.Qty) {
		return LineSplit{}, ErrMixedDeliveredPieces
	}
	movedDelivered := decimal.Max(decimal.Zero, k.Sub(pending))
	return LineSplit{
		Keep: LinePart{Qty: l.Qty.Sub(k), Delivered: l.Delivered.Sub(movedDelivered)},
		Move: LinePart{Qty: k, Delivered: movedDelivered},
	}, nil
}

// SplitMovement parte un movimiento de inventario q de un renglón de n piezas al sacarle k: lo que
// se queda con el original y lo que se lleva el nuevo.
//
// Lo que se queda se redondea a 4 decimales (la escala de la columna) y lo que se va es el resto,
// para que las dos mitades sumen q exacto: si cada una se redondeara por su lado, cada partición
// dejaría un residuo de existencias que nadie movió.
func SplitMovement(q, n, k decimal.Decimal) (keep, move decimal.Decimal) {
	keep = Round4(q.Mul(n.Sub(k)).Div(n))
	return keep, q.Sub(keep)
}
