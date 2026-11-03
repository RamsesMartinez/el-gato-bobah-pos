//go:build integration

package integration

import (
	"encoding/json"
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

// Editar un gasto por el router real (spec 053): permisos por capacidad, `preview` que no se
// adivina, arreglos que salen como [] y los códigos que la pantalla necesita.
func TestEditExpenseHTTP(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	// Un rol propio que edita gastos pero no recibe mercancía: el cajero, para esta prueba.
	resolver := func(r domain.Role) []domain.Permission {
		if r == domain.RoleCajero {
			return []domain.Permission{domain.PermExpensesEdit}
		}
		return domain.PermissionsFor(r)
	}
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Backoffice: app.NewBackofficeService(k.st, nil), Settings: app.NewSettingsService(k.st, "pepper-de-prueba"),
		MenuCache: cache.NewMenuCache(""), Broker: realtime.NewBroker(), Permissions: resolver,
	})
	r := httpapi.Router(config.Config{}, jm, h, k.st)
	token := func(username, role string) string {
		id := makeUser(t, k.st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	admin, gerente, editor := token("admin_053", "admin"), token("gerente_053", "gerente"), token("editor_053", "cajero")
	id := k.paidWithoutGoods(t, "67.50", k.day(-2))
	base := "/api/v1/expenses/" + strconv.FormatInt(id, 10)
	ct := "application/json"
	add := `{"add":[{"itemType":"ingrediente","ingredientId":` + strconv.FormatInt(k.tapas, 10) +
		`,"description":"Tapas","quantity":"50","unitId":` + strconv.Itoa(int(k.pieza)) + `,"amount":"67.50"}]}`

	// El gerente recibe pero no edita (por omisión solo admin, #167).
	if w := do(t, r, http.MethodPost, base+"/edit?preview=1", gerente, []byte(add), ct); w.Code != http.StatusForbidden {
		t.Fatalf("gerente editando = %d, quiere 403", w.Code)
	}
	// `preview=true` no es `1`: se rechaza en vez de confirmar en silencio.
	if w := do(t, r, http.MethodPost, base+"/edit?preview=true", admin, []byte(add), ct); w.Code != http.StatusBadRequest {
		t.Fatalf("preview=true = %d, quiere 400", w.Code)
	}
	// Agregar algo que «ya llegó» exige también recibir.
	arrived := strings.Replace(add, `"amount":"67.50"`, `"amount":"67.50","arrivedOn":"`+k.day(0)+`"`, 1)
	if w := do(t, r, http.MethodPost, base+"/edit?preview=1", editor, []byte(arrived), ct); w.Code != http.StatusForbidden {
		t.Fatalf("«ya llegó» sin permiso de recibir = %d, quiere 403", w.Code)
	}
	w := do(t, r, http.MethodPost, base+"/edit?preview=1", editor, []byte(add), ct)
	if w.Code != http.StatusOK {
		t.Fatalf("vista previa = %d %s", w.Code, w.Body.String())
	}
	// Todo arreglo sale como [] aunque esté vacío (AGENTS.md): la pantalla hace .map sobre ellos.
	for _, f := range []string{`"header":[]`, `"inventory":[]`, `"costs":[]`, `"decisions":[]`} {
		if !strings.Contains(w.Body.String(), f) {
			t.Fatalf("la vista previa no trae %s: %s", f, w.Body.String())
		}
	}
	var res struct {
		Preview struct {
			Fingerprint string `json:"fingerprint"`
		} `json:"preview"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	stale := strings.Replace(add, `]}`, `],"fingerprint":"otra"}`, 1)
	if w := do(t, r, http.MethodPost, base+"/edit", editor, []byte(stale), ct); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "EXPENSE_EDIT_STALE") {
		t.Fatalf("huella vieja = %d %s, quiere 409 EXPENSE_EDIT_STALE", w.Code, w.Body.String())
	}
	ok := strings.Replace(add, `]}`, `],"fingerprint":"`+res.Preview.Fingerprint+`"}`, 1)
	if w := do(t, r, http.MethodPost, base+"/edit", editor, []byte(ok), ct); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"deliveries":[]`) {
		t.Fatalf("confirmar = %d %s", w.Code, w.Body.String())
	}
	// El gerente sí recibe.
	item := k.items(t, id)["Tapas"]
	deliver := `{"lines":[{"itemId":` + strconv.FormatInt(item, 10) + `,"qty":"50"}]}`
	if w := do(t, r, http.MethodPost, base+"/deliveries?preview=1", gerente, []byte(deliver), ct); w.Code != http.StatusOK {
		t.Fatalf("gerente recibiendo = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodPost, base+"/deliveries?preview=1", editor, []byte(deliver), ct); w.Code != http.StatusForbidden {
		t.Fatalf("sin permiso de recibir = %d, quiere 403", w.Code)
	}
	// Falta decidir: 400 con su código.
	raise := strings.Replace(add, `"amount":"67.50"`, `"amount":"90"`, 1)
	w = do(t, r, http.MethodPost, base+"/edit?preview=1", admin, []byte(raise), ct)
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	raise = strings.Replace(raise, `]}`, `],"fingerprint":"`+res.Preview.Fingerprint+`"}`, 1)
	if w := do(t, r, http.MethodPost, base+"/edit", admin, []byte(raise), ct); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "DECISION_REQUIRED") {
		t.Fatalf("sin decidir = %d %s, quiere 400 DECISION_REQUIRED", w.Code, w.Body.String())
	}
}
