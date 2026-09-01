package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

var (
	// ErrPedidoYaPagado: no queda nada por cobrar de ese pedido.
	ErrPedidoYaPagado = fmt.Errorf("%w: ese pedido ya está cobrado", ErrConflict)
	// ErrCobroExcede: se intentó cobrar más de lo que el pedido debe.
	ErrCobroExcede = fmt.Errorf("%w: no puedes cobrar más de lo que falta de ese pedido", ErrValidation)
	// ErrPedidoNoCobrable: el pedido se canceló o se reembolsó; su dinero ya se decidió.
	ErrPedidoNoCobrable = fmt.Errorf("%w: ese pedido ya no se puede cobrar", ErrConflict)
	// ErrCobroFueraDeLugar: se intentó crear un pedido ya cobrado.
	//
	// Crear y cobrar de un golpe se saltaba la cocina por completo, y era el camino corto: el que
	// se usaba. El mensaje NOMBRA el camino correcto porque un error que solo dice "no" manda al
	// operador —o a quien integre contra la API— a adivinar cuál es.
	ErrCobroFueraDeLugar = fmt.Errorf("%w: el pedido se confirma primero y se cobra después, con /pay", ErrValidation)
)

// PorCobrar es lo que falta de un pedido, nunca negativo.
//
// No negativo porque un pedido puede quedar sobrepagado desde el cobro (el cliente dejó de más y
// se registró así), y arrastrar ese negativo a la suma del tablero lo convertiría en un descuento
// sobre lo que deben los demás pedidos.
func PorCobrar(total, pagado decimal.Decimal) decimal.Decimal {
	falta := Round2(total.Sub(pagado))
	if falta.IsNegative() {
		return decimal.Zero
	}
	return falta
}

// ValidarCobro rechaza un cobro imposible sobre un pedido ya creado.
//
// El tope contra lo que FALTA es lo que impide inflar la venta: sin él, un doble tap sobre "Cobrar
// $275" registraría $550 de ingreso por comida que se vendió una vez, y el corte cuadraría contra
// una cifra inventada. Aceptar de más solo tendría sentido para propina, y la propina viaja aparte
// precisamente porque no es ingreso del negocio.
func ValidarCobro(estado string, total, pagado, monto decimal.Decimal) error {
	if estado == StatusCancelada || estado == StatusReembolsada {
		return fmt.Errorf("%w (%s)", ErrPedidoNoCobrable, estado)
	}
	falta := PorCobrar(total, pagado)
	if falta.IsZero() {
		return ErrPedidoYaPagado
	}
	if !monto.IsPositive() {
		return fmt.Errorf("%w: el monto a cobrar tiene que ser mayor que cero", ErrValidation)
	}
	if monto.GreaterThan(falta) {
		return fmt.Errorf("%w: faltan %s y se intentó cobrar %s", ErrCobroExcede, falta, monto)
	}
	return nil
}
