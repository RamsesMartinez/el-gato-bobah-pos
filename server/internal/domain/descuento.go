package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// El divisor del porcentaje es `cien`, que ya vive en platform_price.go: el mismo 100 con el que
// se calcula el sobreprecio de plataforma.

// ResolverDescuento convierte lo que capturó el operador en PESOS, contra el subtotal del servidor.
//
// El porcentaje es una forma de teclear, no un dato que se conserve: lo que el negocio dejó de
// cobrar es una cantidad de dinero ya ocurrida. Guardar la fórmula la dejaría viva, y el descuento
// de un pedido crecería solo cuando alguien le agregue un café — con el ticket ya impreso en la
// mano del cliente.
//
// El subtotal contra el que se resuelve es SIEMPRE el que calculó el servidor. Creerle al cliente
// una cifra de dinero es lo mismo que BuildOrder no hace con los precios.
//
// Los dos parámetros juntos son ambigüedad, no una precedencia a decidir: se rechaza. Ausentes los
// dos, no hay descuento — que es distinto de un descuento de cero capturado a propósito, aunque
// ambos guarden 0: la diferencia vive en el rastro, que solo se estampa cuando hay monto.
func ResolverDescuento(subtotal decimal.Decimal, monto, porcentaje *decimal.Decimal) (decimal.Decimal, error) {
	switch {
	case monto != nil && porcentaje != nil:
		return decimal.Zero, fmt.Errorf("%w: el descuento viene como monto y como porcentaje a la vez", ErrValidation)
	case monto == nil && porcentaje == nil:
		return decimal.Zero, nil
	}

	var pesos decimal.Decimal
	if porcentaje != nil {
		p := *porcentaje
		// escalaSana antes de multiplicar: un exponente absurdo convierte la multiplicación en
		// minutos de CPU, igual que en Round2.
		if !escalaSana(p) || p.IsNegative() || p.GreaterThan(cien) {
			return decimal.Zero, fmt.Errorf("%w: el porcentaje de descuento va de 0 a 100", ErrValidation)
		}
		pesos = Round2(subtotal.Mul(p).Div(cien))
	} else {
		pesos = Round2(*monto)
	}

	// allowZero: un descuento de cero es válido y significa "sin descuento".
	if !ValidMoney(pesos, true) {
		return decimal.Zero, fmt.Errorf("%w: el descuento no es un monto válido", ErrValidation)
	}
	if pesos.GreaterThan(subtotal) {
		// El máximo va EN EL MENSAJE: sin él, el operador vuelve a teclear a ciegas con el cliente
		// enfrente. Y es 422 y no 400 porque el dato está bien formado; lo que no se puede es
		// descontar más de lo que se vendió.
		return decimal.Zero, fmt.Errorf("%w (máximo %s)", ErrDescuentoMayorQueLaVenta, subtotal.StringFixed(2))
	}
	return pesos, nil
}

// AplicarDescuento resta el descuento del total, ANTES de que se le sume el envío.
//
// El piso en cero no recorta el descuento registrado, solo el total: el descuento es lo que una
// persona decidió y quedó guardado; el total es lo que se cobra, y cobrar en negativo devolvería
// dinero que nadie autorizó. Los dos se separan porque cancelar un renglón después puede dejar el
// subtotal por debajo del descuento sin que nadie haya cambiado el descuento.
func AplicarDescuento(o BuiltOrder, descuento decimal.Decimal) (BuiltOrder, error) {
	descuento = Round2(descuento)
	if !ValidMoney(descuento, true) {
		return BuiltOrder{}, ErrValidation
	}
	o.Discount = descuento
	total := Round2(o.Subtotal.Sub(descuento))
	if total.IsNegative() {
		total = decimal.Zero
	}
	o.Total = total
	if !ValidMoney(o.Total, true) {
		return BuiltOrder{}, ErrValidation
	}
	return o, nil
}
