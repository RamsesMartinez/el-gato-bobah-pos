package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// #335: una referencia que no existe en la empresa (la FK compuesta la frenó) es 422 con su código y
// el sustantivo, no 500 «Error interno» con el detalle solo en el correo.
func TestMissingReferenceIsA422ThatNamesIt(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, fmt.Errorf("ajustar: %w", domain.ReferenceNotFoundWrapping("products",
		&pgconn.PgError{Code: "23503", Detail: `Key is not present in table "products".`})))
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d", w.Code)
	}
	var env errorEnvelope
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if env.Error.Code != "REFERENCE_NOT_FOUND" || !strings.Contains(env.Error.Message, "el producto") ||
		strings.Contains(env.Error.Message, "products") {
		t.Fatalf("respuesta: %+v", env.Error)
	}
}

// #335: un 500 dice la causa por categoría, para que quien llamó sepa qué pasó, sin el texto del error
// (la consulta, los valores) que sigue yendo solo a la bitácora y al correo.
func TestInternalErrorCarriesACauseButNotItsText(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, fmt.Errorf("guardar: %w", &pgconn.PgError{Code: "23514", Message: "new row violates check constraint \"secreto\""}))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", w.Code)
	}
	var env errorEnvelope
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if !strings.Contains(env.Error.Cause, "regla de los datos") {
		t.Fatalf("sin causa: %+v", env.Error)
	}
	if strings.Contains(w.Body.String(), "secreto") {
		t.Fatalf("el texto interno llegó en la respuesta: %s", w.Body.String())
	}
}
