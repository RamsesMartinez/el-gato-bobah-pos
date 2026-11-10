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
)

// AllAPIScopes son los alcances en su orden canónico (el que se guarda y se muestra).
func AllAPIScopes() []APIScope {
	return []APIScope{ScopeRead, ScopeWriteExpenses, ScopeWriteCash, ScopeWriteInventory, ScopeWriteCatalog}
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

// Estados de una clave de idempotencia; los mismos que el check de la migración 0111.
const (
	IdemStatusPending = "pending"
	IdemStatusDone    = "done"
	IdemStatusUnknown = "unknown"
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
)

// DecideIdempotency decide con lo guardado (nil si la clave es nueva) y la huella de la llamada.
func DecideIdempotency(existing *IdempotencyRecord, argsHash []byte) IdempotencyDecision {
	if existing == nil {
		return IdemProceed
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
	rd("stock_levels", "Existencias actuales de insumos y productos.", PermInventoryView),
	rd("stock_movements", "Movimientos de almacén recientes.", PermInventoryView),
	rd("waste_reasons_list", "Motivos de merma.", PermInventoryWriteOff),
	wr("waste_record", "Registra lo que se echó a perder. Lo que pasa el tope de la empresa queda por aprobar.", ScopeWriteInventory, PermInventoryWriteOff),
	wr("stock_adjust", "Ajusta la existencia de un insumo o producto (cantidad con signo).", ScopeWriteInventory, PermInventoryCount),
	rd("products_list", "Productos del menú con precio, costo y si están activos; con búsqueda.", PermMenuConfigure),
	rd("modifier_options_list", "Extras (opciones de modificador) con su precio y grupo; con búsqueda.", PermMenuConfigure),
	rd("platform_prices_list", "Plataformas de reparto y los precios propios de cada producto y extra.", ""),
	wr("product_update", "Cambia el precio o si está activo un producto. No toca nada más.", ScopeWriteCatalog, PermMenuConfigure),
	wr("modifier_option_update", "Cambia el precio o si está activo un extra. No toca nada más.", ScopeWriteCatalog, PermMenuConfigure),
	wr("platform_price_set", "Pone el precio de un producto o extra en una plataforma de reparto.", ScopeWriteCatalog, PermPlatformPricesCapture),
	wr("platform_price_delete", "Quita el precio propio de un producto o extra en una plataforma.", ScopeWriteCatalog, PermPlatformPricesCapture),
	rd("audit_list", "Bitácora de auditoría: quién hizo qué y cuándo, paginada.", PermAuditView),
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
