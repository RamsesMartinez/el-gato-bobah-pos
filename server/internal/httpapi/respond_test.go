package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

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
// la base (EGB01) y llega crudo de pgx; si la traducción no estuviera en Error, la pantalla diría
// «algo salió mal» y el mensaje de Postgres —con el id de la empresa— viajaría al log como caída.
func TestAmbiguousBranchFromTheDatabaseIsA409WithItsOwnCode(t *testing.T) {
	rec := httptest.NewRecorder()
	Error(rec, fmt.Errorf("crear pedido: %w", &pgconn.PgError{
		Code: "EGB01", Message: "branch_ambiguous: la empresa 2 tiene 2 sucursales activas",
	}))
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
