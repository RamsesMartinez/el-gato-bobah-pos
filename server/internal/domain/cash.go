package domain

import "github.com/shopspring/decimal"

// ResolveDeclared decide el monto declarado a persistir para un método de pago al cerrar
// caja. Si el método está marcado auto-declare (configurable a nivel negocio), el declarado
// es siempre el esperado — el valor que mande el cliente se ignora, igual que el servidor
// recalcula precios en BuildOrder: así un front comprometido/con bug no puede subdeclarar un
// método que nunca requirió conteo físico.
func ResolveDeclared(autoDeclare bool, expected, clientDeclared decimal.Decimal) decimal.Decimal {
	if autoDeclare {
		return expected
	}
	return clientDeclared
}
