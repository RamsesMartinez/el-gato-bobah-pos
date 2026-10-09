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

// MethodRefunds: lo que un turno devolvió por un medio, partido en lo que salió del cajón y lo que
// no, cada uno con su propina. Viene así del libro de devoluciones; las reglas de abajo deciden cómo
// se presenta.
type MethodRefunds struct {
	OffDrawer, OffDrawerTips decimal.Decimal
	Drawer, DrawerTips       decimal.Decimal
}

// Sale es la venta devuelta, sin propina, salga o no del cajón (spec 029). Es lo que el corte llama
// «Devoluciones» y la misma cifra que Ventas resta de cada medio: una devolución en efectivo y una
// con tarjeta son el mismo hecho, y presentarlas en dos lugares distintos del corte —una como
// salida de caja, la otra dentro de ingresos— hacía que dos pantallas dijeran cifras distintas.
func (r MethodRefunds) Sale() decimal.Decimal {
	return Round2(r.OffDrawer.Sub(r.OffDrawerTips).Add(r.Drawer.Sub(r.DrawerTips)))
}

// Tips es la propina devuelta. Se resta de la propina del turno y no de la venta: la propina nunca
// fue ingreso del negocio, y el reporte de propinas tampoco la cuenta.
func (r MethodRefunds) Tips() decimal.Decimal {
	return Round2(r.OffDrawerTips.Add(r.DrawerTips))
}

// NegativeMethodNote explica un medio que en el turno salió en negativo. Solo pasa cuando el turno
// devolvió dinero de ventas que se cobraron en otro turno; sin decirlo, el cajero busca un faltante
// que no existe.
func NegativeMethodNote(total decimal.Decimal) string {
	if !total.IsNegative() {
		return ""
	}
	return "Negativo porque se devolvió dinero de ventas cobradas en otro turno."
}
