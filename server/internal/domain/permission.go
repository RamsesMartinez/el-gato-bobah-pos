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

	// PermCashAdjustCount: abrir un corte desde una hora anterior y cambiar si un pago de gasto ya
	// registrado entra al arqueo (spec 048). Los dos mueven el esperado de un corte.
	PermCashAdjustCount Permission = "cash.adjust_count"
	// PermExpensesEdit: corregir un gasto ya guardado: su encabezado y agregar o quitar renglones
	// (spec 053). Mueve el importe y lo que la compra sumó al almacén; por omisión solo el
	// administrador (#167).
	PermExpensesEdit Permission = "expenses.edit"
	// PermExpensesCancel: anular un gasto capturado por error, aunque esté pagado (#216). Lo saca de
	// los reportes y, si su pago está en un corte abierto, cambia el esperado del cajón.
	PermExpensesCancel Permission = "expenses.cancel"

	// PermOrdersDeliveredView: ver los pedidos entregados del día y reimprimir su ticket (068). Es
	// del mostrador: sin él, una cajera no podía reimprimir el ticket de un pedido que entregó otro
	// usuario. Devolver dinero de esos pedidos va aparte, en PermPaymentsVoid.
	PermOrdersDeliveredView Permission = "orders.delivered_view"

	// Lo que hasta la 068 se pedía por nombre de rol, ahora por capacidad (roles por empresa). La
	// siembra de cada uno reproduce exactamente a quién dejaba pasar el RequireRole que reemplaza.
	PermCashOperate                Permission = "cash.operate"
	PermOrdersPlatformRef          Permission = "orders.platform_ref"
	PermPlatformPricesCapture      Permission = "platform_prices.capture"
	PermSalesView                  Permission = "sales.view"
	PermPlatformSettlementsCapture Permission = "platform_settlements.capture"
	PermReportsView                Permission = "reports.view"
	PermExpensesCapture            Permission = "expenses.capture"
	PermPlatformsConfigure         Permission = "platforms.configure"
	// PermPlatformsCredentials: el acceso a las apps de plataforma y las llaves de pedidos. Con él
	// se habla con Uber a nombre del negocio: por omisión solo el administrador.
	PermPlatformsCredentials Permission = "platforms.credentials"
	PermBusinessConfigure    Permission = "business.configure"
	PermEmployeesManage      Permission = "employees.manage"
	// PermRolesManage: decidir qué puede cada rol. El rol admin nunca lo pierde (ErrRoleAdminKeepsRoles):
	// una empresa sin quien corrija los roles no tiene salida.
	PermRolesManage Permission = "roles.manage"
	// PermIntegrationsManage: crear y revocar los tokens con los que un agente (Claude, por el
	// servidor MCP de la spec 072) opera el negocio sin pantalla. Una llave así trabaja desde fuera
	// del local: por omisión solo el administrador.
	PermIntegrationsManage Permission = "integrations.manage"
	// PermInventoryReportOut: avisar que algo se acabó o que no se encuentra (084). Deja la existencia
	// del artículo en 0 con un ajuste fijo, sin cantidad: por eso puede tenerlo quien no ajusta
	// existencias. Lo traen los cuatro sugeridos porque quien vende es quien ve que se acabó.
	PermInventoryReportOut Permission = "inventory.report_out"
)

// AllPermissions devuelve todos los permisos que existen, en un orden fijo.
func AllPermissions() []Permission {
	return []Permission{PermPaymentsVoid, PermOrdersCancel, PermOrdersMoveLines, PermOrdersCancelPending,
		PermAttendanceConfigure, PermAttendanceReport, PermAttendanceCorrectOthers, PermAttendanceCorrectOwn,
		PermAuditView, PermPaymentsChangeTerminal, PermCashConfigure, PermInventoryHealthView, PermMenuConfigure,
		PermInventoryReceive, PermInventoryConfigure, PermInventoryCount, PermInventoryWriteOff, PermInventoryWriteOffApprove, PermInventoryView, PermCashCloseWithoutClockOut, PermCashAdjustCount,
		PermExpensesEdit, PermExpensesCancel, PermOrdersDeliveredView,
		PermCashOperate, PermOrdersPlatformRef, PermPlatformPricesCapture, PermSalesView, PermPlatformSettlementsCapture,
		PermReportsView, PermExpensesCapture, PermPlatformsConfigure, PermPlatformsCredentials, PermBusinessConfigure,
		PermEmployeesManage, PermRolesManage, PermIntegrationsManage, PermInventoryReportOut}
}

// PermissionsFor devuelve los permisos de un rol. Nunca devuelve nil: la sesión lo entrega como
// arreglo y un nil saldría como `null`.
//
// Es la siembra de los roles sugeridos: la migración 0108 la copió a `system_role_permissions`, y
// TestThePermissionCatalogMatchesTheDomain exige que las dos digan lo mismo. Lo que puede un rol en
// una empresa lo decide la base (sugerido + ajustes de la empresa), nunca este mapa.
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
			PermInventoryWriteOff, PermInventoryWriteOffApprove, PermInventoryView, PermCashCloseWithoutClockOut, PermCashAdjustCount,
			PermExpensesCancel, PermOrdersDeliveredView,
			PermCashOperate, PermOrdersPlatformRef, PermPlatformPricesCapture, PermSalesView, PermPlatformSettlementsCapture,
			PermReportsView, PermExpensesCapture, PermPlatformsConfigure, PermBusinessConfigure, PermInventoryReportOut}
	case RoleCajero:
		return []Permission{PermOrdersMoveLines, PermOrdersCancelPending, PermInventoryWriteOff, PermOrdersDeliveredView,
			PermCashOperate, PermOrdersPlatformRef, PermPlatformPricesCapture, PermInventoryReportOut}
	case RoleMesero:
		return []Permission{PermOrdersMoveLines, PermOrdersCancelPending, PermInventoryWriteOff, PermInventoryReportOut}
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
	case PermInventoryReportOut:
		return "Tu usuario no puede avisar que algo se acabó"
	case PermInventoryWriteOffApprove:
		return "Tu usuario no puede aprobar lo que se echó a perder"
	case PermInventoryView:
		return "Tu usuario no puede ver el almacén"
	case PermCashCloseWithoutClockOut:
		return "Tu usuario no puede cerrar la caja sin la salida de quien la abrió"
	case PermExpensesEdit:
		return "Tu usuario no puede editar gastos"
	case PermCashAdjustCount:
		return "Tu usuario no puede cambiar lo que entra al arqueo"
	case PermExpensesCancel:
		return "Tu usuario no puede anular gastos"
	case PermOrdersDeliveredView:
		return "Tu usuario no puede ver los pedidos entregados"
	case PermCashOperate:
		return "Tu usuario no puede operar la caja"
	case PermOrdersPlatformRef:
		return "Tu usuario no puede corregir el folio de plataforma"
	case PermPlatformPricesCapture:
		return "Tu usuario no puede capturar precios por plataforma"
	case PermSalesView:
		return "Tu usuario no puede ver las ventas"
	case PermPlatformSettlementsCapture:
		return "Tu usuario no puede capturar liquidaciones de plataforma"
	case PermReportsView:
		return "Tu usuario no puede ver los reportes"
	case PermExpensesCapture:
		return "Tu usuario no puede capturar gastos"
	case PermPlatformsConfigure:
		return "Tu usuario no puede configurar las plataformas"
	case PermPlatformsCredentials:
		return "Tu usuario no puede cambiar el acceso a las apps de plataforma"
	case PermBusinessConfigure:
		return "Tu usuario no puede configurar el negocio"
	case PermEmployeesManage:
		return "Tu usuario no puede dar de alta ni editar empleados"
	case PermRolesManage:
		return "Tu usuario no puede cambiar lo que puede cada rol"
	case PermIntegrationsManage:
		return "Tu usuario no puede crear ni revocar tokens de integración"
	}
	return "Tu usuario no puede hacer esto"
}

// permissionMeta es el área y la etiqueta de cada permiso: la pantalla de roles los agrupa por área
// y los nombra para quien opera. El área se guarda también en la base (`permissions.area`).
var permissionMeta = map[Permission][2]string{
	PermOrdersDeliveredView:        {"orders", "Ver entregados y reimprimir su ticket"},
	PermOrdersMoveLines:            {"orders", "Pasar productos a otro pedido"},
	PermOrdersCancelPending:        {"orders", "Quitar lo que falta por entregar"},
	PermOrdersCancel:               {"orders", "Cancelar pedidos"},
	PermOrdersPlatformRef:          {"orders", "Corregir el folio de plataforma"},
	PermPaymentsVoid:               {"orders", "Devolver dinero"},
	PermPaymentsChangeTerminal:     {"orders", "Cambiar la terminal de un pago"},
	PermCashOperate:                {"cash", "Abrir, cerrar y operar la caja"},
	PermCashConfigure:              {"cash", "Configurar cajas, conceptos y terminales"},
	PermCashAdjustCount:            {"cash", "Cambiar lo que entra al arqueo"},
	PermCashCloseWithoutClockOut:   {"cash", "Cerrar sin la salida de quien abrió"},
	PermSalesView:                  {"sales", "Ver ventas"},
	PermPlatformSettlementsCapture: {"sales", "Capturar liquidaciones de plataforma"},
	PermReportsView:                {"sales", "Ver reportes"},
	PermExpensesCapture:            {"expenses", "Capturar y pagar gastos"},
	PermExpensesEdit:               {"expenses", "Editar gastos guardados"},
	PermExpensesCancel:             {"expenses", "Anular gastos"},
	PermInventoryView:              {"inventory", "Ver el almacén"},
	PermInventoryReceive:           {"inventory", "Recibir compras"},
	PermInventoryCount:             {"inventory", "Contar y ajustar existencias"},
	PermInventoryWriteOff:          {"inventory", "Registrar lo que se echó a perder"},
	PermInventoryReportOut:         {"inventory", "Avisar que algo se acabó o no se encuentra"},
	PermInventoryWriteOffApprove:   {"inventory", "Aprobar lo que se echó a perder"},
	PermInventoryConfigure:         {"inventory", "Configurar el almacén"},
	PermInventoryHealthView:        {"inventory", "Ver la salud del inventario"},
	PermMenuConfigure:              {"menu", "Configurar el menú y las recetas"},
	PermPlatformsConfigure:         {"platforms", "Configurar las plataformas"},
	PermPlatformPricesCapture:      {"platforms", "Capturar precios por plataforma"},
	PermPlatformsCredentials:       {"platforms", "Acceso a las apps de plataforma"},
	PermBusinessConfigure:          {"business", "Configurar el negocio y la impresión"},
	PermIntegrationsManage:         {"business", "Administrar integraciones (tokens para asistentes)"},
	PermAttendanceReport:           {"team", "Ver la asistencia del personal"},
	PermAttendanceCorrectOthers:    {"team", "Corregir la asistencia de otros"},
	PermAttendanceCorrectOwn:       {"team", "Corregir su propia asistencia"},
	PermAttendanceConfigure:        {"team", "Configurar la asistencia"},
	PermAuditView:                  {"team", "Ver la bitácora"},
	PermEmployeesManage:            {"team", "Administrar empleados"},
	PermRolesManage:                {"team", "Administrar roles"},
}

// PermissionAreas son las áreas en el orden en que la pantalla las muestra.
func PermissionAreas() []string {
	return []string{"orders", "cash", "sales", "expenses", "inventory", "menu", "platforms", "business", "team"}
}

// PermissionArea es el área de un permiso; "" si no existe.
func PermissionArea(p Permission) string { return permissionMeta[p][0] }

// PermissionLabel es el nombre que ve quien opera; "" si no existe.
func PermissionLabel(p Permission) string { return permissionMeta[p][1] }

// KnownPermission dice si el producto tiene ese permiso.
func KnownPermission(p Permission) bool { _, ok := permissionMeta[p]; return ok }
