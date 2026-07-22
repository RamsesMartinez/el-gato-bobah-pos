package domain

import "github.com/shopspring/decimal"

// Tipos de movimiento de efectivo del cajón (columna kind con check en la BD).
const (
	CashEntrada = "entrada"
	CashSalida  = "salida"
)

// ValidCashKind rechaza en la frontera cualquier tipo que no sea entrada/salida (un check
// violado sería 500; así cae como 400/422 limpio).
func ValidCashKind(kind string) bool {
	return kind == CashEntrada || kind == CashSalida
}

// ValidTransfer valida las reglas PURAS de un traspaso entre cajas (lo que no toca BD): origen y
// destino existentes y distintos, y monto acotado > 0. El estado que sí depende de la BD (ambas
// cajas abiertas, misma moneda) lo verifica el servicio. Pásale el monto ya redondeado (Round2).
func ValidTransfer(fromRegisterID, toRegisterID int64, amount decimal.Decimal) bool {
	if fromRegisterID <= 0 || toRegisterID <= 0 || fromRegisterID == toRegisterID {
		return false
	}
	return ValidMoney(amount, false)
}

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
