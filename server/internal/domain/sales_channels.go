package domain

import "github.com/shopspring/decimal"

// ChannelPrice es el precio de un producto en un lugar de venta (spec 092): el propio si se
// capturó, si no el de mostrador. Vacío nunca significa cero.
func ChannelPrice(base decimal.Decimal, own *decimal.Decimal) decimal.Decimal {
	if own != nil {
		return *own
	}
	return base
}

// ValidChannelPrice acepta el vacío (hereda) y un precio mayor que cero con dos decimales. El cero
// se rechaza: regalar un producto en la web no se captura por accidente.
func ValidChannelPrice(p *decimal.Decimal) error {
	if p == nil {
		return nil
	}
	if !ValidMoney(*p, false) || !p.Equal(Round2(*p)) {
		return ErrValidation
	}
	return nil
}

// MeasuresStock dice si el producto descuenta algo del almacén al venderse por sí mismo, que es lo
// que hace falta para que «vender sin existencia» signifique algo.
func MeasuresStock(p ProductModeFacts) bool {
	switch ModeOf(p) {
	case ModeOwnStock, ModeRecipe, ModePackage:
		return true
	}
	return false
}
