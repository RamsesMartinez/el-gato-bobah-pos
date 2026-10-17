//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// El tope de aprobación de la merma (spec 039, FR-010): un monto mayor que cero o vacío. Un valor
// mal escrito se rechaza; caer a «sin tope» en silencio dejaría pasar mermas que debían esperar.
func TestWasteThresholdHTTP(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Backoffice: app.NewBackofficeService(st, clock), MenuCache: cache.NewMenuCache(""), Broker: realtime.NewBroker(),
	})
	r := httpapi.Router(config.Config{}, jm, h, st)
	id := makeUser(t, st, "gerente_tope_http", "gerente")
	tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: "g", Role: domain.RoleGerente})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		body string
		want int
	}{
		{`{"threshold":"100"}`, http.StatusNoContent},
		{`{"threshold":null}`, http.StatusNoContent},
		{`{"threshold":"0"}`, http.StatusBadRequest},
		{`{"threshold":"-5"}`, http.StatusBadRequest},
		{`{"threshold":"cien"}`, http.StatusBadRequest},
		{`{"threshold":"1e20"}`, http.StatusBadRequest},
	} {
		if w := do(t, r, http.MethodPut, "/api/v1/settings/waste-threshold", tok, []byte(c.body), "application/json"); w.Code != c.want {
			t.Errorf("%s = %d %s; quiere %d", c.body, w.Code, w.Body.String(), c.want)
		}
	}
}
