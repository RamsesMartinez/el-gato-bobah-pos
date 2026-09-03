package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// Las reglas de devolver dinero, puras y sin I/O.
//
// El sistema no tenía ninguna: `Refund` marcaba la orden y anotaba como pérdida `orders.total` sin
// mirar un solo cobro. Un pedido entregado con $220 pendientes registraba $220 de pérdida por un
// ingreso que nunca ocurrió, y la cuenta por cobrar desaparecía del tablero sin haberse cobrado.
//
// Que vivan aquí es lo que permite probarlas sin base de datos, y es donde la pantalla las espeja en
// vez de reinventarlas.

var (
	// ErrDevolucionExcede: se pide devolver más de lo que entró, o más de lo que queda por devolver.
	// Es validación y no conflicto: el monto que llegó está mal, no el estado del pedido.
	ErrDevolucionExcede = fmt.Errorf("%w: no puedes devolver más de lo que se cobró de ese pedido", ErrValidation)
	// ErrSinCobrosQueDevolver: el pedido no tiene un solo cobro. Va aparte de ErrDevolucionExcede
	// porque la pantalla tiene que poder explicarlo: hoy el tablero ofrece "Reembolsar" al lado de
	// "Cobrar $220" en la misma tarjeta, y el operador no tiene cómo saber por qué rebota.
	ErrSinCobrosQueDevolver = fmt.Errorf("%w: este pedido no se ha cobrado, así que no hay nada que devolver", ErrConflict)
	// ErrCancelarSinDevolver: se intenta cancelar un pedido que ya tiene cobros sin confirmar que el
	// dinero se le regresa al cliente.
	//
	// Cancelarlo a secas lo sacaba de los reportes y dejaba los cobros en la base, con el arqueo
	// esperando ese dinero en el cajón: devolverlo dejaba el corte con un faltante que ningún renglón
	// explicaba, y no devolverlo dejaba al negocio con dinero que no aparecía en ninguna venta.
	ErrCancelarSinDevolver = fmt.Errorf("%w: este pedido ya tiene cobros; para cancelarlo hay que devolver ese dinero", ErrConflict)
)

// CobradoPorMetodo: cuánto entró por cada medio de pago en un pedido.
//
// `Activo` viaja pero NO decide: devolver por un método desactivado se permite. Cobrar con uno
// inactivo se rechaza porque no debe entrar dinero nuevo por ahí; el que ya entró tiene que poder
// salir por donde entró, o queda atrapado y el arqueo nunca cuadra.
type CobradoPorMetodo struct {
	MetodoID   int16
	Nombre     string
	EsEfectivo bool
	Activo     bool
	Monto      decimal.Decimal
}

// ParteDeDevolucion: cuánto se devuelve por un medio, y si eso sale del cajón.
type ParteDeDevolucion struct {
	MetodoID     int16
	Nombre       string
	Monto        decimal.Decimal
	SaleDelCajon bool
}

// MontoDevolvible: cuánto queda por devolver de lo que ya entró.
//
// Nunca negativo. Si por lo que sea se devolvió de más, lo que queda es cero — no una deuda del
// cliente hacia el negocio, que es lo que un número negativo diría.
func MontoDevolvible(cobrado, yaDevuelto decimal.Decimal) decimal.Decimal {
	queda := Round2(cobrado.Sub(yaDevuelto))
	if queda.IsNegative() {
		return decimal.Zero
	}
	return queda
}

// ValidarDevolucion decide si una devolución se puede registrar.
//
// El tope es lo COBRADO menos lo ya devuelto, nunca el total del pedido: un pedido de $500 cobrado a
// medias no puede devolver $500, y uno sin cobrar no puede devolver nada.
func ValidarDevolucion(monto, cobrado, yaDevuelto decimal.Decimal) error {
	if cobrado.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("%w: este pedido no tiene cobros que devolver", ErrSinCobrosQueDevolver)
	}
	if !ValidMoney(Round2(monto), false) {
		return fmt.Errorf("%w: el monto a devolver no es una cantidad de dinero", ErrValidation)
	}
	if Round2(monto).LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("%w: devolver cero no es devolver", ErrValidation)
	}
	queda := MontoDevolvible(cobrado, yaDevuelto)
	if Round2(monto).GreaterThan(queda) {
		return fmt.Errorf("%w: se pide devolver %s y solo quedan %s de lo cobrado",
			ErrDevolucionExcede, Round2(monto), queda)
	}
	return nil
}

// RepartirDevolucion decide de qué medio sale cada peso.
//
// EL DINERO SALE POR DONDE ENTRÓ, y en el orden en que entró. Devolver en efectivo lo que entró por
// tarjeta saca del cajón dinero que nunca estuvo ahí, y el arqueo cierra con un faltante inventado.
//
// Se acota a lo que entró aunque el llamador pida de más: `ValidarDevolucion` ya lo rechaza antes,
// pero esta función no puede confiar en que la llamen bien — devolver de más es inventar dinero.
//
// Solo el efectivo marca `SaleDelCajon`: es el único que de verdad se saca de la caja y hace un
// movimiento. Lo de tarjeta y plataformas se registra contra su método y se concilia con la
// terminal, porque ese dinero nunca pasó por el cajón.
func RepartirDevolucion(entradas []CobradoPorMetodo, monto decimal.Decimal) []ParteDeDevolucion {
	restante := Round2(monto)
	var partes []ParteDeDevolucion
	for _, e := range entradas {
		if restante.LessThanOrEqual(decimal.Zero) {
			break
		}
		disponible := Round2(e.Monto)
		if disponible.LessThanOrEqual(decimal.Zero) {
			continue
		}
		toma := disponible
		if restante.LessThan(toma) {
			toma = restante
		}
		partes = append(partes, ParteDeDevolucion{
			MetodoID: e.MetodoID, Nombre: e.Nombre, Monto: toma, SaleDelCajon: e.EsEfectivo,
		})
		restante = Round2(restante.Sub(toma))
	}
	return partes
}
