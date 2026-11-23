package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"uuid"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
)

// La cuenta en captura se nombra con un UUID, y hasta la 090 el despachador solo aceptaba enteros en
// el camino: ningún texto del agente podía formar parte de la ruta. El id de cuenta sigue sin ser
// texto: se lee con uuid.Parse y viaja como uuid.UUID, así que el camino solo lleva hex y guiones.
func TestDispatcherRejectsNonUUIDDraftID(t *testing.T) {
	for _, bad := range []string{"", "../../me/pin", "%2F..%2Fme", "1", "x0a1b2c3d-0000-0000-0000-000000000000",
		"0a1b2c3d-0000-0000-0000-000000000000/../x", " 0a1b2c3d-0000-0000-0000-000000000000"} {
		if _, err := parseDraftID(bad); err == nil {
			t.Errorf("%q se aceptó como id de cuenta", bad)
		}
	}
	id := uuid.MustParse("0a1b2c3d-4e5f-4a6b-8c7d-0e1f2a3b4c5d")
	got, err := parseDraftID(id.String())
	if err != nil || got != id {
		t.Fatalf("un UUID válido no se leyó: %v", err)
	}

	var seen string
	mux := chi.NewRouter()
	mux.Post(pDraftSend, func(w http.ResponseWriter, r *http.Request) {
		seen = chi.URLParam(r, "id")
		w.WriteHeader(http.StatusOK)
	})
	s := &mcpSession{mux: mux, outer: httptest.NewRequest(http.MethodPost, "/mcp", nil),
		principal: app.APIPrincipal{TokenID: 1, TokenName: "x"}}
	status, _, err := s.dispatchDraft(context.Background(), "order_draft_send", http.MethodPost, pDraftSend, id, nil)
	if err != nil || status != http.StatusOK || seen != id.String() {
		t.Fatalf("despacho con UUID: status %d, err %v, id %q", status, err, seen)
	}
	if _, _, err := s.dispatchDraft(context.Background(), "order_pay", http.MethodPost, pDraftSend, id, nil); err == nil {
		t.Fatal("una herramienta despachó la ruta de otra")
	}
}
