package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// El front pinta el nombre del producto que tumbó el cobro, y para eso lo necesita como DATO, no
// escarbado del texto del mensaje: un mensaje es prosa que cambia, y parsearlo en el cliente es
// justo la lógica que no debe vivir ahí.
func TestErrorProductoNoDisponibleViajaConNombreEId(t *testing.T) {
	casos := []struct {
		nombre     string
		err        error
		wantName   string
		wantStatus int
	}{
		{"producto desactivado", domain.ProductUnavailable{ProductID: 7, Name: "Chococino"}, "Chococino", 422},
		{"producto fuera del menú", domain.ProductUnavailable{ProductID: 510}, "", 422},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			w := httptest.NewRecorder()
			Error(w, c.err)

			if w.Code != c.wantStatus {
				t.Fatalf("status = %d, quería %d", w.Code, c.wantStatus)
			}
			var got struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
					Details *struct {
						ProductID   int64  `json:"productId"`
						ProductName string `json:"productName"`
					} `json:"details"`
				} `json:"error"`
			}
			if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
				t.Fatalf("respuesta ilegible: %v", err)
			}
			if got.Error.Code != "UNPROCESSABLE" {
				t.Fatalf("code = %q", got.Error.Code)
			}
			if got.Error.Details == nil {
				t.Fatal("falta details: el front no tendría de dónde sacar el producto")
			}
			if got.Error.Details.ProductID != c.err.(domain.ProductUnavailable).ProductID {
				t.Fatalf("productId = %d", got.Error.Details.ProductID)
			}
			if got.Error.Details.ProductName != c.wantName {
				t.Fatalf("productName = %q, quería %q", got.Error.Details.ProductName, c.wantName)
			}
		})
	}
}

// Un error cualquiera no debe cargar `details`: es opcional y sin él la respuesta es la de antes.
func TestErrorSinDetallesNoTraeElCampo(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, domain.ErrNotFound)

	var got map[string]map[string]any
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("respuesta ilegible: %v", err)
	}
	if _, hay := got["error"]["details"]; hay {
		t.Fatal("details no debe aparecer cuando no hay nada que detallar")
	}
}

// Un folio repetido sale como 409 con código PROPIO, no como el CONFLICT genérico.
//
// El caso va antes de ErrConflict en el switch —que es quien lo envuelve—, y este test es lo que
// impide que alguien lo mueva después y se lo lleve al genérico sin que nada falle: la pantalla
// necesita el código distinguible para llevar el foco al campo del folio con el pedido dueño a la
// vista, en vez de un "conflicto" que no dice qué corregir.
func TestElFolioRepetidoTieneSuPropioCodigo(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, fmt.Errorf("ese folio de Uber Eats ya está en el pedido Tigre (#187) del 5 de septiembre (%w)",
		domain.ErrPlatformRefTaken))

	if rec.Code != http.StatusConflict {
		t.Fatalf("respondió %d y debía ser 409", rec.Code)
	}
	var sobre struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &sobre); err != nil {
		t.Fatalf("el cuerpo no es el sobre de siempre: %v", err)
	}
	if sobre.Error.Code != "PLATFORM_REF_TAKEN" {
		t.Fatalf("el código quedó en %q: cayó al CONFLICT genérico y la pantalla no puede distinguirlo",
			sobre.Error.Code)
	}
	// El mensaje conserva QUÉ pedido lo tiene. Sin eso, el operador busca a ciegas entre las ventas
	// del día con el repartidor esperando.
	if !strings.Contains(sobre.Error.Message, "Tigre") || !strings.Contains(sobre.Error.Message, "#187") {
		t.Fatalf("el mensaje perdió el pedido que ya tiene el folio: %q", sobre.Error.Message)
	}
}

// DOS SUCURSALES Y NADIE ELIGIÓ ES UN 409 CON SU CÓDIGO, NO UN 500. El error nace en un trigger de
// la base (EGB01) y store lo traduce envolviendo el mensaje de Postgres, que trae el id de la
// empresa: la respuesta lleva el texto para quien opera, no ese interior.
func TestAmbiguousBranchFromTheDatabaseIsA409WithItsOwnCode(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, fmt.Errorf("crear pedido: %w (branch_ambiguous: la empresa 2 tiene 2 sucursales activas)",
		domain.ErrBranchAmbiguous))
	if rec.Code != http.StatusConflict {
		t.Fatalf("respondió %d y debía ser 409", rec.Code)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "BRANCH_AMBIGUOUS" {
		t.Fatalf("código %q", body.Error.Code)
	}
	if strings.Contains(body.Error.Message, "empresa 2") {
		t.Fatalf("el mensaje expone el interior del error: %q", body.Error.Message)
	}
}

// KMS SIN RESPONDER AL GUARDAR ES UN 503 CON SU CÓDIGO, NO UN 500. No es culpa de quien captura y
// reintentar es lo correcto; y el mensaje no trae el interior del error (la URL de Google, el estado
// HTTP), que en un 500 se oculta pero aquí viajaría.
func TestKeyServiceDownIsA503WithItsOwnCode(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, fmt.Errorf("cifrar el client secret: %w: %w", domain.ErrKeyServiceUnavailable,
		errors.New("secrets: decrypt returned HTTP 503 https://cloudkms.googleapis.com/v1/projects/p")))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("respondió %d y debía ser 503", rec.Code)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "KEY_SERVICE_UNAVAILABLE" {
		t.Fatalf("código %q", body.Error.Code)
	}
	if strings.Contains(body.Error.Message, "googleapis") || strings.Contains(body.Error.Message, "HTTP") {
		t.Fatalf("el mensaje expone el interior del error: %q", body.Error.Message)
	}
}

// errorOf corre Error y devuelve el status y el sobre, para leer lo que ve quien opera.
func errorOf(t *testing.T, err error) (int, errorBody) {
	t.Helper()
	w := httptest.NewRecorder()
	Error(w, err)
	var env errorEnvelope
	if e := json.Unmarshal(w.Body.Bytes(), &env); e != nil {
		t.Fatalf("respuesta ilegible: %v (%s)", e, w.Body.String())
	}
	return w.Code, env.Error
}

// El prefijo del sentinel («conflicto:», «datos inválidos:») es un detalle de cómo se envuelven los
// errores en Go, no algo que quien opera pueda accionar. Llegaba a la pantalla en TODOS los 400 y
// 409, también en los que envuelven dos veces, y en los 422 que cuelgan de ErrValidation.
func TestOperatorMessagesCarryNoSentinelPrefix(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{"409 envuelto una vez", domain.ErrPedidoYaPagado, 409, "ese pedido ya está cobrado"},
		{"409 con contexto", fmt.Errorf("%w (línea %d)", domain.ErrLineaCancelada, 5), 409, "ese producto está cancelado (línea 5)"},
		{"400 envuelto una vez", domain.ErrEntregaInvalida, 400, "la cantidad a entregar tiene que ser mayor que cero"},
		{"400 envuelto dos veces", fmt.Errorf("%w: faltan 150 y se intentó cobrar 151", domain.ErrCobroExcede), 400,
			"no puedes cobrar más de lo que falta de ese pedido: faltan 150 y se intentó cobrar 151"},
		{"422 que envuelve ErrValidation", domain.ErrCobroFueraDeLugar, 422,
			"el pedido se confirma primero y se cobra después, con /pay"},
		{"403 con texto", fmt.Errorf("%w: %s", domain.ErrForbidden, "Tu usuario no puede devolver pagos"), 403,
			"Tu usuario no puede devolver pagos"},
		{"404 con texto", fmt.Errorf("%w: Ese producto ya no está en el pedido", domain.ErrNotFound), 404,
			"Ese producto ya no está en el pedido"},
		{"sentinels encadenados al inicio", fmt.Errorf("%w: %w: Ese número no existe", domain.ErrConflict, domain.ErrValidation), 400,
			"Ese número no existe"},
		// El nombre del sentinel también es español de todos los días: en medio del texto es parte
		// de la frase, no un prefijo. Quitarlo ahí dejaba «Producto Taco».
		{"el nombre del sentinel en medio del texto se queda", fmt.Errorf("%w: Producto no encontrado: Taco", domain.ErrNotFound), 404,
			"Producto no encontrado: Taco"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, body := errorOf(t, c.err)
			if status != c.wantStatus {
				t.Fatalf("status = %d, quiere %d", status, c.wantStatus)
			}
			if body.Message != c.wantMsg {
				t.Fatalf("mensaje = %q, quiere %q", body.Message, c.wantMsg)
			}
		})
	}
}

// Cancelar un pedido con algo entregado ofrece «Quitar lo que falta» y «Cerrar pedido». El texto
// viejo mandaba a «hacer un reembolso», que es salida de dinero y no lo que hace falta ahí.
func TestCancelWithDeliveriesDoesNotSuggestARefund(t *testing.T) {
	_, body := errorOf(t, domain.ErrCancelarConEntregas)
	if strings.Contains(strings.ToLower(body.Message), "reembols") {
		t.Fatalf("el texto sugiere un reembolso: %q", body.Message)
	}
	if strings.HasPrefix(body.Message, "conflicto") {
		t.Fatalf("el texto lleva el prefijo del sentinel: %q", body.Message)
	}
}

// Cada sentinel nuevo de dividir la cuenta sale con su status, su código y el texto exacto de
// contracts/api.md. Un sentinel que cayera al caso por defecto respondería 500 con «Error interno»
// justo cuando quien opera tiene al cliente enfrente.
func TestSplitBillSentinelsMapToStatusCodeAndText(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
		text   string
	}{
		{domain.ErrPieceAlreadyPaid, 409, "CONFLICT", "Ese producto ya se pagó"},
		{domain.ErrSplitPartAlreadyCharged, 409, "CONFLICT", "Esa parte ya se cobró"},
		{domain.ErrChargeKeyMismatch, 409, "CONFLICT", "Ese cobro ya se hizo con otros productos. Vuelve a intentarlo"},
		{domain.ErrPaymentVoidedKey, 409, "CONFLICT", "Ese pago ya se devolvió. Vuelve a cobrar"},
		{domain.ErrPaymentAlreadyVoided, 409, "CONFLICT", "Ese pago ya se devolvió"},
		{domain.ErrPaymentFromClosedShift, 409, "CONFLICT", "Ese pago es de un turno cerrado: devuélvelo desde Pedidos entregados"},
		{domain.ErrOrderFromClosedShift, 409, "CONFLICT", "Ese pedido es de un turno cerrado; no se divide"},
		{domain.ErrPlatformOrderNotSplittable, 409, "CONFLICT", "Los pedidos de plataforma no se dividen"},
		{domain.ErrOrderWouldBeOverpaid, 409, "CONFLICT", "Ya se cobró más de lo que quedaría. Primero hay que devolver un pago"},
		{domain.ErrMixedDeliveredPieces, 409, "CONFLICT", "Ese producto tiene piezas entregadas y otras sin entregar. Pásalas todas juntas"},
		{domain.ErrAlreadyItsOwnOrder, 409, "CONFLICT", "Ya es su propio pedido; no hace falta pasarlo"},
		{domain.ErrMoveKeyMismatch, 409, "CONFLICT", "Esto ya se pasó a otro pedido"},
		{domain.ErrOrderHasPayments, 409, "CONFLICT", "Tiene pagos: hay que devolverlos primero"},
		{domain.ErrNoProducts, 409, "CONFLICT", "Este pedido ya no tiene productos: ciérralo"},
		{domain.ErrDiscountWithPayments, 409, "CONFLICT", "Ya hay pagos; el descuento se pone antes de cobrar"},
		{domain.ErrOneChargeShape, 400, "VALIDATION", "Elige una sola forma de cobrar"},
		{domain.ErrEmptySelection, 400, "VALIDATION", "Elige qué productos paga"},
		{domain.ErrTooManyPieces, 400, "VALIDATION", "No hay tantas piezas por quitar"},
		{domain.ErrMoveWithDiscount, 409, "CONFLICT", "Quita el descuento antes de pasar productos"},
		{domain.ErrMoveTargetClosed, 409, "CONFLICT", "Ese pedido ya no recibe productos"},
		{domain.ErrMoveTargetOtherShift, 409, "CONFLICT", "Ese pedido es de otro turno"},
		{domain.ErrMergeWithShipping, 409, "CONFLICT", "Ese pedido tiene envío; cóbralo o quítalo antes de juntarlo"},
		{domain.ErrMoveRefundedLine, 409, "CONFLICT", "Ese producto tiene una devolución; no se puede pasar"},
		{domain.ErrMoveLegacyLine, 409, "CONFLICT", "Ese producto es de un pedido viejo; no se puede pasar"},
		{domain.ErrOrderClosedForVoid, 409, "CONFLICT", "Ese pedido ya se cerró; no se le pueden devolver pagos"},
		// Las variantes por operación conservan el sentinel y llevan su cola.
		{domain.ErrPieceAlreadyPaidToMove, 409, "CONFLICT", "Ese producto ya se pagó; no se puede pasar"},
		{domain.ErrPieceAlreadyPaidToRemove, 409, "CONFLICT", "Ese producto ya se pagó. Primero hay que devolver el pago"},
		{domain.ErrOrderFromClosedShiftToMove, 409, "CONFLICT", "Ese pedido es de un turno cerrado; no se puede pasar"},
		// No hay fiados (2026-10-09): el cierre se niega con su propio código y sin «conflicto».
		{domain.NoOwingOrders([]domain.OwingOrder{{Number: 7, Name: "Persa", Total: decimal.RequireFromString("65")}}),
			409, "UNPAID_ORDERS", "hay pedidos entregados sin cobrar: Persa (#7) debe $65.00. Cóbralos o cancélalos antes de cerrar"},
		{domain.ErrCancelDeliveredWithPayments, 409, "CONFLICT", "Este pedido ya tiene pagos; cobra lo que falta"},
	}
	for _, c := range cases {
		t.Run(c.text, func(t *testing.T) {
			status, body := errorOf(t, c.err)
			if status != c.status || body.Code != c.code || body.Message != c.text {
				t.Fatalf("= %d %s %q, quiere %d %s %q", status, body.Code, body.Message, c.status, c.code, c.text)
			}
		})
	}
	for variant, base := range map[error]error{
		domain.ErrPieceAlreadyPaidToMove:     domain.ErrPieceAlreadyPaid,
		domain.ErrPieceAlreadyPaidToRemove:   domain.ErrPieceAlreadyPaid,
		domain.ErrOrderFromClosedShiftToMove: domain.ErrOrderFromClosedShift,
	} {
		if !errors.Is(variant, base) {
			t.Errorf("%q no es %q: quien lo compare con errors.Is no lo reconocería", variant, base)
		}
	}
}

// Los códigos de la 030 (contracts/api.md): la pantalla decide qué ofrecer por el código, no por el
// texto. ORDER_CLOSED ofrece «empezar cuenta nueva»; PLATFORM_ORDER_NO_LINES no ofrece nada;
// DRAFT_CHANGED recarga la cuenta. Un CONFLICT genérico no le diría cuál de las tres hacer.
func TestDraftErrorsHaveTheirOwnCode(t *testing.T) {
	casos := []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrDraftChanged, 409, "DRAFT_CHANGED"},
		{domain.ErrDraftDiscarded, 409, "DRAFT_DISCARDED"},
		{domain.ErrDraftAlreadySent, 409, "DRAFT_SENT"},
		{domain.ErrDraftHasOrderHeader, 422, "DRAFT_HAS_ORDER_HEADER"},
		{domain.ErrOrderClosed, 409, "ORDER_CLOSED"},
		{domain.ErrPlatformOrderNoLines, 422, "PLATFORM_ORDER_NO_LINES"},
		// Envuelto con %w por quien agrega contexto, sigue siendo el mismo código.
		{fmt.Errorf("%w (renglón 3)", domain.ErrDraftChanged), 409, "DRAFT_CHANGED"},
		{fmt.Errorf("%w (pedido 12)", domain.ErrDraftAlreadySent), 409, "DRAFT_SENT"},
	}
	for _, c := range casos {
		t.Run(c.code, func(t *testing.T) {
			w := httptest.NewRecorder()
			Error(w, c.err)
			var got struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if w.Code != c.status || got.Error.Code != c.code {
				t.Fatalf("%v → %d %s, quería %d %s", c.err, w.Code, got.Error.Code, c.status, c.code)
			}
			// El texto es para quien opera: sin el nombre del sentinel base ni palabras internas.
			for _, interna := range []string{"conflicto", "datos inválidos", "borrador", "draft", "versión"} {
				if strings.Contains(strings.ToLower(got.Error.Message), interna) {
					t.Fatalf("el mensaje %q nombra %q", got.Error.Message, interna)
				}
			}
		})
	}
}
