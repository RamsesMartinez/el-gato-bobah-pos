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
)

// AllPermissions devuelve todos los permisos que existen, en un orden fijo.
func AllPermissions() []Permission {
	return []Permission{PermPaymentsVoid, PermOrdersCancel, PermOrdersMoveLines, PermOrdersCancelPending,
		PermAttendanceConfigure, PermAttendanceReport, PermAttendanceCorrectOthers, PermAttendanceCorrectOwn,
		PermAuditView}
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
			PermAttendanceReport, PermAttendanceCorrectOthers, PermAuditView}
	case RoleCajero, RoleMesero:
		return []Permission{PermOrdersMoveLines, PermOrdersCancelPending}
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
	}
	return "Tu usuario no puede hacer esto"
}
