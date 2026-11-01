package domain

import (
	"slices"
	"strings"
	"testing"
)

// El mapa de hoy, rol por rol. Si alguien le quita `orders.cancel_pending` a un rol, un pedido sin
// productos vuelve a quedarse sin salida para quien atiende el mostrador; si le da `payments.void`
// a un cajero, cualquiera puede sacar dinero cobrado del corte.
func TestPermissionsForEachRoleOfToday(t *testing.T) {
	all := AllPermissions()
	// El gerente revisa a los demás pero no se corrige a sí mismo ni configura la asistencia: una
	// corrección propia mueve horas pagadas sin que nadie más la vea (decisión del 2026-10-10).
	// Cambiar la terminal de un pago mueve dinero entre terminales del corte: lo hace quien lo revisa.
	// Ver la salud del inventario y cambiar qué resta un producto (spec 037) son de quien configura el
	// menú y el almacén; dárselos a un cajero le dejaría mover existencias desde la hoja de receta.
	// Recibir compras, configurar el almacén y contar (spec 038) mueven lo guardado y el costo de los
	// insumos: tampoco son de quien opera la caja.
	// Registrar lo que se echó a perder (spec 039) lo hace cualquiera del local (decisión del 9-oct);
	// aprobar la merma que pasa el tope, solo quien revisa.
	// Ver el catálogo de artículos (spec 042) es de quien hoy ve el almacén: administrador y gerente.
	// Cerrar la caja sin la salida de quien la abrió (spec 046) deja horas «sin salida»: lo autoriza
	// quien revisa; un cajero solo puede pedir que alguien lo haga.
	// Mover la hora de apertura y la casilla del arqueo de un gasto ya registrado (spec 048) cambian
	// el esperado de un corte: también de quien revisa.
	gerente := []Permission{PermPaymentsVoid, PermOrdersCancel, PermOrdersMoveLines, PermOrdersCancelPending,
		PermAttendanceReport, PermAttendanceCorrectOthers, PermAuditView, PermPaymentsChangeTerminal, PermCashConfigure,
		PermInventoryHealthView, PermMenuConfigure, PermInventoryReceive, PermInventoryConfigure, PermInventoryCount,
		PermInventoryWriteOff, PermInventoryWriteOffApprove, PermInventoryView, PermCashCloseWithoutClockOut, PermCashAdjustCount}
	cases := []struct {
		role Role
		want []Permission
	}{
		{RoleAdmin, all},
		{RoleGerente, gerente},
		{RoleCajero, []Permission{PermOrdersMoveLines, PermOrdersCancelPending, PermInventoryWriteOff}},
		{RoleMesero, []Permission{PermOrdersMoveLines, PermOrdersCancelPending, PermInventoryWriteOff}},
		{Role("dueño"), nil},
		{Role(""), nil},
	}
	for _, c := range cases {
		t.Run(string(c.role), func(t *testing.T) {
			got := PermissionsFor(c.role)
			if got == nil {
				t.Fatalf("PermissionsFor(%q) = nil: la sesión lo mandaría como null y no como []", c.role)
			}
			for _, p := range all {
				if slices.Contains(got, p) != slices.Contains(c.want, p) {
					t.Errorf("PermissionsFor(%q) contiene %q = %v, quiere %v",
						c.role, p, slices.Contains(got, p), slices.Contains(c.want, p))
				}
			}
			if len(got) != len(c.want) {
				t.Errorf("PermissionsFor(%q) = %v, quiere %v", c.role, got, c.want)
			}
		})
	}
}

// Cada permiso dice qué no puede hacer el usuario, y ninguno nombra un rol: con roles propios por
// empresa, «gerente» puede no existir, y un texto que lo nombre manda a buscar a alguien que no hay.
func TestEveryPermissionHasADeniedMessageWithoutRoleNames(t *testing.T) {
	want := map[Permission]string{
		PermPaymentsVoid:             "Tu usuario no puede devolver pagos",
		PermOrdersCancel:             "Tu usuario no puede cancelar pedidos",
		PermOrdersMoveLines:          "Tu usuario no puede pasar productos",
		PermOrdersCancelPending:      "Tu usuario no puede quitar productos",
		PermAttendanceConfigure:      "Tu usuario no puede configurar la asistencia",
		PermAttendanceReport:         "Tu usuario no puede ver la asistencia del personal",
		PermAttendanceCorrectOthers:  "Tu usuario no puede corregir la asistencia de otras personas",
		PermAttendanceCorrectOwn:     "Tu usuario no puede corregir su propia asistencia",
		PermAuditView:                "Tu usuario no puede ver la bitácora",
		PermPaymentsChangeTerminal:   "Tu usuario no puede cambiar la terminal de un pago",
		PermCashConfigure:            "Tu usuario no puede configurar la caja",
		PermInventoryHealthView:      "Tu usuario no puede ver la salud del inventario",
		PermMenuConfigure:            "Tu usuario no puede configurar el menú",
		PermInventoryReceive:         "Tu usuario no puede registrar lo que llega al almacén",
		PermInventoryConfigure:       "Tu usuario no puede configurar el almacén",
		PermInventoryCount:           "Tu usuario no puede contar el almacén",
		PermInventoryWriteOff:        "Tu usuario no puede registrar lo que se echó a perder",
		PermInventoryWriteOffApprove: "Tu usuario no puede aprobar lo que se echó a perder",
		PermInventoryView:            "Tu usuario no puede ver el almacén",
		PermCashCloseWithoutClockOut: "Tu usuario no puede cerrar la caja sin la salida de quien la abrió",
		PermCashAdjustCount:          "Tu usuario no puede cambiar lo que entra al arqueo",
	}
	for _, p := range AllPermissions() {
		msg := PermissionDeniedMessage(p)
		if msg != want[p] {
			t.Errorf("PermissionDeniedMessage(%q) = %q, quiere %q", p, msg, want[p])
		}
		for _, r := range []Role{RoleAdmin, RoleGerente, RoleCajero, RoleMesero} {
			if strings.Contains(strings.ToLower(msg), string(r)) {
				t.Errorf("PermissionDeniedMessage(%q) nombra el rol %q: %q", p, r, msg)
			}
		}
	}
	if len(AllPermissions()) != len(want) {
		t.Errorf("AllPermissions() = %v, quiere los %d de la tabla", AllPermissions(), len(want))
	}
}
