package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// La edición de un insumo son dos peticiones. Si la ficha ya se guardó y la segunda no termina bien,
// el agente tiene que saberlo: con el «error interno; puede que no se haya aplicado» de siempre le
// diría a la persona que nada cambió cuando el nombre ya cambió (revisión de código de la 076, M1).
func TestIngredientUpdateSaysTheSheetWasSavedWhenTheAltNamesFail(t *testing.T) {
	name := "Jamón"
	for _, c := range []struct {
		caso      string
		patchBody string
		putStatus int
	}{
		{"el PUT de otros nombres revienta", `{"updatedAt":"2026-11-11T10:00:00Z"}`, http.StatusInternalServerError},
		{"el PATCH no devuelve versión legible", `{}`, http.StatusOK},
	} {
		t.Run(c.caso, func(t *testing.T) {
			puts := 0
			mux := chi.NewRouter()
			mux.Get(pArticle, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"id":749,"updatedAt":"2026-11-11T09:00:00Z"}`))
			})
			mux.Patch(pArticle, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(c.patchBody)) })
			mux.Put(pArticleAltNames, func(w http.ResponseWriter, _ *http.Request) {
				puts++
				w.WriteHeader(c.putStatus)
				_, _ = w.Write([]byte(`{"error":{"code":"INTERNAL","message":"boom"}}`))
			})
			s := &mcpSession{mux: mux, outer: httptest.NewRequest(http.MethodPost, "/mcp", nil),
				principal: app.APIPrincipal{TokenID: 1, TokenName: "x"}}
			alt := []string{"Jamón de pierna"}
			status, resp, err := ingredientUpdateRun(s, context.Background(), ingredientUpdateArgs{IngredientID: 749, Name: &name, AltNames: &alt})
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if status != http.StatusConflict {
				t.Fatalf("estado %d: quería 409, para que apply libere la clave y diga lo que pasó", status)
			}
			var env errorEnvelope
			_ = json.Unmarshal(resp, &env)
			if !strings.Contains(env.Error.Message, "La ficha ya quedó cambiada") || strings.Contains(env.Error.Message, "boom") {
				t.Fatalf("mensaje: %q", env.Error.Message)
			}
			if c.patchBody == `{}` && puts != 0 {
				t.Fatal("mandó los otros nombres con una versión vacía")
			}
		})
	}
}

// La vista previa hace una lectura por cada otro nombre nuevo y el tope de la lista solo lo aplica la
// ruta al guardar: sin cota aquí, una llamada con 260 mil nombres eran 260 mil lecturas contando como
// una sola contra el límite del token (auditoría de seguridad de la 076, M1).
func TestIngredientUpdateCapsAltNamesBeforeDispatching(t *testing.T) {
	many := make([]string, domain.MaxAltNames+1)
	for i := range many {
		many[i] = "Nombre " + string(rune('a'+i))
	}
	if err := (ingredientUpdateArgs{IngredientID: 1, AltNames: &many}).check(); err == nil {
		t.Fatalf("%d otros nombres pasaron la revisión de argumentos", len(many))
	}
	ok := many[:domain.MaxAltNames]
	if err := (ingredientUpdateArgs{IngredientID: 1, AltNames: &ok}).check(); err != nil {
		t.Fatalf("%d otros nombres se rechazaron: %v", len(ok), err)
	}
}
