//go:build integration

package integration

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// Revisión de seguridad de la 096: quien solo cuenta capturaba cifras absurdas, leía en la vista
// previa del cierre lo que el sistema tenía y recapturaba esa cifra para que el faltante quedara en
// cero. El conteo es a ciegas también ahí: sin inventory.configure, ni la vista previa ni el cierre
// traen system ni delta. Quien configura los sigue viendo.
func TestTheCloseSheetIsBlindForWhoOnlyCounts(t *testing.T) {
	t.Parallel()
	k := newCountKit(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Admin: app.NewAdminService(k.st), Backoffice: app.NewBackofficeService(k.st, nil),
		MenuCache: cache.NewMenuCache(""), Broker: realtime.NewBroker()})
	r := httpapi.Router(config.Config{}, jm, h, k.st)
	token := func(username, role string) string {
		id := makeUser(t, k.st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	cajero, gerente := token("cajero_ciegas", "cajero"), token("gerente_ciegas", "gerente")
	// La empresa le da al cajero contar, y nada más del almacén: el rol que esta regla protege.
	if _, err := k.st.Pool.Exec(k.ctx, `
		insert into role_permission_overrides (company_id, role_id, permission_id, granted)
		select $1, r.id, p.id, true from roles r, permissions p
		 where r.company_id = $1 and r.system_role_id = 3 and p.code = 'inventory.count'`, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	c, err := k.svc.OpenCount(k.ctx, domain.WarehouseSel{}, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if err := k.svc.CaptureCount(k.ctx, c.ID, k.leche, dd("999999"), k.user); err != nil {
		t.Fatal(err)
	}
	preview := "/api/v1/stock/counts/" + strconv.FormatInt(c.ID, 10) + "/preview"

	w := do(t, r, http.MethodGet, preview, cajero, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("vista previa de quien cuenta = %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, `"system"`) || strings.Contains(body, `"delta"`) || strings.Contains(body, "500") {
		t.Errorf("quien solo cuenta ve lo que el sistema tenía (500 ml) en la vista previa: %s", body)
	}
	if !strings.Contains(body, `"blind":true`) || !strings.Contains(body, `"counted":"999999"`) {
		t.Errorf("la vista a ciegas debe decir blind y traer lo contado: %s", body)
	}
	if w := do(t, r, http.MethodGet, preview, gerente, nil, ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"system":"500"`) {
		t.Errorf("quien configura = %d %s; quiere ver lo que el sistema tenía", w.Code, w.Body.String())
	}
	w = do(t, r, http.MethodPost, "/api/v1/stock/counts/"+strconv.FormatInt(c.ID, 10)+"/close", cajero, nil, "")
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), `"system"`) || strings.Contains(w.Body.String(), `"delta"`) {
		t.Errorf("el cierre de quien cuenta = %d %s; quiere 200 sin system ni delta", w.Code, w.Body.String())
	}
}
