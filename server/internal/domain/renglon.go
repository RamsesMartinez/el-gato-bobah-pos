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

// ReponeInventario dice si cancelar un renglón devuelve su insumo al almacén.
//
// Lo decide el sistema y no el operador: `enviado_a_cocina_at` ya sabe la respuesta. NULL = la
// comanda no salió, la comida no se hizo, el insumo vuelve. Con fecha = está en la plancha, y
// reponerlo inventariaría existencias que se consumieron.
//
// La consecuencia se ANUNCIA en pantalla antes de confirmar: cancelar algo que ya salió a cocina
// baja el total del pedido pero no devuelve el insumo. Callarlo descuadra el almacén sin que nadie
// sepa por qué.
func ReponeInventario(enviadoACocina *time.Time) bool {
	return enviadoACocina == nil
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
