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
	// Editar un gasto ya guardado (spec 053) mueve su importe, sus renglones y lo que sumó al almacén:
	// por omisión solo el administrador (#167); el reparto fino es #61.
	// Ver los entregados del día y reimprimir su ticket (068) es del mostrador: cajero sí, mesero no.
	// Sin él, una cajera no podía reimprimir el ticket de un pedido que entregó otro usuario y tenía
	// que entrar una administradora (2026-11-06). Devolver dinero sigue aparte, en payments.void.
	// Anular un gasto capturado por error (#216) lo hacía ya el gerente con los pendientes; ahora
	// alcanza también a los pagados, y sigue siendo de quien revisa, nunca de quien opera la caja.
	gerente := []Permission{PermPaymentsVoid, PermOrdersCancel, PermOrdersMoveLines, PermOrdersCancelPending,
		PermAttendanceReport, PermAttendanceCorrectOthers, PermAuditView, PermPaymentsChangeTerminal, PermCashConfigure,
		PermInventoryHealthView, PermMenuConfigure, PermInventoryReceive, PermInventoryConfigure, PermInventoryCount,
		PermInventoryWriteOff, PermInventoryWriteOffApprove, PermInventoryView, PermCashCloseWithoutClockOut, PermCashAdjustCount, PermExpensesCancel,
		PermOrdersDeliveredView,
		// Lo que hasta la 068 se pedía con RequireRole(admin, gerente) o (admin, gerente, cajero): mismo
		// conjunto de roles, ahora como permiso por capacidad.
		PermCashOperate, PermOrdersPlatformRef, PermPlatformPricesCapture, PermSalesView,
		PermPlatformSettlementsCapture, PermReportsView, PermExpensesCapture, PermPlatformsConfigure,
		PermBusinessConfigure,
		// Reportar que algo se acabó (084): lo ve quien vende; los cuatro sugeridos lo traen.
		PermInventoryReportOut}
	// El cajero abría caja, corregía el folio de plataforma y capturaba precios por plataforma
	// (RequireRole admin, gerente, cajero); el mesero, nada de eso.
	cajero := []Permission{PermOrdersMoveLines, PermOrdersCancelPending, PermInventoryWriteOff, PermOrdersDeliveredView,
		PermCashOperate, PermOrdersPlatformRef, PermPlatformPricesCapture, PermInventoryReportOut}
	cases := []struct {
		role Role
		want []Permission
	}{
		{RoleAdmin, all},
		{RoleGerente, gerente},
		{RoleCajero, cajero},
		{RoleMesero, []Permission{PermOrdersMoveLines, PermOrdersCancelPending, PermInventoryWriteOff, PermInventoryReportOut}},
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
		PermPaymentsVoid:               "Tu usuario no puede devolver pagos",
		PermOrdersCancel:               "Tu usuario no puede cancelar pedidos",
		PermOrdersMoveLines:            "Tu usuario no puede pasar productos",
		PermOrdersCancelPending:        "Tu usuario no puede quitar productos",
		PermAttendanceConfigure:        "Tu usuario no puede configurar la asistencia",
		PermAttendanceReport:           "Tu usuario no puede ver la asistencia del personal",
		PermAttendanceCorrectOthers:    "Tu usuario no puede corregir la asistencia de otras personas",
		PermAttendanceCorrectOwn:       "Tu usuario no puede corregir su propia asistencia",
		PermAuditView:                  "Tu usuario no puede ver la bitácora",
		PermPaymentsChangeTerminal:     "Tu usuario no puede cambiar la terminal de un pago",
		PermCashConfigure:              "Tu usuario no puede configurar la caja",
		PermInventoryHealthView:        "Tu usuario no puede ver la salud del inventario",
		PermMenuConfigure:              "Tu usuario no puede configurar el menú",
		PermInventoryReceive:           "Tu usuario no puede registrar lo que llega al almacén",
		PermInventoryConfigure:         "Tu usuario no puede configurar el almacén",
		PermInventoryCount:             "Tu usuario no puede contar el almacén",
		PermInventoryWriteOff:          "Tu usuario no puede registrar lo que se echó a perder",
		PermInventoryReportOut:         "Tu usuario no puede avisar que algo se acabó",
		PermInventoryWriteOffApprove:   "Tu usuario no puede aprobar lo que se echó a perder",
		PermInventoryView:              "Tu usuario no puede ver el almacén",
		PermCashCloseWithoutClockOut:   "Tu usuario no puede cerrar la caja sin la salida de quien la abrió",
		PermCashAdjustCount:            "Tu usuario no puede cambiar lo que entra al arqueo",
		PermExpensesEdit:               "Tu usuario no puede editar gastos",
		PermExpensesCancel:             "Tu usuario no puede anular gastos",
		PermOrdersDeliveredView:        "Tu usuario no puede ver los pedidos entregados",
		PermCashOperate:                "Tu usuario no puede operar la caja",
		PermOrdersPlatformRef:          "Tu usuario no puede corregir el folio de plataforma",
		PermPlatformPricesCapture:      "Tu usuario no puede capturar precios por plataforma",
		PermSalesView:                  "Tu usuario no puede ver las ventas",
		PermPlatformSettlementsCapture: "Tu usuario no puede capturar liquidaciones de plataforma",
		PermReportsView:                "Tu usuario no puede ver los reportes",
		PermExpensesCapture:            "Tu usuario no puede capturar gastos",
		PermPlatformsConfigure:         "Tu usuario no puede configurar las plataformas",
		PermPlatformsCredentials:       "Tu usuario no puede cambiar el acceso a las apps de plataforma",
		PermBusinessConfigure:          "Tu usuario no puede configurar el negocio",
		PermEmployeesManage:            "Tu usuario no puede dar de alta ni editar empleados",
		PermRolesManage:                "Tu usuario no puede cambiar lo que puede cada rol",
		PermIntegrationsManage:         "Tu usuario no puede crear ni revocar tokens de integración",
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

// Cada permiso tiene área y etiqueta: la pantalla de roles los agrupa por área, y uno sin área no
// aparecería en ninguna y nadie podría darlo ni quitarlo.
func TestEveryPermissionHasAreaAndLabel(t *testing.T) {
	areas := map[string]bool{}
	for _, a := range PermissionAreas() {
		areas[a] = true
	}
	for _, p := range AllPermissions() {
		if !areas[PermissionArea(p)] {
			t.Errorf("%q tiene área %q, que no está en PermissionAreas()", p, PermissionArea(p))
		}
		if PermissionLabel(p) == "" || PermissionLabel(p) == string(p) {
			t.Errorf("%q no tiene etiqueta para quien opera", p)
		}
	}
}
