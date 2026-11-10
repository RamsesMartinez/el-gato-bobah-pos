package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
)

// El despachador solo sirve las rutas de la lista blanca de cada herramienta. Sin esto, el
// principal interno pasa RequireAuth y alcanza rutas que solo piden sesión: /me/pin no pide el PIN
// actual, y un token de lectura podría fijar el PIN del admin y desbloquear la tableta.
func TestDispatcherRefusesRoutesOutsideCatalog(t *testing.T) {
	hits := map[string]int{}
	mux := chi.NewRouter()
	for _, p := range []string{"/api/v1/me/pin", "/api/v1/me/password", "/api/v1/users/", "/api/v1/roles",
		"/api/v1/integrations/tokens", "/api/v1/auth/pin-switch", "/api/v1/orders/{id}/pay", "/api/v1/expenses/"} {
		mux.Post(p, func(w http.ResponseWriter, _ *http.Request) { hits[p]++; w.WriteHeader(http.StatusNoContent) })
	}
	s := &mcpSession{mux: mux, outer: httptest.NewRequest(http.MethodPost, "/mcp", nil),
		principal: app.APIPrincipal{TokenID: 1, TokenName: "x"}}
	for _, p := range []string{"/api/v1/me/pin", "/api/v1/me/password", "/api/v1/users/", "/api/v1/roles",
		"/api/v1/integrations/tokens", "/api/v1/auth/pin-switch", "/api/v1/orders/{id}/pay"} {
		_, _, err := s.dispatch(context.Background(), "expense_create", http.MethodPost, p, map[string]int64{"id": 1}, nil, map[string]any{})
		if !errors.Is(err, errDispatchDenied) {
			t.Errorf("%s: el despachador no lo rechazó (%v)", p, err)
		}
		if hits[p] != 0 {
			t.Errorf("%s: llegó al handler", p)
		}
	}
	// Una herramienta tampoco puede usar la ruta de otra.
	if _, _, err := s.dispatch(context.Background(), "expenses_list", http.MethodPost, "/api/v1/expenses/", nil, nil, map[string]any{}); !errors.Is(err, errDispatchDenied) {
		t.Errorf("una lectura despachó la escritura de otra herramienta: %v", err)
	}
	status, _, err := s.dispatch(context.Background(), "expense_create", http.MethodPost, "/api/v1/expenses/", nil, nil, map[string]any{})
	if err != nil || status != http.StatusNoContent || hits["/api/v1/expenses/"] != 1 {
		t.Fatalf("la ruta propia no se despachó: %d %v", status, err)
	}
}

// Un parámetro de ruta que falta deja la llave en el camino y no se despacha.
func TestDispatcherRefusesMissingParams(t *testing.T) {
	mux := chi.NewRouter()
	mux.Get("/api/v1/expenses/{id}", func(w http.ResponseWriter, _ *http.Request) { t.Fatal("llegó al handler") })
	s := &mcpSession{mux: mux, outer: httptest.NewRequest(http.MethodPost, "/mcp", nil)}
	if _, _, err := s.dispatch(context.Background(), "expense_get", http.MethodGet, pExpense, nil, nil, nil); !errors.Is(err, errDispatchDenied) {
		t.Fatalf("sin id: %v", err)
	}
}
