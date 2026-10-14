//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/cache"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// «Salud del inventario» por el ROUTER real: permiso, parámetros y forma del JSON.
func newHealthAPI(t *testing.T, perms httpapi.PermissionResolver) (http.Handler, *store.Store, func(username, role string) string) {
	t.Helper()
	st := newTestStore(t)
	jm := auth.NewManager("secreto-de-pruebas-suficientemente-largo-para-el-manager", nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT:             jm,
		InventoryHealth: app.NewInventoryHealthService(st),
		MenuCache:       cache.NewMenuCache(""),
		Broker:          realtime.NewBroker(),
		Permissions:     perms,
	})
	r := httpapi.Router(config.Config{}, jm, h, st)
	token := func(username, role string) string {
		id := makeUser(t, st, username, role)
		tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: username, Role: domain.Role(role)})
		if err != nil {
			t.Fatalf("Issue(%s): %v", username, err)
		}
		return tok
	}
	return r, st, token
}

const healthPath = "/api/v1/stock/health?from=2026-07-01&to=2026-07-31"

// Toda respuesta que promete un arreglo lo entrega aunque esté vacío: el JSON crudo, no una
// estructura de Go, porque deserializar borra la diferencia entre null y [] (AGENTS.md §1).
func TestInventoryHealthJSONNeverSendsNullArrays(t *testing.T) {
	t.Parallel()
	r, _, token := newHealthAPI(t, nil)
	w := do(t, r, http.MethodGet, healthPath, token("admin_salud_json", "admin"), nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET = %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "null") {
		t.Fatalf("una empresa vacía recibe null donde la pantalla espera una lista: %s", w.Body.String())
	}
	var v map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"unsoldDeduction", "comboChoices", "negativeStock", "noPurchases", "malformedRecipes", "pendingRecipes"} {
		if _, ok := v[k]; !ok {
			t.Errorf("falta la sección %q: %s", k, w.Body.String())
		}
	}
}

// Un parámetro mal formado se rechaza; nunca se contesta otro periodo en silencio.
func TestInventoryHealthRejectsMalformedParameters(t *testing.T) {
	t.Parallel()
	r, _, token := newHealthAPI(t, nil)
	admin := token("admin_salud_param", "admin")
	for _, q := range []string{
		"?from=2026-13-01", "?from=2026-07-31&to=2026-07-01", "?section=todo", "?section=negative_stock&limit=0",
		"?section=negative_stock&offset=-1", "?limit=abc", "?noPurchasesSince=ayer",
	} {
		if w := do(t, r, http.MethodGet, "/api/v1/stock/health"+q, admin, nil, ""); w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
			t.Errorf("GET %s = %d, quiere un rechazo: %s", q, w.Code, w.Body.String())
		}
	}
}

// Cada renglón lleva a donde se corrige: es el segundo y último toque (SC-002).
func TestInventoryHealthRowsLeadWhereTheyAreFixed(t *testing.T) {
	t.Parallel()
	r, st, token := newHealthAPI(t, nil)
	k := &healthKit{st: st, orders: app.NewOrdersService(st, clock)}
	k.cashier = makeUser(t, st, "cajero_salud_fix", "cajero")
	abrirCajaPrincipal(t, st, k.cashier)
	shot := makeProduct(t, st, "Shot fix", decimal.RequireFromString("30"), false)
	opt := opcionConTope(t, st, "Extras fix", "Extra fix", decimal.Zero, 1, shot)
	leche := makeIngredient(t, st, "Leche fix")
	cafe := makeProduct(t, st, "Café fix", decimal.RequireFromString("40"), false)
	withRecipe(t, st, "products", cafe, map[int64]string{leche: "100"})
	k.sell(t, line(shot, "1", opt), line(cafe, "1"))

	w := do(t, r, http.MethodGet, healthPath, token("admin_salud_fix", "admin"), nil, "")
	var v app.InventoryHealthView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	want := map[string]string{
		"product": "/catalogo/recetas?kind=product&open=product:" + itoa(int(shot)),
		"extra":   "/catalogo/recetas?kind=extra&open=option:" + itoa(int(opt)),
	}
	for _, it := range v.UnsoldDeduction.Items {
		if it.FixAt != want[it.Kind] {
			t.Errorf("%s %d lleva a %q, quiere %q", it.Kind, it.ID, it.FixAt, want[it.Kind])
		}
	}
	n := negativeOf(v, leche)
	if n == nil || n.FixAt != "/almacen?tab=insumos&open="+itoa(int(leche)) {
		t.Errorf("el insumo en negativo lleva a %+v, quiere su ficha en Almacén", n)
	}
	if v.PendingRecipes.Undecided.FixAt != "/catalogo/recetas?kind=product&status=pending" ||
		v.PendingRecipes.Review.FixAt != "/catalogo/recetas?kind=product&status=review" {
		t.Errorf("los pendientes llevan a %+v", v.PendingRecipes)
	}
}

// Decide el permiso, no el nombre del rol: con un resolutor que se lo da al cajero y se lo niega al
// admin, entra el cajero y no el admin. Si la ruta siguiera dentro del grupo con RequireRole, el
// cajero rebotaría aunque tuviera el permiso.
func TestInventoryHealthIsGatedByPermissionNotRole(t *testing.T) {
	t.Parallel()
	perms := func(r domain.Role) []domain.Permission {
		if r == domain.RoleCajero {
			return []domain.Permission{domain.PermInventoryHealthView}
		}
		return []domain.Permission{}
	}
	r, _, token := newHealthAPI(t, perms)
	if w := do(t, r, http.MethodGet, healthPath, token("cajero_salud_perm", "cajero"), nil, ""); w.Code != http.StatusOK {
		t.Errorf("con el permiso, el cajero recibe %d: %s", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodGet, healthPath, token("admin_salud_perm", "admin"), nil, ""); w.Code != http.StatusForbidden {
		t.Errorf("sin el permiso, el admin recibe %d", w.Code)
	}

	// Con el resolutor de hoy, el cajero no lo tiene.
	r2, _, token2 := newHealthAPI(t, nil)
	if w := do(t, r2, http.MethodGet, healthPath, token2("cajero_salud_hoy", "cajero"), nil, ""); w.Code != http.StatusForbidden {
		t.Errorf("el cajero de hoy ve la salud del inventario: %d", w.Code)
	}
	if !slices.Contains(domain.PermissionsFor(domain.RoleGerente), domain.PermInventoryHealthView) {
		t.Error("el gerente de hoy perdió el permiso")
	}
}
