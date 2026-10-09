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
	// ErrRefundOnRefundedOrder: el pedido ya se reembolsó por el flujo anterior al libro de
	// devoluciones. Ese flujo no escribía el libro, así que para él el pedido no tenía nada
	// devuelto y se podía devolver completo otra vez (spec 031, D18).
	ErrRefundOnRefundedOrder = fmt.Errorf("%w: ese pedido ya devolvió su dinero", ErrConflict)
	// ErrCashRefundNeedsOpenRegister: devolver dinero del cajón sin turno principal abierto. La
	// salida no quedaría en ningún arqueo: la apertura siguiente cuenta su fondo ya sin esos
	// billetes y nadie sabría por qué (spec 031, D7).
	ErrCashRefundNeedsOpenRegister = fmt.Errorf("%w: abre la caja para devolver efectivo", ErrConflict)
)

// CobradoPorMetodo: cuánto entró por cada medio de pago en un pedido.
//
// `Activo` viaja pero NO decide: devolver por un método desactivado se permite. Cobrar con uno
// inactivo se rechaza porque no debe entrar dinero nuevo por ahí; el que ya entró tiene que poder
// salir por donde entró, o queda atrapado y el arqueo nunca cuadra.
type CobradoPorMetodo struct {
	MetodoID int16
	Nombre   string
	// TocaElCajon: si el dinero de ese método está en el cajón, que es lo que decide si devolverlo
	// registra una salida de caja. Es `affects_cash_drawer` y NO "es de tipo efectivo": «Didi
	// efectivo» es de tipo plataforma y sus billetes están en el mismo montón que los del mostrador
	// cuando el reparto lo hace gente del local (spec 015, FR-018).
	TocaElCajon bool
	Activo      bool
	Monto       decimal.Decimal
	// Refunded: lo que ya salió por este medio. El reparto trabaja sobre lo que QUEDA de cada medio;
	// con lo cobrado en bruto, una segunda devolución volvía a sacar del primer medio lo que ya
	// había salido por él (D1).
	Refunded decimal.Decimal
	// Tip y TipRefunded: lo mismo para la propina, que solo regresa al cancelar.
	Tip         decimal.Decimal
	TipRefunded decimal.Decimal
}

// ParteDeDevolucion: cuánto se devuelve por un medio, y si eso sale del cajón.
type ParteDeDevolucion struct {
	MetodoID int16
	Nombre   string
	Monto    decimal.Decimal
	// Tip: propina que regresa por este medio. Va aparte del monto porque no es ingreso del
	// negocio: no entra a lo devuelto de la venta.
	Tip          decimal.Decimal
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
// Marca `SaleDelCajon` lo que ESTÁ en el cajón, que es lo único que de verdad se saca de la caja y
// hace un movimiento. Lo demás —tarjeta, transferencia, el efectivo de una app que se lleva el
// repartidor de la plataforma— se registra contra su método y se concilia aparte, porque ese dinero
// nunca pasó por el cajón.
//
// La regla decía "solo el efectivo" y con eso bastaba mientras el único efectivo fuera el del
// mostrador. Dejó de bastar: la condición es el interruptor «va al cajón» del método, no su tipo.
func RepartirDevolucion(entradas []CobradoPorMetodo, monto decimal.Decimal) []ParteDeDevolucion {
	restante := Round2(monto)
	var partes []ParteDeDevolucion
	for _, e := range entradas {
		if restante.LessThanOrEqual(decimal.Zero) {
			break
		}
		disponible := Round2(e.Monto.Sub(e.Refunded))
		if disponible.LessThanOrEqual(decimal.Zero) {
			continue
		}
		toma := disponible
		if restante.LessThan(toma) {
			toma = restante
		}
		partes = append(partes, ParteDeDevolucion{
			MetodoID: e.MetodoID, Nombre: e.Nombre, Monto: toma, SaleDelCajon: e.TocaElCajon,
		})
		restante = Round2(restante.Sub(toma))
	}
	return partes
}

// LineRefundable: cuánto se puede devolver contra UN renglón.
//
// El menor de dos topes: lo que queda del pedido y lo que vale el renglón menos lo ya devuelto
// contra él. Con solo el del pedido, un platillo de $60 en un pedido de $500 devolvía $500, y otra
// vez por cada platillo (D4). El importe es el del renglón sin prorratear el descuento: el tope del
// pedido sigue mandando.
func LineRefundable(cobrado, devueltoTotal, importe, devueltoRenglon decimal.Decimal) decimal.Decimal {
	delPedido := MontoDevolvible(cobrado, devueltoTotal)
	delRenglon := MontoDevolvible(importe, devueltoRenglon)
	if delRenglon.LessThan(delPedido) {
		return delRenglon
	}
	return delPedido
}

// ErrPaymentHasRefunds: devolver ese pago dejaría una devolución sin un cobro detrás.
var ErrPaymentHasRefunds = fmt.Errorf("%w: ese pago ya tiene una devolución; no se puede devolver otra vez", ErrConflict)

// VoidKeepsRefunds decide si se puede devolver un pago sin regresar dos veces el mismo dinero.
//
// `quedaDelMedio` es lo cobrado por ese medio SIN el pago que se devuelve; `devueltoDelMedio`, lo
// que ya salió por él. Si lo segundo supera a lo primero, el cliente recibiría el pago completo
// además de lo ya devuelto, y el pedido volvería a deber (D2). Por medio y no "cualquier
// devolución": una devolución por tarjeta no impide devolver el pago en efectivo.
func VoidKeepsRefunds(quedaDelMedio, devueltoDelMedio decimal.Decimal) error {
	if Round2(devueltoDelMedio).GreaterThan(Round2(quedaDelMedio)) {
		return ErrPaymentHasRefunds
	}
	return nil
}

// SplitCancellationRefund: lo que regresa cada medio al cancelar con devolución, cuenta y propina.
//
// Cancelar dice que la venta no ocurrió, así que el cliente recibe lo que dio, propina incluida.
// Devolver solo la cuenta dejaba la propina en el esperado del cajón y fuera de todo reparto,
// porque el pedido cancelado ya no cuenta para propinas (D9). Un medio cuya cuenta ya se devolvió
// puede regresar solo su propina.
func SplitCancellationRefund(entradas []CobradoPorMetodo) []ParteDeDevolucion {
	var partes []ParteDeDevolucion
	for _, e := range entradas {
		monto := MontoDevolvible(e.Monto, e.Refunded)
		propina := MontoDevolvible(e.Tip, e.TipRefunded)
		if monto.IsZero() && propina.IsZero() {
			continue
		}
		partes = append(partes, ParteDeDevolucion{
			MetodoID: e.MetodoID, Nombre: e.Nombre, Monto: monto, Tip: propina, SaleDelCajon: e.TocaElCajon,
		})
	}
	return partes
}
