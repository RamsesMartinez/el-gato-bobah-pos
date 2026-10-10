package domain

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// El estado que ve quien atiende sale de aquí y el front no lo recalcula: con dos implementaciones
// la ficha diría «Pagada» mientras el ticket dice que falta un centavo.
func TestAccountState(t *testing.T) {
	d := decimal.RequireFromString
	casos := []struct {
		nombre  string
		draft   bool
		status  string
		paid    string
		total   string
		state   string
		listada bool
	}{
		{"cuenta en captura", true, "", "0", "0", AccountCapturing, true},
		{"abierta sin pagos", false, StatusAbierta, "0", "100", AccountInKitchen, true},
		{"abierta saldada: pagada en cocina", false, StatusAbierta, "100", "100", AccountPaidInKitchen, true},
		{"lista con pago parcial", false, StatusLista, "40", "100", AccountPartlyPaid, true},
		{"entregada con deuda", false, StatusEntregada, "40", "100", AccountDeliveredOwes, true},
		{"entregada sin un peso pagado", false, StatusEntregada, "0", "100", AccountDeliveredOwes, true},
		{"entregada saldada: cerrada", false, StatusEntregada, "100", "100", "", false},
		// Todo regalado: no hay nada que cobrar, así que está cerrada, no «debe $0».
		{"entregada de $0", false, StatusEntregada, "0", "0", "", false},
		// Desde la 031 un centavo de diferencia es deuda: no hay tolerancia en PedidoSaldado.
		{"entregada con $0.01 de diferencia: debe (031 quitó la tolerancia)", false, StatusEntregada, "99.99", "100", AccountDeliveredOwes, true},
		{"entregada con $0.02 de diferencia: debe", false, StatusEntregada, "99.98", "100", AccountDeliveredOwes, true},
		{"cancelada", false, StatusCancelada, "0", "100", "", false},
		{"reembolsada", false, StatusReembolsada, "100", "100", "", false},
		{"estado desconocido no se lista", false, "otro", "0", "100", "", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			state, listada := AccountState(c.draft, c.status, d(c.paid), d(c.total))
			if listada != c.listada || state != c.state {
				t.Fatalf("AccountState = (%q, %v), quería (%q, %v)", state, listada, c.state, c.listada)
			}
		})
	}
}

func TestAccountGroup(t *testing.T) {
	hoy := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	ayer := hoy.AddDate(0, 0, -1)
	casos := []struct {
		nombre string
		state  string
		fecha  time.Time
		group  string
	}{
		{"capturando", AccountCapturing, time.Time{}, GroupCapturing},
		{"entregada que debe de hoy", AccountDeliveredOwes, hoy, GroupDeliveredOwes},
		{"entregada que debe de ayer", AccountDeliveredOwes, ayer, GroupPreviousDays},
		// Lo que sigue en cocina es trabajo de hoy aunque se haya pedido ayer: está en la plancha.
		{"abierta de ayer sigue en cocina", AccountInKitchen, ayer, GroupInKitchen},
		{"pagada en cocina", AccountPaidInKitchen, hoy, GroupInKitchen},
		{"pago parcial en cocina de ayer", AccountPartlyPaid, ayer, GroupInKitchen},
		// Lo vivo de una cerrada con «Nuevo» es la captura, no la cocina.
		{"cerrada con algo nuevo capturándose", AccountClosedWithNew, ayer, GroupCapturing},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := AccountGroup(c.state, c.fecha, hoy); got != c.group {
				t.Fatalf("AccountGroup = %q, quería %q", got, c.group)
			}
		})
	}
}

// Pedidos 614 y 623 del ambiente de pruebas: entregados y pagados, con una «Nuevo» viva, salían
// como «Pagada · en cocina» cuando ya se habían entregado.
func TestOrderAccountStateClosedWithNew(t *testing.T) {
	d := decimal.RequireFromString
	casos := []struct {
		nombre   string
		status   string
		paid     string
		total    string
		conNuevo bool
		state    string
		listada  bool
	}{
		{"entregada y pagada con Nuevo: cerrada, no en cocina", StatusEntregada, "100", "100", true, AccountClosedWithNew, true},
		{"cancelada con Nuevo: cerrada, no pagada", StatusCancelada, "0", "100", true, AccountClosedWithNew, true},
		{"entregada y pagada sin Nuevo no se lista", StatusEntregada, "100", "100", false, "", false},
		// Viva con Nuevo (p. ej. deuda vieja fuera de la ventana): manda su propio estado.
		{"entregada que debe con Nuevo sigue debiendo", StatusEntregada, "40", "100", true, AccountDeliveredOwes, true},
		{"abierta con Nuevo sigue en cocina", StatusAbierta, "0", "100", true, AccountInKitchen, true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			state, listada := OrderAccountState(c.status, d(c.paid), d(c.total), c.conNuevo)
			if listada != c.listada || state != c.state {
				t.Fatalf("OrderAccountState = (%q, %v), quería (%q, %v)", state, listada, c.state, c.listada)
			}
		})
	}
}
