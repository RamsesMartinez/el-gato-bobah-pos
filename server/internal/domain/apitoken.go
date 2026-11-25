package domain

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// La puerta para agentes (spec 072): un token de acceso personal con el que un cliente MCP opera
// el negocio con la identidad de una persona real del POS. Aquí vive lo que no toca la base: el
// formato del token, los alcances, la decisión de idempotencia y el catálogo de herramientas.

// APIScope es lo que un token puede hacer encima de los permisos del rol de su dueño. Se guarda
// como dato (`api_tokens.scopes`, con un check que enumera los mismos valores): no se renombra.
type APIScope string

const (
	ScopeRead           APIScope = "read"
	ScopeWriteExpenses  APIScope = "write:expenses"
	ScopeWriteCash      APIScope = "write:cash"
	ScopeWriteInventory APIScope = "write:inventory"
	ScopeWriteCatalog   APIScope = "write:catalog"
	// ScopeWritePurchases (spec 073): compras con su mercancía. Toca dinero y almacén a la vez, por eso
	// no cabe en gastos ni en almacén.
	ScopeWritePurchases APIScope = "write:purchases"
	// ScopeWriteOrders (spec 073): cancelar pedidos, que puede devolver dinero ya cobrado. Desde la 090 también
	// capturar, entregar y cobrar pedidos, y anular, mover de terminal o reembolsar un pago.
	ScopeWriteOrders APIScope = "write:orders"
)

// AllAPIScopes son los alcances en su orden canónico (el que se guarda y se muestra).
func AllAPIScopes() []APIScope {
	return []APIScope{ScopeRead, ScopeWriteExpenses, ScopeWriteCash, ScopeWriteInventory, ScopeWriteCatalog,
		ScopeWritePurchases, ScopeWriteOrders}
}

// APITokenLifetimes son las únicas vigencias que se ofrecen, en días. No hay token sin fecha: una
// llave que no vence es una llave que nadie recuerda que existe.
var APITokenLifetimes = []int{30, 90, 180}

// MaxAPITokenName es el tope del nombre; el mismo que el check de la migración 0111.
const MaxAPITokenName = 60

// apiTokenRe es la única forma aceptada. Sin ceros a la izquierda ni signo: dos textos distintos
// nunca nombran al mismo token, y lo que no calza no llega a la base. El secreto va en hex y no en
// base64url porque éste trae `_`, el separador.
var apiTokenRe = regexp.MustCompile(`^egbpat_([1-9][0-9]{0,17})_([1-9][0-9]{0,17})_([0-9a-f]{64})$`)

// ParsedAPIToken son las partes de un token. CompanyID y TokenID son lo que el texto DICE; la
// empresa del principal sale de la fila que se encontró bajo RLS con esa empresa, no del texto.
type ParsedAPIToken struct {
	CompanyID int64
	TokenID   int64
	Secret    string
}

// FormatAPIToken arma el token que se muestra una sola vez.
func FormatAPIToken(companyID, tokenID int64, secret string) string {
	return fmt.Sprintf("egbpat_%d_%d_%s", companyID, tokenID, secret)
}

// ParseAPIToken separa un token. Cualquier forma distinta es ErrUnauthorized, sin decir qué falló.
func ParseAPIToken(raw string) (ParsedAPIToken, error) {
	m := apiTokenRe.FindStringSubmatch(raw)
	if m == nil {
		return ParsedAPIToken{}, ErrUnauthorized
	}
	company, err1 := strconv.ParseInt(m[1], 10, 64)
	id, err2 := strconv.ParseInt(m[2], 10, 64)
	if err1 != nil || err2 != nil {
		return ParsedAPIToken{}, ErrUnauthorized
	}
	return ParsedAPIToken{CompanyID: company, TokenID: id, Secret: m[3]}, nil
}

// APITokenSecretHash es la huella que se guarda. sha256 y no bcrypt: el secreto trae 256 bits
// aleatorios, nada que adivinar, y bcrypt costaría decenas de milisegundos en cada llamada.
func APITokenSecretHash(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// APITokenSecretMatches compara en tiempo constante; una huella vacía no coincide con nada.
func APITokenSecretMatches(secret string, hash []byte) bool {
	if len(hash) != sha256.Size {
		return false
	}
	return subtle.ConstantTimeCompare(APITokenSecretHash(secret), hash) == 1
}

// APITokenHint son los primeros cuatro caracteres del secreto: lo único del secreto que se guarda
// en claro, para reconocer un token pegado en algún lado sin poder usarlo.
func APITokenHint(secret string) string {
	if len(secret) < 4 {
		return secret
	}
	return secret[:4]
}

// APITokenInput es lo que pide quien crea un token.
type APITokenInput struct {
	Name          string
	Scopes        []string
	ExpiresInDays int
}

// NormalizedAPIToken es la entrada ya validada.
type NormalizedAPIToken struct {
	Name          string
	Scopes        []APIScope
	ExpiresInDays int
}

// Normalize valida y deja la entrada en su forma canónica: nombre recortado, `read` siempre, los
// alcances sin duplicados y en el orden de AllAPIScopes.
func (in APITokenInput) Normalize() (NormalizedAPIToken, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len([]rune(name)) > MaxAPITokenName {
		return NormalizedAPIToken{}, fmt.Errorf("%w: el nombre va de 1 a %d caracteres", ErrValidation, MaxAPITokenName)
	}
	if !slices.Contains(APITokenLifetimes, in.ExpiresInDays) {
		return NormalizedAPIToken{}, fmt.Errorf("%w: la vigencia es de 30, 90 o 180 días", ErrValidation)
	}
	want := map[APIScope]bool{ScopeRead: true}
	for _, s := range in.Scopes {
		sc := APIScope(s)
		if !slices.Contains(AllAPIScopes(), sc) {
			return NormalizedAPIToken{}, fmt.Errorf("%w: alcance desconocido %q", ErrValidation, s)
		}
		want[sc] = true
	}
	var scopes []APIScope
	for _, sc := range AllAPIScopes() {
		if want[sc] {
			scopes = append(scopes, sc)
		}
	}
	return NormalizedAPIToken{Name: name, Scopes: scopes, ExpiresInDays: in.ExpiresInDays}, nil
}

// ScopeAllows dice si un token con esos alcances puede usar una herramienta que pide `need`. No da
// permisos: la llamada además pasa por el permiso del rol en la ruta.
func ScopeAllows(scopes []APIScope, need APIScope) bool {
	return slices.Contains(scopes, need)
}

// MissingScopeMessage es el error de una herramienta cuyo alcance el token no tiene.
func MissingScopeMessage(need APIScope) string {
	return fmt.Sprintf("Este token no tiene el alcance %s. Crea uno con ese alcance en Integraciones.", need)
}

// ---- Idempotencia ----

// Estados de una clave de idempotencia; los mismos que el check de las migraciones 0111 y 0121.
const (
	IdemStatusPending = "pending"
	IdemStatusDone    = "done"
	IdemStatusUnknown = "unknown"
	// IdemStatusFailed: la escritura se rechazó o falló sin confirmar nada en la base (#335). Guarda el
	// error, para que se sepa con certeza que reintentar es seguro.
	IdemStatusFailed = "failed"
)

// IdempotencyRecord es lo que ya se guardó con una clave.
type IdempotencyRecord struct {
	Status   string
	ArgsHash []byte
}

// IdempotencyDecision es qué hacer con una escritura que trae una clave.
type IdempotencyDecision int

const (
	// IdemProceed: clave nueva, se aplica.
	IdemProceed IdempotencyDecision = iota
	// IdemReplay: la misma escritura ya terminó; se devuelve lo guardado sin repetirla.
	IdemReplay
	// IdemConflict: la clave ya se usó con otros argumentos (u otra herramienta).
	IdemConflict
	// IdemInProgress: la primera ejecución todavía corre.
	IdemInProgress
	// IdemUnknown: la primera terminó en error interno o se cortó; puede haberse aplicado. Se
	// revisa con una lectura en vez de reintentar: peor un rechazo de más que un gasto doble.
	IdemUnknown
	// IdemRetry: la primera falló sin aplicar nada; se vuelve a correr, con la misma huella o con los
	// argumentos corregidos (antes la clave se borraba en ese caso y se reusaba igual).
	IdemRetry
)

// DecideIdempotency decide con lo guardado (nil si la clave es nueva) y la huella de la llamada.
func DecideIdempotency(existing *IdempotencyRecord, argsHash []byte) IdempotencyDecision {
	if existing == nil {
		return IdemProceed
	}
	if existing.Status == IdemStatusFailed {
		return IdemRetry
	}
	if !bytes.Equal(existing.ArgsHash, argsHash) {
		return IdemConflict
	}
	switch existing.Status {
	case IdemStatusDone:
		return IdemReplay
	case IdemStatusPending:
		return IdemInProgress
	default:
		return IdemUnknown
	}
}

// IdempotencyMessage es el error que recibe el agente en cada decisión que no aplica.
func IdempotencyMessage(d IdempotencyDecision) string {
	switch d {
	case IdemConflict:
		return "Esa idempotency_key ya se usó con otros argumentos. Usa una clave nueva para una escritura distinta."
	case IdemInProgress:
		return "Esa idempotency_key todavía se está aplicando. Espera unos segundos y revisa con una lectura antes de reintentar."
	case IdemUnknown:
		return "La escritura con esa idempotency_key terminó sin respuesta y puede haberse aplicado. Revisa con una lectura; si no está, reintenta con una clave nueva."
	}
	return ""
}

var idemKeyRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{8,100}$`)

// ValidIdempotencyKey: 8 a 100 caracteres de un alfabeto que no necesita escaparse en ningún lado.
func ValidIdempotencyKey(k string) bool { return idemKeyRe.MatchString(k) }

// IdempotencyArgsHash es la huella de una escritura: la herramienta y sus argumentos canónicos (el
// JSON de su struct tipado, sin dry_run ni la clave). La herramienta va dentro para que la misma
// clave en otra herramienta no se confunda con una repetición.
func IdempotencyArgsHash(tool string, canonicalArgs []byte) []byte {
	h := sha256.New()
	h.Write([]byte(tool))
	h.Write([]byte{0})
	h.Write(canonicalArgs)
	return h.Sum(nil)
}

// ---- Errores ----

// InternalOutcome es lo que se sabe de una operación que falló por dentro.
type InternalOutcome int

const (
	// InternalRead: era una lectura; reintentar no cambia nada.
	InternalRead InternalOutcome = iota
	// InternalNotApplied: una escritura que no confirmó nada en la base.
	InternalNotApplied
	// InternalMayHaveApplied: una escritura que pudo haber confirmado antes de fallar.
	InternalMayHaveApplied
)

// InternalErrorHint es lo que recibe quien llamó cuando la operación falló por dentro (#335): el id del
// error —el mismo del correo al administrador y de la bitácora—, la causa por categoría y si reintentar
// es seguro. Nunca el texto del error: puede traer la consulta o datos de otra empresa.
func InternalErrorHint(errorID, cause string, outcome InternalOutcome) string {
	msg := "Error interno del servidor"
	if errorID != "" {
		msg += " (id del error: " + errorID + ")"
	}
	msg += ": " + cause + ". Se avisó al administrador."
	switch outcome {
	case InternalNotApplied:
		return msg + " No se aplicó nada: puedes reintentar con la misma idempotency_key."
	case InternalMayHaveApplied:
		return msg + " La escritura pudo haberse aplicado: revisa con una lectura antes de reintentar."
	}
	return msg + " Puedes volver a intentar."
}

// InternalCause traduce la clase de un error interno (la de alerts.ErrorKind: «pg:SQLSTATE» o el tipo
// de Go) a una causa que se puede decir sin revelar nada de adentro.
func InternalCause(kind string) string {
	code, isPG := strings.CutPrefix(kind, "pg:")
	switch {
	case isPG && strings.HasPrefix(code, "23"):
		return "una regla de los datos rechazó la escritura"
	case isPG && strings.HasPrefix(code, "40"):
		return "chocó con otra operación al mismo tiempo; reintentar suele resolverlo"
	case isPG && code == "57014", strings.Contains(kind, "deadlineExceeded"):
		return "la operación tardó demasiado"
	case isPG && strings.HasPrefix(code, "08"), strings.Contains(kind, "ConnectError"), strings.Contains(kind, "net.OpError"):
		return "la base de datos no respondió"
	case isPG:
		return "la base de datos rechazó la operación"
	}
	return "un error no previsto"
}

// ToolErrorHint traduce el estado y el mensaje de la operación a lo que recibe el agente: la causa
// y cómo corregirla. Un error interno no muestra su texto: puede traer detalles de la base.
func ToolErrorHint(status int, serverMsg, listTool string) string {
	causa := strings.TrimSpace(serverMsg)
	con := func(fix string) string {
		if causa == "" {
			return fix
		}
		return strings.TrimRight(causa, ".") + ". " + fix
	}
	switch {
	case status == 401:
		return "El token no es válido, está revocado o venció. Crea otro en Integraciones."
	case status == 403:
		return con("Pide a quien administra los roles que se lo dé a tu rol.")
	case status == 404:
		if listTool != "" {
			return fmt.Sprintf("No existe en esta empresa. Búscalo con %s.", listTool)
		}
		return "No existe en esta empresa."
	case status == 409:
		return con("Revisa el estado actual con una lectura antes de reintentar.")
	case status == 429:
		return con("Demasiadas llamadas. Espera un momento y vuelve a intentar.")
	case status >= 500:
		return "Error interno; puede que no se haya aplicado. Revisa con una lectura antes de reintentar."
	case status >= 400:
		return con("Revisa los argumentos y vuelve a intentar.")
	}
	return causa
}

// toolFixByCode es cómo corrige un agente los rechazos de la operación que la pantalla resuelve con
// una hoja propia: el agente no ve esa hoja, así que la corrección va en palabras.
var toolFixByCode = map[string]string{
	"EXPENSE_OUTSIDE_COUNT": "La caja no tiene un turno que tome este pago. Abre la caja, o manda " +
		"include_in_count: false en el pago para registrarlo fuera del arqueo.",
	"EXPENSE_BEFORE_LAST_CLOSE": "El día del pago ya está cortado. Manda include_in_count: false para " +
		"registrarlo fuera del arqueo, o cambia paid_on.",
	"NO_OPEN_REGISTER":       "Esa caja no tiene turno abierto. Revisa cuál está abierta con cash_registers_list.",
	"PLATFORM_PRICE_MANAGED": "Ese precio lo pone la plataforma conectada y no se cambia desde el POS.",
	// Las de la 073: abrir y cerrar caja, contar, cancelar y comprar.
	"OPENING_REASON_REQUIRED": "Lo contado no coincide con el cierre anterior: manda opening_reason " +
		"(last_count_wrong, float_changed, unrecorded_withdrawal u other con opening_reason_note).",
	"TIPS_DECISION_REQUIRED": "Hay propina pendiente: manda tips_decision con lo que decidió quien cierra.",
	"REFUND_FOLIO_REQUIRED":  "Se devuelve algo cobrado con tarjeta: manda card_folio con el folio que imprimió la terminal.",
	"TERMINAL_COUNT_REQUIRED": "Falta el corte de una terminal: manda terminal_counts con el total que marcó cada " +
		"terminal.",
	"CLOCK_IN_REQUIRED": "Quien abre la caja tiene que marcar su entrada en la tableta con su PIN antes de abrir. " +
		"Revisa con cash_session_attendance.",
	"OPENER_NOT_CLOCKED_OUT": "Quien abrió la caja no ha marcado su salida. Que la marque en la tableta, o manda " +
		"authorized_close_reason si tu rol puede cerrar sin esa salida.",
	"NEEDS_CONFIRM": "La cifra está fuera de lo de costumbre. Si es lo que se contó, repite ese renglón con " +
		"confirmed: true.",
	"TRANSFER_NEEDS_CONFIRM": "El sistema dice que el origen no alcanza. Pregúntale a la persona; solo si confirma " +
		"que la mercancía sí está ahí, repite el traspaso con confirm_negative: true y una idempotency_key nueva.",
	"INGREDIENT_MERGED": "Ese insumo se unió a otro: búscalo con articles_search y usa el que quedó.",
	"OPEN_ORDERS": "Hay pedidos sin terminar en este turno: entrégalos en la pantalla o cancélalos con " +
		"order_cancel antes de cerrar.",
	"UNPAID_ORDERS": "Hay pedidos entregados que deben: cóbralos en la pantalla o cancélalos antes de cerrar.",
	// Las de la 074: el protocolo de la edición de un gasto (spec 053).
	"EXPENSE_EDIT_STALE": "Algo cambió desde la vista previa: vuelve a pedir la vista previa y aplica con su " +
		"fingerprint (puedes repetir la misma idempotency_key).",
	"DECISION_REQUIRED": "La vista previa pidió una decisión: manda decisions con la clave y una de sus opciones " +
		"(lo que decida la persona).",
	// Las de la 076: la ficha del insumo. El id del que ya tiene el nombre lo agrega ToolErrorHintWith.
	"NAME_TAKEN":     "Usa ese insumo, o elige otro nombre.",
	"ALT_NAME_TAKEN": "Quita ese nombre de la lista, o pregunta a la persona si son el mismo insumo.",
	"STALE":          "La ficha cambió mientras se guardaba: vuelve a intentar.",
	// Las de la 077: desactivar un insumo. La regla es del servicio (#203); aquí solo va qué hacer.
	"ARCHIVE_HAS_STOCK": "Registra la merma (waste_record) o el ajuste (stock_adjust) y repite. Solo si la persona " +
		"decide desactivarlo con esa existencia, manda confirm_on_hand: true.",
	"IN_USE": "Quítalo de esas recetas, o cierra o descarta el conteo, en la pantalla; desactivarlo no cambia " +
		"ninguna receta ni producto.",
}

// ToolErrorHintFor es ToolErrorHint con el código de error de la API: si la operación tiene una
// corrección conocida, la dice en vez de la genérica.
func ToolErrorHintFor(status int, code, serverMsg, listTool string) string {
	if fix, ok := toolFixByCode[code]; ok && status < 500 {
		causa := strings.TrimRight(strings.TrimSpace(serverMsg), ".")
		if causa == "" {
			return fix
		}
		return causa + ". " + fix
	}
	return ToolErrorHint(status, serverMsg, listTool)
}

// ExistingRef es lo que la operación dice del registro que ya tiene un nombre: la pantalla lo usa para
// ofrecer «Abrir ese» o «Reactivar», y el agente solo lo ve si va en la pista.
type ExistingRef struct {
	ID       int64
	Archived bool
}

// ToolErrorHintWith es ToolErrorHintFor con el registro que ya existe: agrega «(id N, archivado)» a la
// causa para que el agente pueda usar ese registro en vez de crear otro.
func ToolErrorHintWith(status int, code, serverMsg, listTool string, ref ExistingRef) string {
	if ref.ID > 0 && status < 500 {
		label := fmt.Sprintf("id %d", ref.ID)
		if ref.Archived {
			label += ", archivado: se reactiva con ingredient_update y active: true"
		}
		serverMsg = fmt.Sprintf("%s (%s)", strings.TrimRight(strings.TrimSpace(serverMsg), "."), label)
	}
	return ToolErrorHintFor(status, code, serverMsg, listTool)
}

// ---- Catálogo ----

// MCPTool es una herramienta del servidor MCP: qué alcance del token pide y qué permiso del rol
// exige la ruta que la atiende. La ruta concreta vive en httpapi; que el permiso declarado aquí sea
// el de la ruta lo vigila TestEveryToolMatchesItsRouteGate.
type MCPTool struct {
	Name        string
	Description string
	Scope       APIScope
	// Permission vacío = la operación de la pantalla no pide permiso (cualquier sesión de la empresa).
	Permission Permission
	Write      bool
}

func rd(name, desc string, p Permission) MCPTool {
	return MCPTool{Name: name, Description: desc, Scope: ScopeRead, Permission: p}
}

func wr(name, desc string, s APIScope, p Permission) MCPTool {
	return MCPTool{Name: name, Description: desc, Scope: s, Permission: p, Write: true}
}

var mcpTools = []MCPTool{
	rd("whoami", "Quién eres para el POS: tu nombre, el token, sus alcances y tus permisos de hoy.", ""),
	rd("sales_summary", "Resumen de ventas de un rango: total, pedidos, ticket promedio, por método y por plataforma.", PermSalesView),
	rd("sales_list", "Lista paginada de ventas de un rango, con filtros de estado y tipo de servicio.", PermSalesView),
	rd("order_get", "Un pedido con sus renglones, pagos y estado.", ""),
	rd("report_products_sold", "Productos vendidos en un rango: cantidad e importe por producto.", PermReportsView),
	rd("attendance_report", "Reporte de asistencia del personal en un rango.", PermAttendanceReport),
	rd("cash_registers_list", "Las cajas del negocio y si tienen turno abierto.", PermCashOperate),
	rd("cash_concepts_list", "Conceptos con los que se registran las salidas de efectivo.", PermCashOperate),
	rd("cash_session_current", "El turno abierto de una caja: fondo, movimientos y efectivo esperado.", PermCashOperate),
	rd("cash_sessions_history", "Cortes de caja anteriores, paginados.", PermCashOperate),
	wr("cash_movement_create", "Registra una entrada o salida de efectivo en el turno abierto de una caja.", ScopeWriteCash, PermCashOperate),
	rd("expenses_list", "Gastos, paginados y filtrables por estado.", PermExpensesCapture),
	rd("expense_get", "Un gasto con sus renglones y pagos.", PermExpensesCapture),
	rd("expense_categories_list", "Categorías de gasto.", ""),
	rd("suppliers_list", "Proveedores.", PermExpensesCapture),
	rd("payment_methods_list", "Métodos de pago (efectivo, tarjeta, transferencia…).", ""),
	wr("expense_create", "Captura un gasto, con o sin su pago. Un pago en efectivo sale de la caja que se indique.", ScopeWriteExpenses, PermExpensesCapture),
	wr("expense_pay", "Agrega un pago a un gasto pendiente.", ScopeWriteExpenses, PermExpensesCapture),
	wr("expense_cancel", "Anula un gasto capturado por error.", ScopeWriteExpenses, PermExpensesCancel),
	wr("supplier_create", "Da de alta un proveedor.", ScopeWriteExpenses, PermExpensesCapture),
	rd("warehouses_list", "Almacenes de la sucursal; el de venta es de donde salen las ventas.", PermInventoryView),
	rd("stock_levels", "Existencias actuales de insumos y productos; cada renglón dice si está activo. Los insumos "+
		"inactivos no salen, igual que en la pantalla. Sin warehouse_id, las del almacén de venta; con all_warehouses, "+
		"la suma de todos.", PermInventoryView),
	rd("stock_movements", "Movimientos de almacén recientes, con su almacén; un traspaso dice el otro almacén. Sin "+
		"warehouse_id, los del almacén de venta; con all_warehouses, los de todos.", PermInventoryView),
	rd("waste_reasons_list", "Motivos de merma.", PermInventoryWriteOff),
	wr("waste_record", "Registra lo que se echó a perder, en el almacén de venta o en warehouse_id. Lo que pasa el "+
		"tope de la empresa queda por aprobar.", ScopeWriteInventory, PermInventoryWriteOff),
	wr("stock_adjust", "Ajusta la existencia de un insumo o producto (cantidad con signo), en el almacén de venta o en "+
		"warehouse_id.", ScopeWriteInventory, PermInventoryCount),
	wr("stock_transfer", "Traspasa mercancía de un almacén a otro de la sucursal (insumos o productos con existencia "+
		"propia, en su unidad base). Si el origen no alcanza, pide confirmar.", ScopeWriteInventory, PermInventoryTransfer),
	rd("stock_transfers_list", "Traspasos entre almacenes recientes, con sus renglones.", PermInventoryView),
	rd("products_list", "Productos del menú con precio, costo y si están activos; con búsqueda.", PermMenuConfigure),
	rd("modifier_options_list", "Extras (opciones de modificador) con su precio y grupo; con búsqueda.", PermMenuConfigure),
	rd("platform_prices_list", "Plataformas de reparto y los precios propios de cada producto y extra.", ""),
	wr("product_update", "Edita un producto: nombre, categoría, precio, costo (manual o de receta), favorito y si "+
		"está activo. Solo cambia lo que mandes.", ScopeWriteCatalog, PermMenuConfigure),
	wr("modifier_option_update", "Edita un extra: nombre, lo que suma al precio, tope por renglón y si está activo. "+
		"Solo cambia lo que mandes; encuentra también los extras de un grupo inactivo y lo avisa.", ScopeWriteCatalog, PermMenuConfigure),
	wr("platform_price_set", "Pone el precio de un producto o extra en una plataforma de reparto.", ScopeWriteCatalog, PermPlatformPricesCapture),
	wr("platform_price_delete", "Quita el precio propio de un producto o extra en una plataforma.", ScopeWriteCatalog, PermPlatformPricesCapture),
	rd("audit_list", "Bitácora de auditoría: quién hizo qué y cuándo, paginada.", PermAuditView),

	// Spec 073: compras, conteos, cancelar pedidos, abrir y cerrar caja.
	rd("articles_search", "Busca insumos y productos de reventa por nombre (o nombre alternativo). Cada renglón dice "+
		"si está activo; include_inactive: true agrega los insumos inactivos, para reactivarlos.", PermExpensesCapture),
	rd("units_list", "Unidades de medida con su equivalencia a la unidad base.", PermExpensesCapture),
	rd("presentations_list", "Empaques de compra de los insumos (bolsa de 5 kg, caja de 12…); con ingredient_id, solo los de ese insumo.", PermInventoryReceive),
	rd("pending_receipts_list", "Compras con mercancía que todavía no llega.", PermInventoryReceive),
	wr("purchase_create", "Captura una compra con su mercancía: entra al almacén y cambia el costo de cada insumo. "+
		"La vista previa propone insumos y muestra el costo nuevo; aplicar exige insumo y empaque o unidad en cada renglón. "+
		"Cada renglón entra al almacén de venta o al de su warehouse_id.",
		ScopeWritePurchases, PermExpensesCapture),
	wr("purchase_receive", "Recibe una compra pendiente: lo que llegó de cada renglón entra al almacén que el renglón ya "+
		"dice (el que se eligió al capturarla); para cambiarlo, expense_edit.",
		ScopeWritePurchases, PermInventoryReceive),
	rd("stock_count_current", "El conteo abierto del almacén de venta (o de warehouse_id) y qué artículos se pueden "+
		"contar. No dice lo que el sistema tiene.", PermInventoryCount),
	wr("stock_count_open", "Abre un conteo de existencias del almacén de venta o de warehouse_id; cerrarlo solo "+
		"ajusta ese almacén.", ScopeWriteInventory, PermInventoryCount),
	wr("stock_count_capture", "Captura lo contado de hasta 50 artículos, como lo dicta quien está en el almacén. "+
		"Transcribe la cifra dictada; nunca la estimes.", ScopeWriteInventory, PermInventoryCount),
	wr("stock_count_close", "Cierra el conteo. La vista previa muestra, por artículo, lo que el sistema tiene, lo "+
		"contado y el ajuste; aplicar reemplaza lo que el sistema tenía por lo contado.", ScopeWriteInventory, PermInventoryCount),
	wr("stock_count_discard", "Descarta el conteo abierto sin tocar el almacén.", ScopeWriteInventory, PermInventoryCount),
	wr("order_cancel", "Cancela un pedido con motivo. Si tiene cobros, pide refund: true (y card_folio si hubo "+
		"tarjeta): el dinero se devuelve y sale del corte.", ScopeWriteOrders, PermOrdersCancel),
	rd("cash_denominations_list", "Billetes y monedas con los que se cuenta el cajón.", PermCashOperate),
	rd("cash_session_attendance", "Qué pide la asistencia para abrir o cerrar una caja.", PermCashOperate),
	wr("cash_session_open", "Abre una caja con el fondo contado (por billetes y monedas, o total con motivo).",
		ScopeWriteCash, PermCashOperate),
	wr("cash_session_close", "Cierra una caja con lo que contó la persona frente al cajón. Exige cada cifra: el "+
		"cajón y cada método que se declara. Nunca mandes lo que el sistema espera.", ScopeWriteCash, PermCashOperate),

	// Spec 074: empaques de compra y corregir un gasto guardado.
	wr("presentation_create", "Crea un empaque de compra de un insumo (bolsa de 375 g, caja de 12…), con marca "+
		"opcional. Marca y contenido los confirma la persona con el ticket; nunca los adivines.",
		ScopeWriteInventory, PermInventoryConfigure),
	wr("presentation_update", "Edita un empaque de compra: solo cambia lo que mandes. brand vacío quita la marca; "+
		"active: false deja de ofrecerlo.", ScopeWriteInventory, PermInventoryConfigure),
	wr("expense_edit", "Corrige un gasto o compra ya guardado: encabezado, renglones agregados (add) y quitados "+
		"(remove). Para cambiar el insumo o el empaque de un renglón, quítalo y agrégalo en la misma llamada. "+
		"La vista previa trae los efectos y un fingerprint; aplicar exige ese fingerprint y las decisiones que pidió. "+
		"Los pagos no cambian. Un renglón agregado entra al almacén de venta o al de su warehouse_id.", ScopeWriteExpenses, PermExpensesEdit),

	// Spec 084: alertas de stock bajo por lote.
	wr("low_stock_update", "Enciende o apaga la alerta de stock bajo de varios insumos o productos (con existencia "+
		"propia) a la vez, con su mínimo en la unidad base. La vista previa dice qué avisos se abrirían o cerrarían al "+
		"guardar. Los eventos y la franja de cada artículo se conservan. Con warehouse_id, min_stock es el mínimo "+
		"propio de ese almacén.", ScopeWriteInventory, PermInventoryConfigure),

	// Spec 076: dar de alta y editar insumos.
	wr("ingredient_create", "Da de alta un insumo (nombre, unidad base g, ml, cm o pieza, y lo demás opcional). "+
		"La vista previa pone primero los insumos parecidos que ya existen: si es uno de ellos, usa ese y no crees otro. "+
		"Si lo creas para una compra, la existencia y el costo llegan con la compra.", ScopeWriteInventory, PermInventoryConfigure),
	wr("ingredient_update", "Edita la ficha de un insumo: nombre, categoría, ubicación, mínimo y otros nombres. Solo "+
		"cambia lo que mandes; clear_* quita un valor; alt_names es la lista completa ([] los quita). active: false lo "+
		"desactiva (deja de salir en conteos, compras y existencias; su historial se conserva) y active: true lo "+
		"reactiva; active va solo en la llamada. La vista previa dice su existencia y las recetas que lo usan; "+
		"confirm_on_hand: true solo si la persona decide desactivarlo con existencia.",
		ScopeWriteInventory, PermInventoryConfigure),

	// Spec 090: pedidos completos, correcciones de cobro, costo con motivo, recetas, modificadores y productos.
	wr("order_draft_create", "Abre una cuenta (tipo de servicio, plataforma, folio de la plataforma, cliente, renglones "+
		"con extras). La vista previa trae el total que calcula el servidor con los precios de la plataforma y, por "+
		"renglón, si el precio es el propio de la plataforma o el base más su margen. Aplicar no la manda a cocina: "+
		"eso es order_draft_send.", ScopeWriteOrders, ""),
	wr("order_draft_send", "Manda a cocina una cuenta abierta: nace el pedido y lo vendido sale del almacén de venta. "+
		"Repetirlo no duplica el pedido.", ScopeWriteOrders, ""),
	wr("order_draft_discard", "Descarta una cuenta abierta que no se va a mandar.", ScopeWriteOrders, ""),
	wr("order_deliver", "Marca como entregado todo lo que falta de un pedido.", ScopeWriteOrders, ""),
	wr("order_discount_set", "Pone el descuento de un pedido (monto o porcentaje) o lo quita con clear: true.", ScopeWriteOrders, ""),
	wr("order_pay", "Cobra un pedido: todo lo que falta o un monto, con un método de pago. La vista previa dice cuánto "+
		"cobraría el servidor. Un pedido de plataforma solo se cobra con el método de su plataforma.", ScopeWriteOrders, ""),
	wr("order_payment_void", "Anula un pago de un turno abierto, con motivo: sale de los pagos y su monto vuelve a quedar "+
		"por cobrar. Para cambiar el método de un pago, anúlalo y cobra de nuevo con order_pay.", ScopeWriteOrders, PermPaymentsVoid),
	wr("order_line_cancel", "Quita un producto de un pedido aunque figure entregado, SOLO si ningún pago lo cubre "+
		"(agregado por error a un pedido ya cobrado), con motivo. Un producto pagado se devuelve con order_refund. "+
		"El pedido debe ser de un turno abierto.", ScopeWriteOrders, PermOrdersCancel),
	wr("order_payment_terminal_set", "Corrige en qué terminal se cobró un pago con tarjeta, con motivo.", ScopeWriteOrders, PermPaymentsChangeTerminal),
	wr("order_refund", "Reembolsa un pedido entregado (todo, un monto o un renglón), con motivo. La vista previa dice "+
		"cuánto queda por devolver, en qué terminal y si hace falta el folio de la terminal.", ScopeWriteOrders, PermPaymentsVoid),
	rd("card_terminals_list", "Las terminales de tarjeta del negocio.", PermCashOperate),
	wr("ingredient_cost_set", "Corrige el costo por unidad base de un insumo, con motivo (queda en su historial). La "+
		"vista previa trae el costo actual, el historial reciente y las recetas que lo usan. No mete mercancía ni crea "+
		"un gasto.", ScopeWriteInventory, PermInventoryConfigure),
	rd("recipe_get", "Lo que resta del almacén de venta un producto o un extra al venderse: su receta, si tiene existencia propia y los "+
		"extras que se llaman igual. Trae el based_on que pide recipe_set.", PermMenuConfigure),
	wr("recipe_set", "Guarda lo que resta un producto o un extra (insumos, el producto que es, los productos de un "+
		"paquete o nada). No copia la receta a los extras con el mismo nombre salvo also_in_same_name: true. Aplicar "+
		"pide el based_on de recipe_get o de la vista previa.", ScopeWriteCatalog, PermMenuConfigure),
	rd("categories_list", "Las categorías del menú, para dar de alta o mover un producto.", PermMenuConfigure),
	rd("modifier_groups_list", "Grupos de modificadores con su mínimo, máximo, si están activos y en cuántos productos están.", PermMenuConfigure),
	wr("modifier_group_create", "Crea un grupo de modificadores con su mínimo y máximo por omisión.", ScopeWriteCatalog, PermMenuConfigure),
	wr("modifier_group_update", "Edita un grupo: nombre, mínimo y máximo por omisión, y si está activo. Solo cambia lo "+
		"que mandes; al desactivarlo, la vista previa dice en qué productos deja de salir.", ScopeWriteCatalog, PermMenuConfigure),
	wr("modifier_option_create", "Crea un extra dentro de un grupo, con lo que suma al precio y su tope por renglón.", ScopeWriteCatalog, PermMenuConfigure),
	rd("product_groups_get", "Los grupos ligados a un producto, con título, mínimo, máximo, orden y si el grupo está activo.", PermMenuConfigure),
	wr("product_group_link", "Liga un grupo a un producto (o cambia su liga) con título, mínimo, máximo y orden; sin "+
		"mínimo ni máximo hereda los del grupo.", ScopeWriteCatalog, PermMenuConfigure),
	wr("product_group_unlink", "Quita un grupo de un producto.", ScopeWriteCatalog, PermMenuConfigure),
	wr("product_create", "Da de alta un producto (nombre, categoría, precio, costo manual opcional, existencia propia, "+
		"favorito). La vista previa pone primero los productos con nombre parecido.", ScopeWriteCatalog, PermMenuConfigure),

	// Spec 091: fotos del catálogo y familia de comida.
	wr("product_image_set", "Pone o reemplaza la foto de un producto: dos WebP cuadrados en base64 (1200×1200 hasta "+
		"400 KB y 400×400 hasta 100 KB). La vista previa los valida sin subir nada.", ScopeWriteCatalog, PermMenuConfigure),
	wr("product_image_remove", "Quita la foto de un producto; vuelve a verse la de su categoría o su dibujo.", ScopeWriteCatalog, PermMenuConfigure),
	wr("category_image_set", "Pone o reemplaza la foto de una categoría, con las mismas medidas que la de un producto. "+
		"Los productos sin foto propia la muestran.", ScopeWriteCatalog, PermMenuConfigure),
	wr("category_image_remove", "Quita la foto de una categoría; vuelve a verse el dibujo de su familia.", ScopeWriteCatalog, PermMenuConfigure),
	wr("category_family_set", "Elige la familia de comida de una categoría (el dibujo de lo que no tiene foto): "+
		"cold_drink, coffee, tea, frappe, soda, soup, snack, wings, dessert, breakfast o dish; vacío = genérica.", ScopeWriteCatalog, PermMenuConfigure),
}

// MCPTools devuelve el catálogo en el orden del contrato.
func MCPTools() []MCPTool { return slices.Clone(mcpTools) }

// MCPToolByName busca una herramienta del catálogo.
func MCPToolByName(name string) (MCPTool, bool) {
	i := slices.IndexFunc(mcpTools, func(t MCPTool) bool { return t.Name == name })
	if i < 0 {
		return MCPTool{}, false
	}
	return mcpTools[i], true
}

// IdempotencyKeyRef nombra una clave: de qué empresa y token, cuál y para qué herramienta.
type IdempotencyKeyRef struct {
	CompanyID int64
	TokenID   int64
	Key       string
	Tool      string
}
