//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

const testMenuBase = "https://menu.test"

// currentMenuName lee el nombre vigente de una empresa como owner.
func currentMenuName(t *testing.T, st *store.Store, company int64) string {
	t.Helper()
	var n string
	if err := st.Pool.QueryRow(context.Background(),
		`select name from public_menu_names where company_id = $1 and retired_at is null`, company).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// jsonKeys junta, por ruta, todas las llaves del JSON crudo: lo que de verdad sale por la puerta.
func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var walk func(prefix string, x any)
	walk = func(prefix string, x any) {
		switch y := x.(type) {
		case map[string]any:
			for k, v := range y {
				seen[prefix+k] = true
				walk(prefix+k+".", v)
			}
		case []any:
			for _, v := range y {
				walk(prefix+"[].", v)
			}
		}
	}
	walk("", doc)
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// LO QUE SALE POR LA PUERTA PÚBLICA (spec 097, FR-019 y SC-005). Sin sesión, cualquiera puede pedir
// el menú de un negocio: lo que trae se mide sobre el JSON crudo contra una lista blanca, para que un
// campo nuevo de la oferta web (un costo, una existencia) no se cuele sin que alguien lo decida.
func TestThePublicMenuExposesOnlyThePublicFields(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	admin, actx := adminAsApp(t, st, defaultCompanyID, "pm-fields")
	svc := app.NewPublicMenuService(appRoleStore(t), testMenuBase)
	ctx := context.Background()

	plain := makeProduct(t, st, "Agua natural", dec("20"), false)
	priced := makeProduct(t, st, "Agua de coco", dec("40"), false)
	hidden := makeProduct(t, st, "Solo mostrador", dec("30"), false)
	soldOut := makeProduct(t, st, "Lata agotada", dec("25"), true)
	if err := updateWith(t, actx, admin, st, priced, &app.SalesInput{Counter: true, Web: true, WebPrice: ptr(dec("45")), AllowOversell: true}); err != nil {
		t.Fatal(err)
	}
	if err := updateWith(t, actx, admin, st, hidden, &app.SalesInput{Counter: true, Web: false, AllowOversell: true}); err != nil {
		t.Fatal(err)
	}
	setOversell(t, st, soldOut, false)
	setOwnStock(t, st, soldOut, "0")
	// Una subcategoría con su producto: el menú la pinta dentro de su principal.
	var parent, child int64
	if err := st.Pool.QueryRow(ctx, `select category_id from products where id = $1`, plain).Scan(&parent); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into categories (company_id, name, parent_id, food_family) values ($1, 'Con gas', $2, 'soda') returning id`,
		defaultCompanyID, parent).Scan(&child); err != nil {
		t.Fatal(err)
	}
	inChild := makeProduct(t, st, "Agua mineral", dec("22"), false)
	if _, err := st.Pool.Exec(ctx, `update products set category_id = $1 where id = $2`, child, inChild); err != nil {
		t.Fatal(err)
	}

	name := currentMenuName(t, st, defaultCompanyID)
	menu, moved, err := svc.Menu(ctx, name)
	if err != nil || moved != nil || menu == nil {
		t.Fatalf("el menú vigente: %v %v %v", menu, moved, err)
	}

	allowed := map[string]bool{
		"kind": true, "name": true, "qrPath": true, "business": true, "business.name": true, "brand": true, "brand.palette": true,
		"template": true, "categories": true,
		"categories.[].id": true, "categories.[].name": true, "categories.[].family": true, "categories.[].imageUrl": true,
		"categories.[].products": true, "categories.[].subcategories": true,
		"categories.[].subcategories.[].id": true, "categories.[].subcategories.[].name": true,
		"categories.[].subcategories.[].family": true, "categories.[].subcategories.[].products": true,
	}
	for _, p := range []string{"categories.[].products.[].", "categories.[].subcategories.[].products.[]."} {
		for _, k := range []string{"id", "name", "description", "price", "imageUrl", "soldOut"} {
			allowed[p+k] = true
		}
	}
	for _, k := range jsonKeys(t, menu) {
		if !allowed[k] {
			t.Errorf("la respuesta pública trae %q, que no está en la lista de FR-019", k)
		}
	}

	byID := map[int64]app.PublicProduct{}
	var sawChild bool
	for _, c := range menu.Categories {
		if c.Products == nil || c.Subcategories == nil {
			t.Fatal("products y subcategories nunca son null: la plantilla los recorre sin preguntar")
		}
		for _, p := range c.Products {
			byID[p.ID] = p
		}
		for _, s := range c.Subcategories {
			if s.ID == child && c.ID == parent && len(s.Products) == 1 && s.Products[0].ID == inChild {
				sawChild = true
			}
			for _, p := range s.Products {
				byID[p.ID] = p
			}
		}
	}
	if !sawChild {
		t.Fatal("la subcategoría no salió dentro de su principal")
	}
	if _, ok := byID[hidden]; ok {
		t.Fatal("un producto apagado para la web salió en el menú público")
	}
	if p := byID[priced]; !p.Price.Equal(dec("45")) {
		t.Fatalf("con precio web el menú debe decir 45, dijo %s", p.Price)
	}
	if p := byID[plain]; !p.Price.Equal(dec("20")) || p.SoldOut {
		t.Fatalf("sin precio web hereda el de mostrador: %+v", p)
	}
	if p, ok := byID[soldOut]; !ok || !p.SoldOut {
		t.Fatalf("un agotado sale marcado, no se oculta: %+v", p)
	}
	raw, _ := json.Marshal(menu)
	for _, leak := range []string{"bobah-pruebas", "gatobobah", "current_cost", "allowOversell", "onHand"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("la respuesta pública contiene %q", leak)
		}
	}
}

// UNA EMPRESA INACTIVA, UN NOMBRE QUE NO EXISTE Y UNA FORMA IMPOSIBLE SE VEN IGUAL: quien recorre
// nombres no aprende qué negocios hubo.
func TestThePublicMenuNotFoundIsIndistinguishable(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	svc := app.NewPublicMenuService(appRoleStore(t), testMenuBase)
	ctx := context.Background()
	other := makeCompany(t, st, "pm-inactiva")
	name := currentMenuName(t, st, other)
	if _, err := st.Pool.Exec(ctx, `update companies set is_active = false where id = $1`, other); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{name, "no-existe-nadie", "ab", "Con-Mayúsculas"} {
		if _, _, err := svc.Menu(ctx, n); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%q: %v, quiero ErrNotFound", n, err)
		}
	}
	if _, err := svc.ByToken(ctx, "zzzzzzzzzzzz"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("token con forma imposible: %v", err)
	}
}

// EL MENÚ DE UN NEGOCIO NUNCA TRAE NADA DE OTRO (constitución IV, tres casos). El nombre resuelve sin
// sesión, pero lo demás se lee con la empresa del nombre fijada: ni una sesión ajena, ni una conexión
// reciclada, ni una sin empresa cambian qué empresa se lee.
func TestThePublicMenuReadsOnlyItsCompanyInTheThreeCases(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	other := makeCompany(t, st, "pm-otra")
	mine := makeProduct(t, st, "Del negocio uno", dec("10"), false)
	name := currentMenuName(t, st, defaultCompanyID)
	otherName := currentMenuName(t, st, other)

	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, ast *store.Store, ctx context.Context) {
		svc := app.NewPublicMenuService(ast, testMenuBase)
		m, _, err := svc.Menu(ctx, otherName)
		if err != nil {
			t.Fatalf("el menú de la otra empresa: %v", err)
		}
		for _, c := range m.Categories {
			for _, p := range c.Products {
				if p.ID == mine {
					t.Fatal("el menú de otra empresa trae un producto de esta")
				}
			}
		}
		m, _, err = svc.Menu(ctx, name)
		if err != nil {
			t.Fatalf("el menú propio: %v", err)
		}
		found := false
		for _, c := range m.Categories {
			for _, p := range c.Products {
				found = found || p.ID == mine
			}
		}
		if !found {
			t.Fatal("el menú propio no trae su producto: la empresa del nombre no se fijó")
		}
	})
}

// AJUSTES › VENTA EN WEB (US2): enlace y QR con la base configurada; una empresa nacida durante el
// traslape (sin menú) lo recibe al abrir Ajustes; una empresa nueva nace con menú nombrado por su
// nombre, no por su slug.
func TestPublicMenuSettings(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ast := appRoleStore(t)
	svc := app.NewPublicMenuService(ast, testMenuBase)
	ctx := context.Background()

	fresh := makeCompany(t, st, "pm-slug-de-login")
	if n := currentMenuName(t, st, fresh); n != "test-pm-slug-de-login" {
		t.Fatalf("una empresa nueva debe nacer con el nombre de la empresa («Test pm-slug-de-login»), nació con %q", n)
	}

	who := makeUserIn(t, st, fresh, "admin_pm_settings", "admin")
	tctx := tenantCtx(t, ast, fresh, who)
	got, err := svc.Settings(tctx)
	if err != nil {
		t.Fatal(err)
	}
	var token string
	if err := st.Pool.QueryRow(ctx, `select qr_token from public_menus where company_id = $1`, fresh).Scan(&token); err != nil {
		t.Fatal(err)
	}
	if got.URL != testMenuBase+"/"+got.Name || got.QRURL != testMenuBase+"/q/"+token || got.Palette != "brand" || got.PreviousNames == nil {
		t.Fatalf("ajustes = %+v", got)
	}

	// Como la dejaría la versión anterior de CreateCompany durante el traslape.
	if _, err := st.Pool.Exec(ctx, `delete from public_menus where company_id = $1`, fresh); err != nil {
		t.Fatal(err)
	}
	again, err := svc.Settings(tctx)
	if err != nil {
		t.Fatalf("sin menú, abrir Ajustes debe crearlo: %v", err)
	}
	if again.Name == "" || again.QRURL == got.QRURL {
		t.Fatalf("menú recreado = %+v", again)
	}

	inTheThreeCases(t, fresh, makeCompany(t, st, "pm-settings-b"), func(t *testing.T, ast *store.Store, ctx context.Context) {
		s, err := app.NewPublicMenuService(ast, testMenuBase).Settings(ctx)
		if err == nil && s.Name == again.Name {
			t.Fatal("una sesión ajena (o sin empresa) ve los ajustes del menú de otra empresa")
		}
	})
}

// CAMBIAR EL NOMBRE SIN ROMPER LO COMPARTIDO (US3).
func TestPublicMenuRename(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ast := appRoleStore(t)
	svc := app.NewPublicMenuService(ast, testMenuBase)
	ctx := context.Background()
	co := makeCompany(t, st, "pm-rename")
	other := makeCompany(t, st, "pm-rename-otra")
	tctx := tenantCtx(t, ast, co, makeUserIn(t, st, co, "admin_pm_rename", "admin"))
	first := currentMenuName(t, st, co)
	var token string
	if err := st.Pool.QueryRow(ctx, `select qr_token from public_menus where company_id = $1`, co).Scan(&token); err != nil {
		t.Fatal(err)
	}

	rename := func(n string) (app.PublicMenuSettings, error) {
		return svc.Update(tctx, app.PublicMenuUpdate{Name: &n})
	}
	movedTo := func(n string) string {
		t.Helper()
		m, mv, err := svc.Menu(ctx, n)
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		if mv == nil {
			return m.Name
		}
		return mv.Name
	}

	if s, err := rename("Doña Lupe"); err != nil || s.Name != "dona-lupe" {
		t.Fatalf("el servidor normaliza: %+v %v", s, err)
	}
	if _, err := rename("dona-lupe-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := rename(first); err != nil {
		t.Fatalf("volver a un nombre propio anterior: %v", err)
	}
	for _, n := range []string{"dona-lupe", "dona-lupe-2", first} {
		if got := movedTo(n); got != first {
			t.Fatalf("%s lleva a %s, debía llevar a %s en un salto", n, got, first)
		}
	}
	if mv, err := svc.ByToken(ctx, token); err != nil || mv.Name != first {
		t.Fatalf("el QR lleva a %+v %v", mv, err)
	}
	s, _ := svc.Settings(tctx)
	if len(s.PreviousNames) != 2 {
		t.Fatalf("anteriores = %v", s.PreviousNames)
	}

	otherName := currentMenuName(t, st, other)
	for _, n := range []string{otherName, "admin"} {
		if _, err := rename(n); !errors.Is(err, domain.ErrMenuNameUnavailable) {
			t.Fatalf("%q: %v, quiero no disponible", n, err)
		}
	}
	if _, err := rename("ab"); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("nombre corto: %v", err)
	}
	if p := "red"; true {
		if _, err := svc.Update(tctx, app.PublicMenuUpdate{Palette: &p}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("paleta desconocida: %v", err)
		}
	}
	if p := "green"; true {
		s, err := svc.Update(tctx, app.PublicMenuUpdate{Palette: &p})
		if err != nil || s.Palette != "green" {
			t.Fatalf("paleta: %+v %v", s, err)
		}
		m, _, _ := svc.Menu(ctx, first)
		if m.Brand.Palette != "green" {
			t.Fatalf("el menú no tomó el color: %+v", m.Brand)
		}
	}

	// Dos cambios a la vez de la misma empresa: se forman; ninguno ve un «no disponible» falso.
	ctx2 := tenantCtx(t, ast, co, makeUserIn(t, st, co, "admin_pm_rename_2", "admin"))
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, n := range []string{"carrera-uno", "carrera-dos"} {
		c := tctx
		if i == 1 {
			c = ctx2
		}
		wg.Add(1)
		go func(i int, n string, c context.Context) {
			defer wg.Done()
			_, errs[i] = svc.Update(c, app.PublicMenuUpdate{Name: &n})
		}(i, n, c)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("cambio simultáneo %d: %v", i, err)
		}
	}
	var current int
	if err := st.Pool.QueryRow(ctx, `select count(*) from public_menu_names where company_id = $1 and retired_at is null`, co).Scan(&current); err != nil || current != 1 {
		t.Fatalf("vigentes = %d %v", current, err)
	}

	// El tope de nombres anteriores: con 20, solo se puede volver a uno de ellos.
	if _, err := st.Pool.Exec(ctx, `
		insert into public_menu_names (company_id, menu_id, name, retired_at)
		select m.company_id, m.id, 'relleno-' || g, now() from public_menus m, generate_series(1, 20) g where m.company_id = $1`, co); err != nil {
		t.Fatal(err)
	}
	if _, err := rename("uno-mas"); !errors.Is(err, domain.ErrMenuNameLimit) {
		t.Fatalf("con el tope lleno: %v", err)
	}
	if _, err := rename("relleno-3"); err != nil {
		t.Fatalf("volver a uno anterior con el tope lleno: %v", err)
	}
}
