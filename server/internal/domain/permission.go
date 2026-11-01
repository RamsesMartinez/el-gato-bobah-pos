package domain

// Permission es una acción que un usuario puede o no hacer, con nombre estable en inglés porque
// viaja como dato a la pantalla y algún día se guardará en la base.
type Permission string

const (
	PermPaymentsVoid        Permission = "payments.void"
	PermOrdersCancel        Permission = "orders.cancel"
	PermOrdersMoveLines     Permission = "orders.move_lines"
	PermOrdersCancelPending Permission = "orders.cancel_pending"

	// Asistencia y auditoría (spec 033). Corregir lo propio va aparte de corregir a otros: una
	// corrección propia mueve horas pagadas sin que nadie más la revise.
	PermAttendanceConfigure     Permission = "attendance.configure"
	PermAttendanceReport        Permission = "attendance.report"
	PermAttendanceCorrectOthers Permission = "attendance.correct_others"
	PermAttendanceCorrectOwn    Permission = "attendance.correct_own"
	PermAuditView               Permission = "audit.view"

	// Corregir en qué terminal se cobró un pago (spec 036). Mueve dinero entre terminales del corte.
	PermPaymentsChangeTerminal Permission = "payments.change_terminal"
	// PermCashConfigure: el fondo fijo de cada caja (spec 035). Cambia lo que el cierre propone dejar
	// y, con eso, cuánto se retira; por eso no es de quien opera la caja.
	PermCashConfigure Permission = "cash.configure"
	// PermCashCloseWithoutClockOut: cerrar la caja cuando quien la abrió se fue sin marcar su salida
	// (spec 046). Deja esa entrada «sin salida», que mueve horas pagadas: no es de quien opera la caja.
	PermCashCloseWithoutClockOut Permission = "cash.close_without_clock_out"

	// Salud del inventario (spec 037): ver qué no cuadra y exportarlo.
	PermInventoryHealthView Permission = "inventory.health_view"
	// PermMenuConfigure: decidir qué resta un producto o un extra. Desde ahí se mueven existencias al
	// salir de la existencia propia, así que no es de quien opera la caja.
	PermMenuConfigure Permission = "menu.configure"

	// Compras al almacén (spec 038). Recibir mueve lo guardado y el costo de los insumos; configurar
	// decide empaques, lugares y el método de costeo; contar reemplaza lo que el sistema creía tener.
	PermInventoryReceive   Permission = "inventory.receive"
	PermInventoryConfigure Permission = "inventory.configure"
	PermInventoryCount     Permission = "inventory.count"

	// Lo que se echó a perder (spec 039): registrarlo lo hace cualquiera del local; aprobar la merma
	// que pasa el tope de la empresa, quien revisa.
	PermInventoryWriteOff        Permission = "inventory.write_off"
	PermInventoryWriteOffApprove Permission = "inventory.write_off_approve"

	// Ver el catálogo de artículos, su ficha, su historial de costos y su bitácora (spec 042). Leer
	// va aparte de configurar para que un rol propio pueda consultar sin poder cambiar nada.
	PermInventoryView Permission = "inventory.view"
)

// AllPermissions devuelve todos los permisos que existen, en un orden fijo.
func AllPermissions() []Permission {
	return []Permission{PermPaymentsVoid, PermOrdersCancel, PermOrdersMoveLines, PermOrdersCancelPending,
		PermAttendanceConfigure, PermAttendanceReport, PermAttendanceCorrectOthers, PermAttendanceCorrectOwn,
		PermAuditView, PermPaymentsChangeTerminal, PermCashConfigure, PermInventoryHealthView, PermMenuConfigure,
		PermInventoryReceive, PermInventoryConfigure, PermInventoryCount, PermInventoryWriteOff, PermInventoryWriteOffApprove, PermInventoryView, PermCashCloseWithoutClockOut}
}

// PermissionsFor devuelve los permisos de un rol. Nunca devuelve nil: la sesión lo entrega como
// arreglo y un nil saldría como `null`.
//
// Este mapa fijo es lo ÚNICO que pasará a la base cuando cada empresa defina sus roles: los
// controles preguntan por permiso y no por nombre de rol, así que ninguno se reescribe ese día.
// Devolver un pago y cancelar un pedido sacan dinero del corte; pasar productos y quitar lo que
// falta no, y negárselos a quien atiende el mostrador deja pedidos sin salida.
func PermissionsFor(role Role) []Permission {
	switch role {
	case RoleAdmin:
		return AllPermissions()
	case RoleGerente:
		return []Permission{PermPaymentsVoid, PermOrdersCancel, PermOrdersMoveLines, PermOrdersCancelPending,
			PermAttendanceReport, PermAttendanceCorrectOthers, PermAuditView, PermPaymentsChangeTerminal, PermCashConfigure,
			PermInventoryHealthView, PermMenuConfigure, PermInventoryReceive, PermInventoryConfigure, PermInventoryCount,
			PermInventoryWriteOff, PermInventoryWriteOffApprove, PermInventoryView, PermCashCloseWithoutClockOut}
	case RoleCajero, RoleMesero:
		return []Permission{PermOrdersMoveLines, PermOrdersCancelPending, PermInventoryWriteOff}
	}
	return []Permission{}
}

// PermissionDeniedMessage es el texto que ve quien no tiene el permiso. No nombra roles: con roles
// propios por empresa, el rol que sí puede quizá no se llame igual o no exista.
func PermissionDeniedMessage(p Permission) string {
	switch p {
	case PermPaymentsVoid:
		return "Tu usuario no puede devolver pagos"
	case PermOrdersCancel:
		return "Tu usuario no puede cancelar pedidos"
	case PermOrdersMoveLines:
		return "Tu usuario no puede pasar productos"
	case PermOrdersCancelPending:
		return "Tu usuario no puede quitar productos"
	case PermAttendanceConfigure:
		return "Tu usuario no puede configurar la asistencia"
	case PermAttendanceReport:
		return "Tu usuario no puede ver la asistencia del personal"
	case PermAttendanceCorrectOthers:
		return "Tu usuario no puede corregir la asistencia de otras personas"
	case PermAttendanceCorrectOwn:
		return "Tu usuario no puede corregir su propia asistencia"
	case PermAuditView:
		return "Tu usuario no puede ver la bitácora"
	case PermPaymentsChangeTerminal:
		return "Tu usuario no puede cambiar la terminal de un pago"
	case PermCashConfigure:
		return "Tu usuario no puede configurar la caja"
	case PermInventoryHealthView:
		return "Tu usuario no puede ver la salud del inventario"
	case PermMenuConfigure:
		return "Tu usuario no puede configurar el menú"
	case PermInventoryReceive:
		return "Tu usuario no puede registrar lo que llega al almacén"
	case PermInventoryConfigure:
		return "Tu usuario no puede configurar el almacén"
	case PermInventoryCount:
		return "Tu usuario no puede contar el almacén"
	case PermInventoryWriteOff:
		return "Tu usuario no puede registrar lo que se echó a perder"
	case PermInventoryWriteOffApprove:
		return "Tu usuario no puede aprobar lo que se echó a perder"
	case PermInventoryView:
		return "Tu usuario no puede ver el almacén"
	case PermCashCloseWithoutClockOut:
		return "Tu usuario no puede cerrar la caja sin la salida de quien la abrió"
	}
	return "Tu usuario no puede hacer esto"
}
