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
	// ErrMetodoInactivo: el método de pago existe pero el negocio lo apagó.
	//
	// El negocio desactiva un método justamente para dejar de recibir por ahí. Aceptarlo igual manda
	// ese dinero a un renglón del corte que nadie está contando, y la única barrera era que el front
	// no lo listara — con una tableta encendida llevando horas con el catálogo en caché.
	ErrMetodoInactivo = fmt.Errorf("%w: ese método de pago ya no está activo", ErrValidation)
	// ErrPropinaExcede: la propina capturada es mayor que la cuenta entera.
	ErrPropinaExcede = fmt.Errorf("%w: la propina no puede ser mayor que la cuenta", ErrValidation)
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
	// El MISMO predicado que cierra el pedido decide que no queda nada por cobrar.
	//
	// PagosCubren tolera un centavo —el residuo de dividir $100 en tres partes de $33.33— y con él
	// se marca el pedido entregado. Restar a pelo dejaba a la barra del POS viéndole $0.01 a un
	// pedido que el sistema ya dio por saldado: dos predicados sobre la misma cifra, que es lo que
	// el corolario del principio III prohíbe. El operador no tenía cómo cobrar ese centavo, y al
	// día siguiente el pedido salía de la vista con la deuda abierta.
	if PagosCubren(pagado, total) {
		return decimal.Zero
	}
	return Round2(total.Sub(pagado))
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

// ValidarPropina topa la propina contra el total del pedido.
//
// Sin tope, la propina solo pasaba por ValidMoney —hasta diez millones— porque ValidarCobro acota
// únicamente el monto: la propina no cuenta para saldar y por eso no la veía nadie. Medido: un
// pedido de $250 aceptó $9,999 de propina. Esa cifra entra al esperado del cajón
// (ExpectedByMethodForSession suma tip_amount) y al reparto por empleado, así que un dedo gordo
// cierra el turno con un faltante por el monto exacto y sin un renglón que lo explique.
//
// El tope es la cuenta entera y no un porcentaje: una propina mayor que todo lo consumido es un
// error de captura muchísimo más seguido que un regalo, y el regalo de verdad se registra en dos
// cobros. Un porcentaje sería una política inventada, y además configurable — dos cosas que este
// repo no agrega sin un caso real.
func ValidarPropina(total, propina decimal.Decimal) error {
	if propina.IsNegative() {
		return fmt.Errorf("%w: la propina no puede ser negativa", ErrValidation)
	}
	if propina.GreaterThan(total) {
		return fmt.Errorf("%w (cuenta %s, propina %s)", ErrPropinaExcede, total, propina)
	}
	return nil
}
