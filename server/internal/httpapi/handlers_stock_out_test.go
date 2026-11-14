package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// «Se acabó» no acepta cantidad (decisión del dueño: deja la existencia en 0). Un cliente que manda
// `quantity` recibe 400 antes de tocar nada, y una clave que no es uuid también.
func TestReportOutRejectsAQuantityAndABadKey(t *testing.T) {
	h := &Handlers{}
	for name, body := range map[string]string{
		"con cantidad":         `{"itemType":"ingrediente","id":1,"clientKey":"8f14e45f-ceea-467a-9575-6b1b2a9c6d1e","quantity":"3"}`,
		"clave que no es uuid": `{"itemType":"ingrediente","id":1,"clientKey":"doble-toque"}`,
	} {
		rec := httptest.NewRecorder()
		h.ReportOut(rec, httptest.NewRequest(http.MethodPost, "/stock/out-reports", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
}
