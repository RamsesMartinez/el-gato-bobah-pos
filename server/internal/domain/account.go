package domain

import (
	"time"

	"github.com/shopspring/decimal"
)

// El estado de una cuenta como lo ve quien atiende (spec 030, R-6). Sale de aquí y viaja como dato:
// el front no reimplementa la regla, porque con dos copias la ficha y el ticket terminan diciendo
// cosas distintas de la misma cuenta.
const (
	AccountCapturing     = "capturing"
	AccountInKitchen     = "in_kitchen"
	AccountPaidInKitchen = "paid_in_kitchen"
	AccountPartlyPaid    = "partly_paid"
	AccountDeliveredOwes = "delivered_owes"
	// AccountClosedWithNew es un pedido ya cerrado (entregado y saldado, o cancelado) que conserva
	// una «Nuevo» capturándose. No es «en cocina»: lo vivo es la captura.
	AccountClosedWithNew = "closed_with_new"
)

// Los grupos de la hoja «+N».
const (
	GroupCapturing     = "capturing"
	GroupInKitchen     = "in_kitchen"
	GroupDeliveredOwes = "delivered_owes"
	GroupPreviousDays  = "previous_days"
)

// AccountState da el estado de una cuenta viva, o `false` si no se lista.
//
// Cerrada = saldada Y entregada (D-9), con el MISMO predicado que cierra el pedido (PedidoSaldado):
// un centavo de redondeo no deja una cuenta «debe $0.01» que nadie puede cobrar, y un pedido de $0
// entregado está cerrado, no debiendo. Cancelada y reembolsada no se listan: su dinero ya se decidió.
func AccountState(isDraft bool, status string, paid, total decimal.Decimal) (string, bool) {
	if isDraft {
		return AccountCapturing, true
	}
	saldado := PedidoSaldado(paid, total)
	switch status {
	case StatusAbierta, StatusLista:
		switch {
		case saldado:
			return AccountPaidInKitchen, true
		case paid.IsPositive():
			return AccountPartlyPaid, true
		default:
			return AccountInKitchen, true
		}
	case StatusEntregada:
		if saldado {
			return "", false
		}
		return AccountDeliveredOwes, true
	}
	return "", false
}

// OrderAccountState da el estado de la ficha de un pedido. Uno que ya no se listaría solo sigue en
// la fila si tiene una «Nuevo» viva, para que lo capturado no se pierda de vista.
func OrderAccountState(status string, paid, total decimal.Decimal, hasNew bool) (string, bool) {
	if state, listed := AccountState(false, status, paid, total); listed {
		return state, true
	}
	if hasNew {
		return AccountClosedWithNew, true
	}
	return "", false
}

// AccountGroup da el grupo de la hoja «+N». Solo la entregada que debe se separa por día: lo que
// sigue en cocina es trabajo de hoy aunque se haya pedido ayer, y la deuda de otro día es la que se
// perdía de vista (caso 3 del lienzo). Compara días de calendario, no instantes.
func AccountGroup(state string, businessDate, today time.Time) string {
	switch state {
	case AccountCapturing, AccountClosedWithNew:
		return GroupCapturing
	case AccountDeliveredOwes:
		if dayOf(businessDate).Before(dayOf(today)) {
			return GroupPreviousDays
		}
		return GroupDeliveredOwes
	default:
		return GroupInKitchen
	}
}

func dayOf(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
