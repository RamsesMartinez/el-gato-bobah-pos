package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

var (
	// ErrEntregaExcede: se intentó entregar más de lo que falta del renglón.
	ErrEntregaExcede = fmt.Errorf("%w: no puedes entregar más de lo que falta de ese producto", ErrValidation)
	// ErrEntregaInvalida: la cantidad a entregar no es un número positivo.
	ErrEntregaInvalida = fmt.Errorf("%w: la cantidad a entregar tiene que ser mayor que cero", ErrValidation)
	// ErrLineaCancelada: el renglón se canceló, así que no hay nada que entregar.
	ErrLineaCancelada = fmt.Errorf("%w: ese producto está cancelado", ErrConflict)
	// ErrCancelarConEntregas: el pedido ya soltó comida y cancelarlo repondría stock inexistente.
	// No sugiere un reembolso: lo que hace falta ahí es quitar lo pendiente o cerrar el pedido, y
	// un reembolso es salida de dinero que nadie pidió.
	ErrCancelarConEntregas = fmt.Errorf("%w: este pedido ya tiene productos entregados; quita lo que falta o ciérralo", ErrConflict)
)

// LineaEntrega es lo que el dominio necesita saber de un renglón para razonar sobre su entrega.
// No trae precio ni producto: entregar no mueve dinero.
type LineaEntrega struct {
	ID        int64
	Cantidad  decimal.Decimal
	Entregado decimal.Decimal
	Cancelada bool
}

// Falta devuelve cuánto queda por entregar de este renglón.
func (l LineaEntrega) Falta() decimal.Decimal {
	return l.Cantidad.Sub(l.Entregado)
}

// ValidarEntrega rechaza entregar una cantidad imposible de un renglón.
//
// El tope contra lo que FALTA (y no contra la cantidad pedida) es lo que hace idempotente al doble
// tap: quien marca "entregué 3" dos veces sobre un renglón de 5 recibe un error claro en vez de
// dejar el renglón en 6 de 5 y el pedido cerrado con comida todavía en la freidora.
func ValidarEntrega(l LineaEntrega, cantidad decimal.Decimal) error {
	if l.Cancelada {
		return fmt.Errorf("%w (línea %d)", ErrLineaCancelada, l.ID)
	}
	if !cantidad.IsPositive() {
		return fmt.Errorf("%w (línea %d)", ErrEntregaInvalida, l.ID)
	}
	if cantidad.GreaterThan(l.Falta()) {
		return fmt.Errorf("%w: faltan %s y se intentó entregar %s (línea %d)",
			ErrEntregaExcede, l.Falta(), cantidad, l.ID)
	}
	return nil
}

// TodoEntregado dice si ya se le dio al cliente todo lo que pidió y sigue vivo.
//
// Lo cancelado no cuenta —esa comida no se hizo—, pero un pedido cuyos renglones se cancelaron
// TODOS tampoco está entregado: nadie recibió nada, y cerrarlo como entregado convertiría una
// cancelación renglón a renglón en una venta que el corte daría por buena.
func TodoEntregado(lineas []LineaEntrega) bool {
	vivas := 0
	for _, l := range lineas {
		if l.Cancelada {
			continue
		}
		vivas++
		if l.Falta().IsPositive() {
			return false
		}
	}
	return vivas > 0
}

// HayEntregaParcial dice si algo de este pedido ya salió de la cocina.
//
// Es la guardia de la cancelación: cancelar repone el stock de todas las líneas, y reponer lo que
// el cliente ya se llevó le inventa al almacén comida que no existe. Cuenta también lo entregado
// de un renglón que después se canceló — la cancelación del renglón no devuelve lo que ya salió.
func HayEntregaParcial(lineas []LineaEntrega) bool {
	for _, l := range lineas {
		if l.Entregado.IsPositive() {
			return true
		}
	}
	return false
}

// CanDeliverAll rechaza entregar un pedido sin productos vivos.
//
// «Entregar todo» sobre un pedido al que se le quitó todo lo cerraba como entregado: una venta de
// $0 que el corte y Ventas contaban como venta. Ese pedido no se entregó; se cierra como
// cancelación («Cerrar pedido»).
func CanDeliverAll(lineas []LineaEntrega) error {
	for _, l := range lineas {
		if !l.Cancelada {
			return nil
		}
	}
	return ErrNoProducts
}

// ErrNothingPending: se pidió quitar lo que falta de un pedido al que ya no le falta nada.
var ErrNothingPending = fmt.Errorf("%w: Ya no falta nada por entregar", ErrConflict)

// PlanCancelPending decide qué hace «Quitar lo que falta»: quitar lo pendiente, o —si el pedido ya
// no tiene productos vivos— cancelarlo vacío.
//
// Un pedido vacío y sin pagos se cancela aunque quien lo pida no pueda «Cancelar pedido»: si no, se
// quedaría en el tablero sin una sola acción que lo cierre. Con pagos no: cancelarlo sacaría ese
// dinero del corte sin que nadie lo devolviera; primero se devuelven.
func PlanCancelPending(lineas []LineaEntrega, paid decimal.Decimal) (closeEmpty bool, err error) {
	vivas, pendientes := 0, 0
	for _, l := range lineas {
		if l.Cancelada {
			continue
		}
		vivas++
		if l.Falta().IsPositive() {
			pendientes++
		}
	}
	switch {
	case vivas == 0 && paid.IsPositive():
		return false, ErrOrderHasPayments
	case vivas == 0:
		return true, nil
	case pendientes == 0:
		return false, ErrNothingPending
	}
	return false, nil
}

// RemovalKeepsPayments rechaza quitar productos si el total quedaría por debajo de lo ya cobrado.
//
// Ese dinero se quedaría sin venta que lo explique y el corte lo contaría como ingreso de un pedido
// que ya no lo vale. Lo que procede es devolver un pago primero.
func RemovalKeepsPayments(newTotal, paid decimal.Decimal) error {
	if paid.GreaterThan(newTotal) {
		return ErrOrderWouldBeOverpaid
	}
	return nil
}

// ReasonToRemove limpia el motivo de quitar productos y lo exige.
func ReasonToRemove(reason string) (string, error) {
	if MotivoLimpio(reason) == "" {
		return "", fmt.Errorf("%w: Elige por qué se quitan", ErrValidation)
	}
	return MotivoValido(reason)
}
