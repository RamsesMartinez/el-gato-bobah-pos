package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Details lleva el error como DATO además de como prosa, para que el front pinte sin escarbar
	// en el texto del mensaje. Se omite cuando no hay nada que detallar: la respuesta de siempre.
	Details *errorDetails `json:"details,omitempty"`
}

// errorDetails son los campos estructurados que un error puede aportar: el producto que tumbó el
// cobro y, si el rechazo fue de un renglón de la cuenta, cuál (para señalarlo en la tableta).
type errorDetails struct {
	ProductID   int64  `json:"productId,omitempty"`
	ProductName string `json:"productName,omitempty"`
	LineID      string `json:"lineId,omitempty"`
	// ExistingID: el motivo de merma que ya existe con ese nombre (spec 039), para ofrecerlo.
	ExistingID int64 `json:"existingId,omitempty"`
	// Ingredients: cuántos artículos tiene el lugar que se quiere archivar (spec 040), para preguntar
	// a dónde pasan.
	Ingredients int32 `json:"ingredients,omitempty"`
	// Archived: el artículo con ese nombre está archivado (spec 042); la pantalla ofrece reactivarlo.
	Archived bool `json:"archived,omitempty"`
}

// JSON writes v as JSON with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// Error maps a domain error to an HTTP status + stable code and writes the envelope.
func Error(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "INTERNAL"
	msg := err.Error()

	switch {
	case errors.Is(err, domain.ErrNotFound):
		status, code = http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, domain.ErrInvalidCredentials):
		status, code = http.StatusUnauthorized, "INVALID_CREDENTIALS"
	case errors.Is(err, domain.ErrUnauthorized):
		status, code = http.StatusUnauthorized, "UNAUTHORIZED"
	// La consola de plataforma responde 401 con el MISMO código para usuario inexistente,
	// contraseña equivocada y operador desactivado. Un código propio por caso sería la
	// enumeración que el sentinel único existe para evitar.
	case errors.Is(err, domain.ErrCredencialDePlataforma):
		status, code = http.StatusUnauthorized, "INVALID_CREDENTIALS"
	case errors.Is(err, domain.ErrForbidden):
		status, code = http.StatusForbidden, "FORBIDDEN"
	case errors.Is(err, domain.ErrCobroFueraDeLugar):
		// 422 y no 400: el cuerpo está bien formado, lo que no se puede es la operación — crear un
		// pedido ya cobrado se salta la cocina. Va ANTES del caso de ErrValidation, que lo envuelve
		// y se lo llevaría a 400. El código propio deja que la pantalla diga qué hacer en vez de un
		// "datos inválidos" que no nombra el camino correcto.
		status, code = http.StatusUnprocessableEntity, "CONFIRMAR_PRIMERO"
	// Spec 032: rechazos a los que la pantalla de caja responde con su propia hoja. Antes que
	// ErrValidation, que los envuelve.
	case errors.Is(err, domain.ErrOpeningReasonRequired):
		status, code = http.StatusUnprocessableEntity, "OPENING_REASON_REQUIRED"
	case errors.Is(err, domain.ErrTipsDecisionNeeded):
		status, code = http.StatusUnprocessableEntity, "TIPS_DECISION_REQUIRED"
	case errors.Is(err, domain.ErrRefundFolioRequired):
		status, code = http.StatusUnprocessableEntity, "REFUND_FOLIO_REQUIRED"
	case errors.Is(err, domain.ErrCardTerminalRequired):
		status, code = http.StatusUnprocessableEntity, "CARD_TERMINAL_REQUIRED"
	case errors.Is(err, domain.ErrTerminalCountRequired):
		status, code = http.StatusUnprocessableEntity, "TERMINAL_COUNT_REQUIRED"
	case errors.Is(err, domain.ErrAttendanceDisabled):
		status, code = http.StatusConflict, "ATTENDANCE_DISABLED"
	case errors.Is(err, domain.ErrAttendanceStateChanged):
		status, code = http.StatusConflict, "ATTENDANCE_STATE_CHANGED"
	case errors.Is(err, domain.ErrAttendanceOverlap):
		status, code = http.StatusConflict, "ATTENDANCE_OVERLAP"
	// Caja con asistencia (spec 046): la hoja ofrece marcar entrada, marcar la salida de quien abrió
	// o pedir el PIN propio según el código.
	case errors.Is(err, domain.ErrCashClockInRequired):
		status, code = http.StatusConflict, "CLOCK_IN_REQUIRED"
	case errors.Is(err, domain.ErrCashOpenerClockedIn):
		status, code = http.StatusConflict, "OPENER_NOT_CLOCKED_OUT"
	case errors.Is(err, domain.ErrCashClockOutWithPIN):
		status, code = http.StatusConflict, "CLOCK_OUT_WITH_PIN"
	case errors.Is(err, domain.ErrValidation):
		status, code = http.StatusBadRequest, "VALIDATION"
	case errors.Is(err, domain.ErrPlatformNotFound):
		// 422 y no 404: el pedido en sí es válido, lo que no se puede es valuarlo con una lista de
		// precios que no es de este negocio. Nunca se cae a margen 0 — eso cobraría precio de
		// mostrador en una plataforma sin que nada avise.
		status, code = http.StatusUnprocessableEntity, "PLATFORM_NOT_FOUND"
	case errors.Is(err, domain.ErrPaymentMethodPlatform):
		// 422 y código propio: el pedido y el método existen, lo que no cuadra es la combinación.
		// El front lo necesita distinguible para decir con cuál sí se puede cobrar, en vez de un
		// "datos inválidos" que no dice qué corregir.
		status, code = http.StatusUnprocessableEntity, "PAYMENT_METHOD_PLATFORM"
	case errors.Is(err, domain.ErrOptionOverMax),
		errors.Is(err, domain.ErrOptionNotFound),
		errors.Is(err, domain.ErrProductNotSell):
		// Un producto que ya no se vende, un extra que ya no es del producto o más piezas de las
		// que hoy se permiten: la tableta armó la línea con un menú viejo (respeta los topes del
		// que tiene). Un solo código para que recargue el menú y pida elegir de nuevo sin
		// escarbar el texto (2026-10-10: se rechazó 18 veces la misma venta con un error rojo).
		status, code = http.StatusUnprocessableEntity, "MENU_CHANGED"
	case errors.Is(err, domain.ErrUnpaidOrders):
		// 409 con código propio, como OPEN_ORDERS: el cierre lista esos pedidos con «Cobrar» y
		// «Cancelar», y el mensaje nombra cada uno con lo que debe (no hay fiados, 2026-10-09).
		status, code = http.StatusConflict, "UNPAID_ORDERS"
	case errors.Is(err, domain.ErrOpenOrders):
		// 409 y código propio, igual que ErrNoOpenRegister: no es un error de lo que mandó el
		// cliente sino del estado del negocio. El front lo necesita distinguible para llevar al
		// operador al tablero con los folios que faltan, en vez de mostrar un error que no puede
		// accionar desde la pantalla de cierre.
		status, code = http.StatusConflict, "OPEN_ORDERS"
	// Emparejamiento (spec 026). Códigos propios porque cada uno pide a la pantalla algo distinto:
	// el precio bloqueado se explica, el de captura abre la elección y el de tipo no debería pasar.
	case errors.Is(err, domain.ErrPlatformPriceManaged):
		status, code = http.StatusConflict, "PLATFORM_PRICE_MANAGED"
	case errors.Is(err, domain.ErrCapturePriceRequired):
		status, code = http.StatusUnprocessableEntity, "CAPTURE_PRICE_REQUIRED"
	case errors.Is(err, domain.ErrLinkKindMismatch):
		status, code = http.StatusUnprocessableEntity, "LINK_KIND_MISMATCH"
	case errors.Is(err, domain.ErrBranchAmbiguous):
		// 409 y código propio: es el estado del negocio (dos sucursales y nadie eligió), no algo
		// que el operador mandó mal. Con un 500 genérico parecería una caída.
		status, code = http.StatusConflict, "BRANCH_AMBIGUOUS"
		msg = domain.ErrBranchAmbiguous.Error()
	case errors.Is(err, domain.ErrNoOpenRegister):
		// 409 y código propio: no es un error de lo que mandó el cliente sino del estado del
		// negocio. El front lo necesita distinguible para bloquear la pantalla de venta y mandar a
		// abrir turno, en vez de mostrar un mensaje que el operador no puede accionar desde ahí.
		status, code = http.StatusConflict, "NO_OPEN_REGISTER"
	// Una sola puerta para cobrar (spec 030). Todos ANTES de ErrConflict, que los envuelve: la
	// pantalla decide por el código qué ofrecer — recargar la cuenta, empezar una nueva, o nada.
	case errors.Is(err, domain.ErrDraftChanged):
		status, code = http.StatusConflict, "DRAFT_CHANGED"
	case errors.Is(err, domain.ErrDraftDiscarded):
		status, code = http.StatusConflict, "DRAFT_DISCARDED"
	case errors.Is(err, domain.ErrDraftAlreadySent):
		status, code = http.StatusConflict, "DRAFT_SENT"
	case errors.Is(err, domain.ErrOrderClosed):
		status, code = http.StatusConflict, "ORDER_CLOSED"
	case errors.Is(err, domain.ErrPlatformOrderNoLines):
		status, code = http.StatusUnprocessableEntity, "PLATFORM_ORDER_NO_LINES"
	case errors.Is(err, domain.ErrDraftHasOrderHeader):
		status, code = http.StatusUnprocessableEntity, "DRAFT_HAS_ORDER_HEADER"
	case errors.Is(err, domain.ErrPlatformRefTaken):
		// 409 con código propio, y va ANTES de ErrConflict, que lo envuelve. El front lo necesita
		// distinguible para llevar el foco al campo del folio con el pedido dueño a la vista, en
		// vez de un "conflicto" que no dice cuál es ni qué corregir.
		status, code = http.StatusConflict, "PLATFORM_REF_TAKEN"
	// Menús de plataforma (spec 020). Cada uno con su código propio porque la pantalla tiene que
	// decir cosas distintas: los tres primeros NO son errores del operador sino estados del mundo,
	// y un "conflicto" genérico no le diría qué hacer con ninguno.
	case errors.Is(err, domain.ErrPlataformaSinCredenciales):
		// 412 y no 404: la tienda existe, lo que falta es la configuración del despliegue. La
		// pantalla dice «esta tienda no está conectada», que es distinto de «no hay diferencias».
		status, code = http.StatusPreconditionFailed, "PLATFORM_NOT_CONFIGURED"
	// Captura de credenciales (0075). Un código por caso porque cada uno pide corregir algo
	// distinto, y la pantalla lo dice en palabras de quien opera, no las de Uber.
	case errors.Is(err, domain.ErrCredentialsRejected):
		status, code = http.StatusUnprocessableEntity, "PLATFORM_CREDENTIALS_REJECTED"
		msg = domain.ErrCredentialsRejected.Error()
	case errors.Is(err, domain.ErrCredentialsMissingScopes):
		status, code = http.StatusUnprocessableEntity, "PLATFORM_CREDENTIALS_MISSING_SCOPES"
		msg = domain.ErrCredentialsMissingScopes.Error()
	case errors.Is(err, domain.ErrShuttingDown):
		// 503: la copia que se apaga; el reintento cae en la que ya atiende.
		status, code = http.StatusServiceUnavailable, "SHUTTING_DOWN"
		msg = domain.ErrShuttingDown.Error()
	case errors.Is(err, domain.ErrPlatformUnavailable):
		// 503: no es culpa de quien captura y reintentar más tarde es lo correcto.
		status, code = http.StatusServiceUnavailable, "PLATFORM_UNAVAILABLE"
		msg = domain.ErrPlatformUnavailable.Error()
	case errors.Is(err, domain.ErrKeyServiceUnavailable):
		// 503 como la plataforma caída: no es culpa de quien captura y reintentar es lo correcto.
		// Mensaje fijo: el error envuelto trae el estado HTTP de Google, que no le sirve a nadie aquí.
		status, code = http.StatusServiceUnavailable, "KEY_SERVICE_UNAVAILABLE"
		msg = domain.ErrKeyServiceUnavailable.Error()
	case errors.Is(err, domain.ErrCredentialsUnreadable):
		// 412 como «sin credenciales»: la tienda existe, lo que falta es configurar — pero con su
		// propio código, porque la pantalla dice «vuelve a capturarlas», no «nunca se capturaron».
		status, code = http.StatusPreconditionFailed, "PLATFORM_CREDENTIALS_UNREADABLE"
	case errors.Is(err, domain.ErrLecturaEnCurso):
		status, code = http.StatusConflict, "READ_IN_PROGRESS"
	case errors.Is(err, domain.ErrSinLecturaValida), errors.Is(err, domain.ErrLecturaVacia):
		// 410: hubo o hay intento, pero no existe una foto contra la cual emparejar ni comparar.
		// Devolver 200 con una lista vacía haría creer que el menú coincide en todo.
		status, code = http.StatusGone, "NO_VALID_READ"
	case errors.Is(err, domain.ErrParejaOcupada):
		status, code = http.StatusConflict, "LINK_TAKEN"
	case errors.Is(err, domain.ErrConexionDuplicada):
		status, code = http.StatusConflict, "CONNECTION_EXISTS"
	case errors.Is(err, domain.ErrItemInexistente):
		// 404 y no 422: el id viajó bien, simplemente no está en la última lectura. Sin este caso
		// el PUT respondería 200 y la pantalla seguiría diciendo «sin pareja», sin error visible.
		status, code = http.StatusNotFound, "PLATFORM_ITEM_NOT_FOUND"
	case errors.Is(err, domain.ErrUnitChangePending):
		status, code = http.StatusConflict, "UNIT_CHANGE_PENDING"
	case errors.Is(err, domain.ErrCountNeedsConfirm):
		status, code = http.StatusConflict, "NEEDS_CONFIRM"
	case errors.Is(err, domain.ErrArticleInUse):
		status, code = http.StatusConflict, "IN_USE"
	case errors.Is(err, domain.ErrArticleStale):
		status, code = http.StatusConflict, "STALE"
	case errors.Is(err, domain.ErrPrepCost):
		status, code = http.StatusConflict, "PREP_COST"
	case errors.Is(err, domain.ErrArticleNameTaken):
		status, code = http.StatusConflict, "NAME_TAKEN"
	case errors.Is(err, domain.ErrConflict):
		status, code = http.StatusConflict, "CONFLICT"
	case errors.Is(err, domain.ErrTooManyRequests):
		status, code = http.StatusTooManyRequests, "TOO_MANY_REQUESTS"
	case errors.Is(err, domain.ErrDescuentoMayorQueLaVenta):
		// 422 y no 400: el monto está bien formado; lo que no se puede es descontar más de lo que
		// se vendió. El código propio deja que la pantalla ponga el máximo donde el operador lo
		// está tecleando, en vez de un aviso genérico de "datos inválidos".
		status, code = http.StatusUnprocessableEntity, "DISCOUNT_OVER_SUBTOTAL"
	case errors.Is(err, domain.ErrEmptyOrder):
		// reglas de negocio del armado del pedido: 422 con el mensaje real (no 500 opaco)
		status, code = http.StatusUnprocessableEntity, "UNPROCESSABLE"
	case errors.Is(err, domain.ErrWeakPassword):
		// política de contraseña: 422 con el motivo concreto (envuelto en el error)
		status, code = http.StatusUnprocessableEntity, "WEAK_PASSWORD"
	case errors.Is(err, domain.ErrResetInvalid):
		// enlace de recuperación inválido/usado/vencido: 410 Gone con mensaje accionable
		status, code = http.StatusGone, "RESET_INVALID"
	case errors.Is(err, app.ErrDocExtractDisabled):
		// 501 y no 500: la extracción de documentos es opcional y no está configurada. Distinguirlo
		// permite al front ofrecer la captura manual en vez de mostrar "error del servidor".
		status, code = http.StatusNotImplemented, "NOT_CONFIGURED"
	}

	if status == http.StatusInternalServerError {
		slog.Error("request failed", "error", err)
		msg = "Error interno del servidor"
		// Sin details en un 500: el mensaje ya se ocultó por opaco, y adjuntar el interior del
		// error por otra puerta lo desharía.
		JSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: msg}})
		return
	}

	// El producto que tumbó el cobro viaja también como dato. Ojo: ProductName va VACÍO cuando el
	// catálogo de la empresa no conoce el id — ver domain.ProductUnavailable; rellenarlo ahí
	// filtraría el menú de otra empresa.
	var details *errorDetails
	var pu domain.ProductUnavailable
	if errors.As(err, &pu) {
		details = &errorDetails{ProductID: pu.ProductID, ProductName: pu.Name}
	}
	var lr domain.LineRejected
	if errors.As(err, &lr) && lr.LineID != "" {
		if details == nil {
			details = &errorDetails{ProductID: lr.ProductID}
		}
		details.LineID = lr.LineID
	}
	var exists domain.WasteReasonExists
	if errors.As(err, &exists) {
		details = &errorDetails{ExistingID: exists.ID}
	}
	var place domain.CountPlaceExists
	if errors.As(err, &place) {
		details = &errorDetails{ExistingID: place.ID}
	}
	var article domain.ArticleNameTaken
	if errors.As(err, &article) {
		details = &errorDetails{ExistingID: article.ID, Archived: article.Archived}
	}
	var busy domain.CountPlaceHasIngredients
	if errors.As(err, &busy) {
		details = &errorDetails{Ingredients: busy.Ingredients}
	}
	JSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: operatorMessage(msg), Details: details}})
}

// sentinelPrefixes son los sentinels base que deciden el status. Se envuelven con `%w: texto`, y
// su nombre («conflicto», «datos inválidos») quedaba pegado delante de todo 400, 403, 404 y 409:
// un detalle de cómo se arman los errores en Go que quien opera no puede accionar.
var sentinelPrefixes = []string{
	domain.ErrConflict.Error() + ": ",
	domain.ErrValidation.Error() + ": ",
	domain.ErrForbidden.Error() + ": ",
	domain.ErrNotFound.Error() + ": ",
}

// operatorMessage quita los prefijos de los sentinels base del INICIO del mensaje, también los
// encadenados («conflicto: datos inválidos: …»). Solo del inicio: en medio del texto «no
// encontrado» es parte de la frase («Producto no encontrado: Taco»), no un prefijo. El status ya
// lo decidió errors.Is; el texto es solo para quien opera.
func operatorMessage(msg string) string {
	for {
		stripped := msg
		for _, p := range sentinelPrefixes {
			stripped = strings.TrimPrefix(stripped, p)
		}
		if stripped == msg {
			return msg
		}
		msg = stripped
	}
}

// Decode parses a JSON request body into v, returning a validation error on failure.
func Decode(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return domain.ErrValidation
	}
	return nil
}
