//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Los defectos 1 y 3 del #289 (spec 090), contra la API completa servida como gatobobah_app.

func adminJWT(t *testing.T, w mcpWorld) string {
	t.Helper()
	tok, err := w.jm.Issue(domain.User{ID: w.admin, CompanyID: w.company, Name: "admin-mcp", Role: domain.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

type optionFields struct {
	name   string
	price  decimal.Decimal
	max    int
	active bool
}

func optionNowRow(t *testing.T, w mcpWorld, id int64) optionFields {
	t.Helper()
	var o optionFields
	if err := w.owner.Pool.QueryRow(context.Background(), `select name, price_delta, max_per_line, is_active
		from modifier_options where id = $1`, id).Scan(&o.name, &o.price, &o.max, &o.active); err != nil {
		t.Fatal(err)
	}
	return o
}

// groupWithOption deja un grupo (activo o no) con una opción de $15 y tope 2.
func groupWithOption(t *testing.T, w mcpWorld, group string, active bool) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	var g, o int64
	if err := w.owner.Pool.QueryRow(ctx, `insert into modifier_groups (company_id, name, is_active) values ($1, $2, $3) returning id`,
		w.company, group, active).Scan(&g); err != nil {
		t.Fatal(err)
	}
	if err := w.owner.Pool.QueryRow(ctx, `insert into modifier_options (company_id, group_id, name, price_delta, max_per_line)
		values ($1, $2, $3, 15, 2) returning id`, w.company, g, "Opción de "+group).Scan(&o); err != nil {
		t.Fatal(err)
	}
	return g, o
}

// El 15-nov un PATCH con solo {name} dejó dos opciones de pago en $0: el handler rellenaba lo que no
// venía con 0 y 1. La pantalla manda siempre los tres campos y por eso nunca se vio en ella.
func TestOptionPatchKeepsWhatItDoesNotSend(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	jwt := adminJWT(t, w)
	_, opt := groupWithOption(t, w, "Pago parcial", true)
	patch := func(body string) {
		t.Helper()
		rec := do(t, w.router, http.MethodPatch, "/api/v1/admin/modifier-options/"+itoa(int(opt)), jwt, []byte(body), "application/json")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("PATCH %s: %d %s", body, rec.Code, rec.Body.String())
		}
	}

	patch(`{"name":"Pago con tarjeta"}`)
	if got := optionNowRow(t, w, opt); got.name != "Pago con tarjeta" || !got.price.Equal(decimal.NewFromInt(15)) || got.max != 2 {
		t.Fatalf("solo el nombre: %+v (el precio debía seguir en 15 y el tope en 2)", got)
	}
	patch(`{"priceDelta":"9.50"}`)
	if got := optionNowRow(t, w, opt); got.name != "Pago con tarjeta" || !got.price.Equal(decimal.RequireFromString("9.50")) || got.max != 2 {
		t.Fatalf("solo el precio: %+v (un priceDelta sin name no se puede ignorar)", got)
	}
	patch(`{"maxPerLine":3}`)
	if got := optionNowRow(t, w, opt); !got.price.Equal(decimal.RequireFromString("9.50")) || got.max != 3 {
		t.Fatalf("solo el tope: %+v", got)
	}
	patch(`{"name":"Pago","priceDelta":"0","maxPerLine":1}`)
	if got := optionNowRow(t, w, opt); got.name != "Pago" || !got.price.IsZero() || got.max != 1 {
		t.Fatalf("los tres, como la pantalla: %+v", got)
	}
	for _, bad := range []string{`{"name":""}`, `{"maxPerLine":0}`} {
		rec := do(t, w.router, http.MethodPatch, "/api/v1/admin/modifier-options/"+itoa(int(opt)), jwt, []byte(bad), "application/json")
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%s: %d, esperaba rechazo", bad, rec.Code)
		}
	}
}

// La opción de un grupo inactivo existe: la lectura por id la encuentra y dice que su grupo está
// apagado. Es la mitad del defecto 2 (la otra es la herramienta, en mcp_090_test.go).
func TestOptionByIDSeesInactiveGroups(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	jwt := adminJWT(t, w)
	_, opt := groupWithOption(t, w, "Grupo apagado", false)
	rec := do(t, w.router, http.MethodGet, "/api/v1/admin/modifier-options/"+itoa(int(opt)), jwt, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET por id: %d %s", rec.Code, rec.Body.String())
	}
	var v struct {
		ID          int64  `json:"id"`
		GroupActive bool   `json:"groupActive"`
		MaxPerLine  int    `json:"maxPerLine"`
		PriceDelta  string `json:"priceDelta"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil || v.ID != opt || v.GroupActive || v.MaxPerLine != 2 || v.PriceDelta != "15" {
		t.Fatalf("opción leída: %+v (%v)", v, err)
	}
	inTheThreeCases(t, w.company, w.other, func(t *testing.T, st *store.Store, ctx context.Context) {
		if _, err := app.NewAdminService(st).GetOption(ctx, opt); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("la opción de otra empresa se leyó: %v", err)
		}
	})
}

// El defecto 3: un producto con un grupo apagado ligado se veía igual que los demás en la lista.
func TestProductListCountsInactiveGroups(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	jwt := adminJWT(t, w)
	prod := makeProduct(t, w.owner, "Té con grupo apagado", decimal.NewFromInt(60), false)
	on, _ := groupWithOption(t, w, "Toppings encendido", true)
	off, _ := groupWithOption(t, w, "Toppings apagado", false)
	for _, g := range []int64{on, off} {
		if _, err := w.owner.Pool.Exec(ctx, `insert into product_modifier_groups (company_id, product_id, group_id) values ($1, $2, $3)`,
			w.company, prod, g); err != nil {
			t.Fatal(err)
		}
	}
	rec := do(t, w.router, http.MethodGet, "/api/v1/admin/products/?search=grupo%20apagado&limit=0", jwt, nil, "")
	var page struct {
		Items []struct {
			ID                 int64 `json:"id"`
			GroupCount         int   `json:"groupCount"`
			InactiveGroupCount int   `json:"inactiveGroupCount"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || len(page.Items) != 1 {
		t.Fatalf("lista: %d %s", rec.Code, rec.Body.String())
	}
	if p := page.Items[0]; p.ID != prod || p.GroupCount != 1 || p.InactiveGroupCount != 1 {
		t.Fatalf("producto: %+v (1 activo y 1 apagado)", p)
	}
}

// El aviso de archivar un grupo dice en cuántos productos deja de ofrecerse: solo cuentan los activos
// (un producto archivado no se ofrece de todos modos, y contarlo haría mentir al aviso).
func TestGroupListCountsActiveProducts(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	jwt := adminJWT(t, w)
	g, _ := groupWithOption(t, w, "Jarabes por contar", true)
	on := makeProduct(t, w.owner, "Té activo con jarabe", decimal.NewFromInt(50), false)
	off := makeProduct(t, w.owner, "Té archivado con jarabe", decimal.NewFromInt(50), false)
	if _, err := w.owner.Pool.Exec(ctx, `update products set is_active = false where id = $1`, off); err != nil {
		t.Fatal(err)
	}
	for _, p := range []int64{on, off} {
		if _, err := w.owner.Pool.Exec(ctx, `insert into product_modifier_groups (company_id, product_id, group_id) values ($1, $2, $3)`,
			w.company, p, g); err != nil {
			t.Fatal(err)
		}
	}
	rec := do(t, w.router, http.MethodGet, "/api/v1/admin/groups/?search=Jarabes%20por%20contar", jwt, nil, "")
	var page struct {
		Items []struct {
			ProductCount       int `json:"productCount"`
			ActiveProductCount int `json:"activeProductCount"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || len(page.Items) != 1 {
		t.Fatalf("grupos: %d %s", rec.Code, rec.Body.String())
	}
	if g := page.Items[0]; g.ProductCount != 2 || g.ActiveProductCount != 1 {
		t.Fatalf("grupo: %+v (2 ligados, 1 activo)", g)
	}
}

// El hermano que no se movió (revisión de la 090): editar ya topaba el tope por renglón a int16, crear
// no, y 70000 se guardaba como 4464 sin avisar. Un nombre repetido en el grupo tiene que decir eso, no
// «conflicto».
func TestOptionCreateAndPatchShareTheirLimits(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	jwt := adminJWT(t, w)
	g, opt := groupWithOption(t, w, "Topes compartidos", true)
	create := func(body string) (int, string) {
		t.Helper()
		rec := do(t, w.router, http.MethodPost, "/api/v1/admin/groups/"+itoa(int(g))+"/options", jwt, []byte(body), "application/json")
		return rec.Code, rec.Body.String()
	}
	for _, body := range []string{`{"name":"Grande","maxPerLine":70000}`, `{"name":"Caro","priceDelta":"99999999999"}`} {
		if code, b := create(body); code != http.StatusBadRequest {
			t.Errorf("crear %s: %d %s", body, code, b)
		}
	}
	for _, body := range []string{`{"maxPerLine":70000}`, `{"priceDelta":"99999999999"}`} {
		if rec := do(t, w.router, http.MethodPatch, "/api/v1/admin/modifier-options/"+itoa(int(opt)), jwt, []byte(body), "application/json"); rec.Code != http.StatusBadRequest {
			t.Errorf("editar %s: %d %s", body, rec.Code, rec.Body.String())
		}
	}
	if code, b := create(`{"name":"Opción de Topes compartidos"}`); code != http.StatusConflict || !strings.Contains(b, "ya hay un extra con ese nombre") {
		t.Errorf("nombre repetido: %d %s", code, b)
	}
	// Un máximo de 65536 daba la vuelta a 0 (grupo donde no se puede elegir nada) o tronaba un check (500).
	for _, body := range []string{`{"name":"Vuelta","defaultMin":0,"defaultMax":65536}`, `{"name":"Vuelta 2","defaultMin":1,"defaultMax":65536}`} {
		if rec := do(t, w.router, http.MethodPost, "/api/v1/admin/groups/", jwt, []byte(body), "application/json"); rec.Code != http.StatusBadRequest {
			t.Errorf("grupo %s: %d %s", body, rec.Code, rec.Body.String())
		}
	}
	prod := makeProduct(t, w.owner, "Producto con topes", decimal.NewFromInt(10), false)
	for _, body := range []string{`{"groupId":` + itoa(int(g)) + `,"override":true,"minSelect":0,"maxSelect":65536}`,
		`{"groupId":` + itoa(int(g)) + `,"position":4294967297}`} {
		if rec := do(t, w.router, http.MethodPost, "/api/v1/admin/products/"+itoa(int(prod))+"/groups", jwt, []byte(body), "application/json"); rec.Code != http.StatusBadRequest {
			t.Errorf("liga %s: %d %s", body, rec.Code, rec.Body.String())
		}
	}
}
