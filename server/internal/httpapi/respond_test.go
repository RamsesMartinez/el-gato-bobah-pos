package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

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
