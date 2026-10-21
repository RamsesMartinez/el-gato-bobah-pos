//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// GESTIÓN DE ARTÍCULOS DEL ALMACÉN (spec 042).

// stockAdjust deja un ajuste en el artículo como owner: el trigger de existencias suma la cantidad.
func stockAdjust(t *testing.T, st *store.Store, company, ingredient int64, qty string) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(), `
		insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity)
		values ($1, 'ingrediente', $2, 'ajuste', $3::numeric)`, company, ingredient, qty); err != nil {
		t.Fatalf("ajuste de %d: %v", ingredient, err)
	}
}

func articleNames(t *testing.T, ctx context.Context, svc *app.BackofficeService, f app.ArticleFilter) []string {
	t.Helper()
	rows, err := svc.ListArticles(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Name
	}
	return out
}

// US1: encontrar un artículo entre cientos sin bajar a mano (FR-001). El buscador ignora mayúsculas y
// acentos y también mira los empaques; los filtros funcionan solos y juntos.
func TestFindingAnArticle(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "articulos-buscar")
	svc, ctx := asWaste(t, co)

	hielo := ingredientIn(t, st, co, "Hielo en bolsa", "g")
	leche := ingredientIn(t, st, co, "Leche entera", "ml")
	vasos := ingredientIn(t, st, co, "Vasos de 16 oz", "pieza")
	azucar := ingredientIn(t, st, co, "Azúcar", "g")
	cajeta := ingredientIn(t, st, co, "Cajeta", "ml")
	ctxOwner := context.Background()
	if _, err := st.Pool.Exec(ctxOwner, `update ingredients set current_cost = 0.006 where id = any($1)`, []int64{hielo, vasos, azucar}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctxOwner, `update ingredients set is_active = false where id = $1`, cajeta); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctxOwner, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base) values ($1, $2, 'Garrafa', 4000)`, co, leche); err != nil {
		t.Fatal(err)
	}
	var refri int64
	if err := st.Pool.QueryRow(ctxOwner, `insert into count_places (company_id, name, position) values ($1, 'Refri', 0) returning id`, co).Scan(&refri); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctxOwner, `update ingredients set count_place_id = $2 where id = any($1)`, []int64{hielo, leche}, refri); err != nil {
		t.Fatal(err)
	}
	stockAdjust(t, st, co, azucar, "-250")
	stockAdjust(t, st, co, hielo, "27000")

	for _, c := range []struct {
		name string
		f    app.ArticleFilter
		want []string
	}{
		{"sin filtro: solo los activos, por nombre", app.ArticleFilter{}, []string{"Azúcar", "Hielo en bolsa", "Leche entera", "Vasos de 16 oz"}},
		{"«hie» encuentra el hielo", app.ArticleFilter{Q: "hie"}, []string{"Hielo en bolsa"}},
		{"«AZUCAR» sin acento encuentra «Azúcar»", app.ArticleFilter{Q: "AZUCAR"}, []string{"Azúcar"}},
		{"el nombre de un empaque también encuentra", app.ArticleFilter{Q: "garrafa"}, []string{"Leche entera"}},
		{"archivados: solo los archivados", app.ArticleFilter{Archived: true}, []string{"Cajeta"}},
		{"por lugar", app.ArticleFilter{PlaceID: &refri}, []string{"Hielo en bolsa", "Leche entera"}},
		{"sin lugar", app.ArticleFilter{NoPlace: true}, []string{"Azúcar", "Vasos de 16 oz"}},
		{"sin costo", app.ArticleFilter{NoCost: true}, []string{"Leche entera"}},
		{"existencia negativa", app.ArticleFilter{Negative: true}, []string{"Azúcar"}},
		{"filtros juntos: en el refri y sin costo", app.ArticleFilter{PlaceID: &refri, NoCost: true}, []string{"Leche entera"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := articleNames(t, ctx, svc, c.f); !slices.Equal(got, c.want) {
				t.Fatalf("= %v, quiere %v", got, c.want)
			}
		})
	}

	// La revisión de código encontró que la lista perdió la marca de las recetas estimadas por revisar.
	t.Run("una receta estimada se marca por revisar", func(t *testing.T) {
		if _, err := st.Pool.Exec(context.Background(), `update ingredients set composition_status = 'estimated' where id = $1`, azucar); err != nil {
			t.Fatal(err)
		}
		rows, err := svc.ListArticles(ctx, app.ArticleFilter{Q: "azucar"})
		if err != nil || len(rows) != 1 || rows[0].CompositionStatus != "estimated" {
			t.Fatalf("%+v %v", rows, err)
		}
	})
	t.Run("cada renglón trae su existencia, su lugar y sus empaques", func(t *testing.T) {
		rows, err := svc.ListArticles(ctx, app.ArticleFilter{Q: "leche"})
		if err != nil || len(rows) != 1 {
			t.Fatalf("%v %v", rows, err)
		}
		r := rows[0]
		if r.CountPlace == nil || *r.CountPlace != "Refri" || len(r.Presentations) != 1 || r.Presentations[0].Name != "Garrafa" || r.BaseUnitKind != "volumen" {
			t.Fatalf("renglón = %+v", r)
		}
	})

	t.Run("la lista de una empresa no ve los artículos de otra, en los tres casos", func(t *testing.T) {
		inTheThreeCases(t, co, defaultCompanyID, func(t *testing.T, st *store.Store, ctx context.Context) {
			rows, err := app.NewBackofficeService(st, clock).ListArticles(ctx, app.ArticleFilter{Q: "hielo en bolsa"})
			// Sin empresa, la sucursal no se resuelve y la consulta se niega entera: no filtra nada.
			if errors.Is(err, domain.ErrBranchAmbiguous) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range rows {
				if r.ID == hielo {
					t.Fatalf("se vio el artículo %d de otra empresa", hielo)
				}
			}
		})
	})
}

// La lista se lee con `inventory.view`: el administrador y el gerente sí; el cajero, no (FR-013). Una
// empresa sin artículos recibe [] y no null.
func TestArticlesHTTPNeedsInventoryView(t *testing.T) {
	t.Parallel()
	r, _, token := newStockAPI(t)
	for _, c := range []struct {
		role string
		want int
	}{{"admin", http.StatusOK}, {"gerente", http.StatusOK}, {"cajero", http.StatusForbidden}, {"mesero", http.StatusForbidden}} {
		w := do(t, r, http.MethodGet, "/api/v1/stock/articles", token("art_"+c.role, c.role), nil, "")
		if w.Code != c.want {
			t.Errorf("%s = %d, quiere %d", c.role, w.Code, c.want)
		}
		if c.want == http.StatusOK {
			var raw json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil || string(raw) == "null" {
				t.Errorf("%s: cuerpo %q; quiere un arreglo", c.role, w.Body.String())
			}
		}
	}
	admin := token("art_admin_mal", "admin")
	if w := do(t, r, http.MethodGet, "/api/v1/stock/articles?status=borrados", admin, nil, ""); w.Code != http.StatusBadRequest {
		t.Errorf("un estado desconocido = %d, quiere 400 (no se cae a «activos» en silencio)", w.Code)
	}
	if w := do(t, r, http.MethodGet, "/api/v1/stock/articles?placeId=abc", admin, nil, ""); w.Code != http.StatusBadRequest {
		t.Errorf("un lugar ilegible = %d, quiere 400", w.Code)
	}
}

func gramos(t *testing.T, st *store.Store) int16 { return unitID(t, st, "g") }

func articleCount(t *testing.T, st *store.Store, company int64, name string) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from ingredients where company_id = $1 and name = $2`, company, name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func auditCount(t *testing.T, st *store.Store, action string, entity int64) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from audit_events where action = $1 and entity_id = $2`, action, entity).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// US2: crear un artículo desde la lista, sin gasto ni receta (FR-002). La existencia inicial entra
// como un ajuste; el costo inicial, como una corrección a mano; todo en una sola operación.
func TestCreatingAnArticleFromTheList(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "articulos-crear")
	user := makeUserIn(t, st, co, "crea_articulos", "admin")
	svc, ctx := asWaste(t, co)
	ctx = app.WithActorUser(ctx, user)
	ml := unitID(t, st, "ml")
	stock, cost := qty("2000"), qty("0.024")

	id, err := svc.CreateArticle(ctx, app.ArticleCreate{Name: "  Leche   entera ", BaseUnitID: ml, InitialStock: &stock, InitialCost: &cost})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("queda con su nombre limpio, su existencia y su costo", func(t *testing.T) {
		var name, source string
		var onHand, current string
		if err := st.Pool.QueryRow(context.Background(), `
			select i.name, i.cost_source::text, i.current_cost::text, coalesce((select sum(on_hand) from stock_levels where ingredient_id = i.id), 0)::text
			  from ingredients i where i.id = $1`, id).Scan(&name, &source, &current, &onHand); err != nil {
			t.Fatal(err)
		}
		if name != "Leche entera" || source != "manual" || qty(current).Cmp(cost) != 0 || qty(onHand).Cmp(stock) != 0 {
			t.Fatalf("nombre %q, origen %s, costo %s, existencia %s", name, source, current, onHand)
		}
		var reason string
		if err := st.Pool.QueryRow(context.Background(), `select reason from stock_movements where ingredient_id = $1 and movement_type = 'ajuste'`, id).Scan(&reason); err != nil || reason != "existencia inicial" {
			t.Fatalf("el ajuste de la existencia inicial: %q %v", reason, err)
		}
		var corrections int
		if err := st.Pool.QueryRow(context.Background(), `select count(*) from ingredient_cost_corrections where ingredient_id = $1 and user_id = $2`, id, user).Scan(&corrections); err != nil || corrections != 1 {
			t.Fatalf("correcciones del costo inicial = %d %v; quiere 1", corrections, err)
		}
	})
	t.Run("deja un solo ingredient.created", func(t *testing.T) {
		if n := auditCount(t, st, "ingredient.created", id); n != 1 {
			t.Fatalf("eventos = %d", n)
		}
	})
	t.Run("un nombre que ya existe sin mayúsculas ni acentos no crea otro y dice cuál es", func(t *testing.T) {
		_, err := svc.CreateArticle(ctx, app.ArticleCreate{Name: "LECHE ENTERA", BaseUnitID: ml})
		var taken domain.ArticleNameTaken
		if !errors.As(err, &taken) || taken.ID != id || taken.Archived {
			t.Fatalf("= %v; quiere el %d, activo", err, id)
		}
	})
	t.Run("si el que existe está archivado lo dice, para ofrecer reactivarlo", func(t *testing.T) {
		if _, err := st.Pool.Exec(context.Background(), `update ingredients set is_active = false where id = $1`, id); err != nil {
			t.Fatal(err)
		}
		_, err := svc.CreateArticle(ctx, app.ArticleCreate{Name: "leche entera", BaseUnitID: ml})
		var taken domain.ArticleNameTaken
		if !errors.As(err, &taken) || !taken.Archived {
			t.Fatalf("= %v; quiere el archivado", err)
		}
	})
	t.Run("una unidad que no es base (kg) se rechaza", func(t *testing.T) {
		if _, err := svc.CreateArticle(ctx, app.ArticleCreate{Name: "Harina kg", BaseUnitID: unitID(t, st, "kg")}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("= %v", err)
		}
	})
	t.Run("con dos sucursales y sin selector, la existencia inicial se niega y no queda nada", func(t *testing.T) {
		co2 := makeCompany(t, st, "articulos-dos-sucursales")
		if _, err := st.Pool.Exec(context.Background(), `insert into branches (company_id, name, code) values ($1, 'Segunda', 'SEG')`, co2); err != nil {
			t.Fatal(err)
		}
		u2 := makeUserIn(t, st, co2, "crea_dos_suc", "admin")
		svc2, ctx2 := asWaste(t, co2)
		ctx2 = app.WithActorUser(ctx2, u2)
		_, err := svc2.CreateArticle(ctx2, app.ArticleCreate{Name: "Café en grano", BaseUnitID: gramos(t, st), InitialStock: &stock})
		if !errors.Is(err, domain.ErrBranchAmbiguous) {
			t.Fatalf("= %v; quiere sucursal ambigua", err)
		}
		if n := articleCount(t, st, co2, "Café en grano"); n != 0 {
			t.Fatalf("quedaron %d artículos a medias", n)
		}
	})
}

// FR-003: ninguna captura deja un artículo creado si se cancela o si su guardado falla. El artículo
// nuevo del gasto y de la receta viaja dentro de su guardado; cancelar no llama al servidor.
func TestANewArticleOnlyExistsIfItsCaptureIsSaved(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	g := gramos(t, k.st)

	t.Run("gasto que se guarda: el artículo nace con su renglón", func(t *testing.T) {
		q := qty("2")
		_, err := k.svc.CreateExpense(ctx, app.ExpenseInput{
			CategoryID: k.category, Amount: qty("100"), Status: domain.ExpensePendiente, UserID: k.user,
			Items: []app.ExpenseItemInput{{ItemType: "ingrediente", NewArticle: &app.NewArticle{Name: "Canela en raja", BaseUnitID: g},
				Description: "Canela", Quantity: q, UnitID: &g, Amount: qty("100")}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if n := articleCount(t, k.st, defaultCompanyID, "Canela en raja"); n != 1 {
			t.Fatalf("artículos = %d; quiere 1", n)
		}
	})
	t.Run("gasto que falla al guardar: no queda el artículo", func(t *testing.T) {
		q := qty("2")
		_, err := k.svc.CreateExpense(ctx, app.ExpenseInput{
			CategoryID: k.category, Amount: qty("100"), Status: domain.ExpensePendiente, UserID: k.user,
			Items: []app.ExpenseItemInput{
				{ItemType: "ingrediente", NewArticle: &app.NewArticle{Name: "Clavo de olor", BaseUnitID: g}, Description: "Clavo", Quantity: q, UnitID: &g, Amount: qty("50")},
				// El segundo renglón apunta a un artículo que no existe: el guardado entero se cae.
				{ItemType: "ingrediente", IngredientID: new(int64(999999)), Description: "Fantasma", Quantity: q, UnitID: &g, Amount: qty("50")},
			},
		})
		if err == nil {
			t.Fatal("el gasto con un renglón imposible se guardó")
		}
		if n := articleCount(t, k.st, defaultCompanyID, "Clavo de olor"); n != 0 {
			t.Fatalf("quedó el artículo de un gasto que no se guardó (%d)", n)
		}
	})
	t.Run("gasto con el nombre de uno que ya existe: usa ese, no crea otro", func(t *testing.T) {
		q := qty("1")
		if _, err := k.svc.CreateExpense(ctx, app.ExpenseInput{
			CategoryID: k.category, Amount: qty("10"), Status: domain.ExpensePendiente, UserID: k.user,
			Items: []app.ExpenseItemInput{{ItemType: "ingrediente", NewArticle: &app.NewArticle{Name: "AZÚCAR COMPRA", BaseUnitID: g},
				Description: "Azúcar", Quantity: q, UnitID: &g, Amount: qty("10")}},
		}); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from ingredients where lower(name) like 'azúcar compra'`).Scan(&n); err != nil || n != 1 {
			t.Fatalf("azúcares = %d %v", n, err)
		}
	})

	as := appRoleStore(t)
	admin := app.NewAdminService(as)
	actx, release, err := as.AcquireTenant(context.Background(), defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	actx = app.WithActorUser(actx, k.user)
	t.Run("receta que se guarda: el insumo nuevo nace con ella", func(t *testing.T) {
		p := makeProduct(t, k.st, "Café con canela", qty("40"), false)
		err := admin.SaveComposition(actx, app.CompositionOfProduct, p, app.CompositionRequest{Mode: "items",
			Items: []app.CompositionInputItem{{NewArticle: &app.NewArticle{Name: "Cardamomo", BaseUnitID: g}, Quantity: qty("2"), UnitID: g}}}, k.user)
		if err != nil {
			t.Fatal(err)
		}
		if n := articleCount(t, k.st, defaultCompanyID, "Cardamomo"); n != 1 {
			t.Fatalf("artículos = %d", n)
		}
	})
	t.Run("receta que falla al guardar: no queda el insumo", func(t *testing.T) {
		p := makeProduct(t, k.st, "Té fallido", qty("40"), false)
		err := admin.SaveComposition(actx, app.CompositionOfProduct, p, app.CompositionRequest{Mode: "items",
			Items: []app.CompositionInputItem{
				{NewArticle: &app.NewArticle{Name: "Anís estrella", BaseUnitID: g}, Quantity: qty("2"), UnitID: g},
				{IngredientID: 999999, Quantity: qty("1"), UnitID: g},
			}}, k.user)
		if err == nil {
			t.Fatal("la receta imposible se guardó")
		}
		if n := articleCount(t, k.st, defaultCompanyID, "Anís estrella"); n != 0 {
			t.Fatalf("quedó el insumo de una receta que no se guardó (%d)", n)
		}
	})
}

// La alta desde la lista pide `inventory.configure`; la ruta vieja POST /ingredients se conserva una
// subida más para las tabletas que todavía corren la pantalla anterior (Revisión B1).
func TestCreatingArticlesHTTP(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	g := strconv.Itoa(int(gramos(t, st)))
	body := []byte(`{"name":"Avena","baseUnitId":` + g + `}`)
	if w := do(t, r, http.MethodPost, "/api/v1/stock/articles", token("art_cajero_crea", "cajero"), body, "application/json"); w.Code != http.StatusForbidden {
		t.Fatalf("cajero = %d, quiere 403", w.Code)
	}
	admin := token("art_admin_crea", "admin")
	if w := do(t, r, http.MethodPost, "/api/v1/stock/articles", admin, body, "application/json"); w.Code != http.StatusCreated {
		t.Fatalf("admin = %d %s", w.Code, w.Body.String())
	}
	w := do(t, r, http.MethodPost, "/api/v1/stock/articles", admin, []byte(`{"name":"AVENA","baseUnitId":`+g+`}`), "application/json")
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"NAME_TAKEN"`) || !strings.Contains(w.Body.String(), `"existingId"`) {
		t.Fatalf("repetido = %d %s; quiere 409 NAME_TAKEN con el que existe", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodPost, "/api/v1/ingredients", admin, []byte(`{"name":"Linaza","baseUnitId":`+g+`}`), "application/json"); w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Fatalf("la ruta vieja = %d; debe seguir viva una subida más", w.Code)
	}
}

// US3: la ficha se edita sin pisar lo que otra persona guardó; el costo se corrige a mano salvo en un
// preparado; el historial es por compra y la bitácora junta lo del artículo y sus empaques.
func TestEditingAnArticle(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	svc := k.svc

	t.Run("editar con la versión vigente guarda y deja ingredient.updated con lo que cambió", func(t *testing.T) {
		a, err := svc.GetArticle(ctx, k.azucar)
		if err != nil {
			t.Fatal(err)
		}
		min := qty("5000")
		if _, err := svc.UpdateArticle(ctx, k.azucar, app.ArticleUpdate{ExpectedUpdatedAt: a.UpdatedAt, Name: new("Azúcar estándar"), MinStock: app.Field[decimal.Decimal]{Set: true, Value: &min}}); err != nil {
			t.Fatal(err)
		}
		b, _ := svc.GetArticle(ctx, k.azucar)
		if b.Name != "Azúcar estándar" || b.MinStock == nil || !b.MinStock.Equal(min) {
			t.Fatalf("ficha = %+v", b)
		}
		if n := auditCount(t, k.st, "ingredient.updated", k.azucar); n != 1 {
			t.Fatalf("eventos = %d", n)
		}
		t.Run("con la versión vieja: 409 y no pisa", func(t *testing.T) {
			_, err := svc.UpdateArticle(ctx, k.azucar, app.ArticleUpdate{ExpectedUpdatedAt: a.UpdatedAt, Name: new("Otro nombre")})
			if !errors.Is(err, domain.ErrArticleStale) {
				t.Fatalf("= %v; quiere ErrArticleStale", err)
			}
		})
	})
	t.Run("renombrar al nombre de otro artículo: 409 con cuál", func(t *testing.T) {
		otro := ingredientIn(t, k.st, defaultCompanyID, "Piloncillo", "g")
		a, _ := svc.GetArticle(ctx, otro)
		_, err := svc.UpdateArticle(ctx, otro, app.ArticleUpdate{ExpectedUpdatedAt: a.UpdatedAt, Name: new("AZUCAR ESTANDAR")})
		var taken domain.ArticleNameTaken
		if !errors.As(err, &taken) || taken.ID != k.azucar {
			t.Fatalf("= %v", err)
		}
	})
	t.Run("corregir el costo de un preparado: 409", func(t *testing.T) {
		prep := ingredientIn(t, k.st, defaultCompanyID, "Jarabe de la casa", "ml")
		recipe := makeRecipe(t, k.st, map[int64]string{k.azucar: "10"})
		if _, err := k.st.Pool.Exec(context.Background(), `update ingredients set is_prep = true, recipe_id = $2, yield_qty = 1000 where id = $1`, prep, recipe); err != nil {
			t.Fatal(err)
		}
		if err := svc.CorrectArticleCost(ctx, prep, qty("0.05")); !errors.Is(err, domain.ErrPrepCost) {
			t.Fatalf("= %v; quiere ErrPrepCost", err)
		}
	})

	// Dos compras, un regalo y una corrección: el historial muestra las dos compras y la corrección, la
	// más reciente arriba, con el empaque y lo pagado por empaque; el regalo no es un costo.
	if _, err := svc.CreateExpense(ctx, k.bultos("2", "1000")); err != nil { // 0.02 por g
		t.Fatal(err)
	}
	if _, err := svc.CreateExpense(ctx, k.bultos("1", "600")); err != nil { // 0.024 por g
		t.Fatal(err)
	}
	gift := qty("1")
	if _, err := svc.CreateExpense(ctx, app.ExpenseInput{ReceivedAt: "2026-07-21", CategoryID: k.category, Amount: qty("1"), Status: domain.ExpensePendiente, UserID: k.user,
		Items: []app.ExpenseItemInput{
			{ItemType: "ingrediente", IngredientID: &k.azucar, Description: "Regalo", Quantity: gift, PresentationID: &k.bulto, QtyReceived: &gift, IsGift: true},
			{Description: "Bolsa", Quantity: qty("1"), Amount: qty("1")},
		}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.CorrectArticleCost(ctx, k.azucar, qty("0.022")); err != nil {
		t.Fatal(err)
	}

	t.Run("historial: compras y corrección, sin el regalo, la más reciente arriba", func(t *testing.T) {
		page, err := svc.ArticleCostHistory(ctx, k.azucar, nil)
		if err != nil {
			t.Fatal(err)
		}
		kinds := []string{}
		for _, r := range page.Items {
			kinds = append(kinds, r.Kind)
		}
		if !slices.Equal(kinds, []string{"correction", "purchase", "purchase"}) {
			t.Fatalf("renglones = %v; quiere corrección y dos compras", kinds)
		}
		last := page.Items[1]
		if last.Presentation == nil || *last.Presentation != "Bulto" || !last.UnitCost.Equal(qty("0.024")) || last.PackCost == nil || !last.PackCost.Equal(qty("600")) {
			t.Fatalf("la última compra = %+v; quiere Bulto, 0.024 por g y $600 por bulto", last)
		}
		if page.Items[0].User == nil {
			t.Fatal("la corrección no dice quién la hizo")
		}
		if page.TrendPct == nil || !page.TrendPct.Equal(qty("10")) {
			t.Fatalf("tendencia = %v; quiere +10 %% (0.022 contra 0.02)", page.TrendPct)
		}
	})
	t.Run("con una sola compra no hay tendencia", func(t *testing.T) {
		solo := ingredientIn(t, k.st, defaultCompanyID, "Sal de grano", "g")
		page, err := svc.ArticleCostHistory(ctx, solo, nil)
		if err != nil || page.TrendPct != nil || page.Items == nil {
			t.Fatalf("%+v %v; sin compras: sin tendencia y [] (no null)", page, err)
		}
	})
	t.Run("la bitácora del artículo trae lo suyo, no lo de otros", func(t *testing.T) {
		page, err := svc.ArticleAudit(ctx, k.azucar, nil)
		if err != nil {
			t.Fatal(err)
		}
		actions := map[string]bool{}
		for _, e := range page.Items {
			actions[e.Action] = true
		}
		if !actions["ingredient.updated"] || !actions["ingredient.cost_corrected"] || actions["ingredient.created"] {
			t.Fatalf("acciones = %v", actions)
		}
	})
	t.Run("ficha, historial y bitácora de otra empresa no se ven, en los tres casos", func(t *testing.T) {
		inTheThreeCases(t, defaultCompanyID, makeCompany(t, k.st, "ficha-ajena"), func(t *testing.T, st *store.Store, ctx context.Context) {
			other := app.NewBackofficeService(st, clock)
			if _, err := other.GetArticle(ctx, k.azucar); err == nil {
				t.Fatal("se vio la ficha de otra empresa")
			}
			if page, err := other.ArticleCostHistory(ctx, k.azucar, nil); err == nil && len(page.Items) > 0 {
				t.Fatal("se vio el historial de otra empresa")
			}
			if page, err := other.ArticleAudit(ctx, k.azucar, nil); err == nil && len(page.Items) > 0 {
				t.Fatal("se vio la bitácora de otra empresa")
			}
		})
	})
}

// Leer la ficha pide `inventory.view`; editarla y corregir el costo, `inventory.configure` (FR-013).
func TestArticleSheetHTTPPermissions(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	id := strconv.FormatInt(ingredientIn(t, st, defaultCompanyID, "Leche permisos", "ml"), 10)
	cajero := token("ficha_cajero", "cajero")
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/stock/articles/" + id, ""},
		{http.MethodGet, "/api/v1/stock/articles/" + id + "/cost-history", ""},
		{http.MethodGet, "/api/v1/stock/articles/" + id + "/audit", ""},
		{http.MethodPatch, "/api/v1/stock/articles/" + id, `{"name":"x","expectedUpdatedAt":"2026-01-01T00:00:00Z"}`},
		{http.MethodPost, "/api/v1/stock/articles/" + id + "/cost", `{"cost":"1"}`},
	} {
		if w := do(t, r, c.method, c.path, cajero, []byte(c.body), "application/json"); w.Code != http.StatusForbidden {
			t.Errorf("cajero %s %s = %d, quiere 403", c.method, c.path, w.Code)
		}
	}
	admin := token("ficha_admin", "admin")
	w := do(t, r, http.MethodGet, "/api/v1/stock/articles/"+id, admin, nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("admin ficha = %d %s", w.Code, w.Body.String())
	}
	var sheet struct {
		UpdatedAt string `json:"updatedAt"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &sheet)
	if w := do(t, r, http.MethodPatch, "/api/v1/stock/articles/"+id, admin, []byte(`{"name":"Leche permisos 2","expectedUpdatedAt":"2020-01-01T00:00:00Z"}`), "application/json"); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "STALE") {
		t.Fatalf("versión vieja = %d %s; quiere 409 STALE", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodPatch, "/api/v1/stock/articles/"+id, admin, []byte(`{"name":"Leche permisos 2","expectedUpdatedAt":"`+sheet.UpdatedAt+`"}`), "application/json"); w.Code != http.StatusOK {
		t.Fatalf("editar = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodGet, "/api/v1/stock/articles/"+id+"/cost-history?before=basura", admin, nil, ""); w.Code != http.StatusBadRequest {
		t.Fatalf("cursor ilegible = %d, quiere 400", w.Code)
	}
}

// US4: archivar quita el artículo de la captura sin borrar su historia (FR-007, FR-008).
func TestArchivingAnArticle(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	svc := k.svc

	t.Run("con una receta activa se niega y dice cuál", func(t *testing.T) {
		check, err := svc.ArticleArchiveCheck(ctx, k.azucar)
		if err != nil {
			t.Fatal(err)
		}
		if len(check.ActiveRecipes) != 1 || check.ActiveRecipes[0].Name != "Agua de azúcar" {
			t.Fatalf("recetas = %+v", check.ActiveRecipes)
		}
		if err := svc.ArchiveArticle(ctx, k.azucar); !errors.Is(err, domain.ErrArticleInUse) {
			t.Fatalf("= %v; quiere ErrArticleInUse", err)
		}
	})

	suelto := ingredientIn(t, k.st, defaultCompanyID, "Colorante rojo", "ml")
	stockAdjust(t, k.st, defaultCompanyID, suelto, "120")
	t.Run("sin recetas: la revisión dice cuánto queda y archiva", func(t *testing.T) {
		check, err := svc.ArticleArchiveCheck(ctx, suelto)
		if err != nil {
			t.Fatal(err)
		}
		if len(check.OnHand) != 1 || !check.OnHand[0].Qty.Equal(qty("120")) || check.ActiveRecipes == nil {
			t.Fatalf("revisión = %+v", check)
		}
		if err := svc.ArchiveArticle(ctx, suelto); err != nil {
			t.Fatal(err)
		}
		if n := auditCount(t, k.st, "ingredient.archived", suelto); n != 1 {
			t.Fatalf("eventos = %d", n)
		}
	})
	t.Run("archivado deja de ofrecerse en el buscador del gasto y en el conteo", func(t *testing.T) {
		arts, err := svc.SearchArticles(ctx, "colorante")
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range arts {
			if a.ID == suelto {
				t.Fatal("el gasto sigue ofreciendo un archivado")
			}
		}
	})
	t.Run("sigue en sus movimientos pasados", func(t *testing.T) {
		var n int
		if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_movements where ingredient_id = $1`, suelto).Scan(&n); err != nil || n != 1 {
			t.Fatalf("movimientos = %d %v", n, err)
		}
	})
	t.Run("reactivar lo devuelve tal como estaba", func(t *testing.T) {
		if err := svc.ReactivateArticle(ctx, suelto); err != nil {
			t.Fatal(err)
		}
		a, err := svc.GetArticle(ctx, suelto)
		if err != nil || !a.IsActive || !a.OnHand.Equal(qty("120")) {
			t.Fatalf("%+v %v", a, err)
		}
		if n := auditCount(t, k.st, "ingredient.reactivated", suelto); n != 1 {
			t.Fatalf("eventos = %d", n)
		}
	})
	t.Run("con un conteo abierto que ya lo capturó se niega", func(t *testing.T) {
		count, err := svc.OpenCount(ctx, k.user)
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.CaptureCount(ctx, count.ID, suelto, qty("100"), k.user); err != nil {
			t.Fatal(err)
		}
		if err := svc.ArchiveArticle(ctx, suelto); !errors.Is(err, domain.ErrArticleInUse) {
			t.Fatalf("= %v; quiere ErrArticleInUse por el conteo abierto", err)
		}
		if err := svc.DiscardCount(ctx, count.ID, k.user); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("con una compra por recibir: avisa y archiva; recibirla después no lo reactiva", func(t *testing.T) {
		q := qty("2")
		g := gramos(t, k.st)
		fresa := ingredientIn(t, k.st, defaultCompanyID, "Fresa congelada", "g")
		exp, err := svc.CreateExpense(ctx, app.ExpenseInput{CategoryID: k.category, Amount: qty("80"), Status: domain.ExpensePendiente, UserID: k.user,
			Items: []app.ExpenseItemInput{{ItemType: "ingrediente", IngredientID: &fresa, Description: "Fresa", Quantity: q, UnitID: &g, Amount: qty("80")}}})
		if err != nil {
			t.Fatal(err)
		}
		check, err := svc.ArticleArchiveCheck(ctx, fresa)
		if err != nil || check.PendingReceipts != 1 {
			t.Fatalf("por recibir = %+v %v", check, err)
		}
		if err := svc.ArchiveArticle(ctx, fresa); err != nil {
			t.Fatal(err)
		}
		var item int64
		if err := k.st.Pool.QueryRow(context.Background(), `select id from expense_items where expense_id = $1`, exp).Scan(&item); err != nil {
			t.Fatal(err)
		}
		if err := svc.ReceiveItem(ctx, exp, item, app.ReceiveItemInput{Qty: q, SeenApplied: qty("0")}, k.user); err != nil {
			t.Fatalf("recibir lo pendiente de un archivado: %v", err)
		}
		a, err := svc.GetArticle(ctx, fresa)
		if err != nil || a.IsActive || !a.OnHand.Equal(qty("2")) {
			t.Fatalf("tras recibir: activo %v, existencia %s, %v", a.IsActive, a.OnHand, err)
		}
	})
	t.Run("archive-check de otra empresa no ve nada, en los tres casos", func(t *testing.T) {
		inTheThreeCases(t, defaultCompanyID, makeCompany(t, k.st, "archivar-ajena"), func(t *testing.T, st *store.Store, ctx context.Context) {
			check, err := app.NewBackofficeService(st, clock).ArticleArchiveCheck(ctx, k.azucar)
			if err == nil && (len(check.ActiveRecipes) > 0 || len(check.OnHand) > 0) {
				t.Fatalf("se vio la revisión de otra empresa: %+v", check)
			}
		})
	})
}

// Archivar, reactivar y la lista de categorías por permiso (FR-013).
func TestArchivingHTTPPermissions(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	id := strconv.FormatInt(ingredientIn(t, st, defaultCompanyID, "Archivable http", "ml"), 10)
	cajero := token("arch_cajero", "cajero")
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/stock/articles/" + id + "/archive-check"},
		{http.MethodPost, "/api/v1/stock/articles/" + id + "/archive"},
		{http.MethodPost, "/api/v1/stock/articles/" + id + "/reactivate"},
		{http.MethodGet, "/api/v1/stock/ingredient-categories"},
		{http.MethodPost, "/api/v1/stock/ingredient-categories"},
	} {
		if w := do(t, r, c.method, c.path, cajero, []byte(`{"name":"x"}`), "application/json"); w.Code != http.StatusForbidden {
			t.Errorf("cajero %s %s = %d, quiere 403", c.method, c.path, w.Code)
		}
	}
	admin := token("arch_admin", "admin")
	if w := do(t, r, http.MethodPost, "/api/v1/stock/articles/"+id+"/archive", admin, nil, ""); w.Code != http.StatusNoContent {
		t.Fatalf("archivar = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodPost, "/api/v1/stock/ingredient-categories", admin, []byte(`{"name":"Lácteos"}`), "application/json"); w.Code != http.StatusCreated {
		t.Fatalf("crear categoría = %d %s", w.Code, w.Body.String())
	}
	w := do(t, r, http.MethodGet, "/api/v1/stock/ingredient-categories", admin, nil, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Lácteos") {
		t.Fatalf("categorías = %d %s", w.Code, w.Body.String())
	}
}

// US6: los empaques sugeridos son de artículos que se miden igual, con su cantidad (FR-011), y cada
// cambio de empaque queda en la bitácora sin tocar las compras pasadas.
func TestPresentationSuggestionsAndAudit(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t) // el azúcar tiene «Bulto = 25 kg»
	ctx := app.WithActorUser(k.ctx, k.user)
	svc := k.svc
	leche := ingredientIn(t, k.st, defaultCompanyID, "Leche sugerencias", "ml")
	l := unitID(t, k.st, "l")
	if _, err := svc.CreatePresentation(ctx, leche, app.PresentationInput{Name: "Garrafa", Qty: qty("4"), UnitID: l}); err != nil {
		t.Fatal(err)
	}

	t.Run("un artículo en líquido no ve el bulto de 25 kg, y uno en peso sí", func(t *testing.T) {
		liquid, err := svc.PresentationSuggestions(ctx, "volumen")
		if err != nil {
			t.Fatal(err)
		}
		if len(liquid) != 1 || liquid[0].Name != "Garrafa" || !liquid[0].QtyInBase.Equal(qty("4000")) {
			t.Fatalf("en líquido = %+v; quiere solo «Garrafa · 4 L»", liquid)
		}
		weight, err := svc.PresentationSuggestions(ctx, "masa")
		if err != nil || len(weight) != 1 || weight[0].Name != "Bulto" {
			t.Fatalf("en peso = %+v %v", weight, err)
		}
	})
	t.Run("una clase desconocida es 400", func(t *testing.T) {
		if _, err := svc.PresentationSuggestions(ctx, "densidad"); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("= %v", err)
		}
	})
	t.Run("crear, editar y retirar dejan su evento", func(t *testing.T) {
		p, err := svc.CreatePresentation(ctx, leche, app.PresentationInput{Name: "Caja", Qty: qty("1"), UnitID: l})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.UpdatePresentation(ctx, p.ID, app.PresentationInput{Name: "Caja", Qty: qty("2"), UnitID: l}); err != nil {
			t.Fatal(err)
		}
		off := false
		if _, err := svc.UpdatePresentation(ctx, p.ID, app.PresentationInput{Name: "Caja", Qty: qty("2"), UnitID: l, Active: &off}); err != nil {
			t.Fatal(err)
		}
		for action, want := range map[string]int{"presentation.created": 1, "presentation.updated": 1, "presentation.retired": 1} {
			if n := auditCount(t, k.st, action, p.ID); n != want {
				t.Errorf("%s = %d, quiere %d", action, n, want)
			}
		}
	})
	t.Run("cambiar la cantidad de un empaque no cambia lo que ya se compró en él", func(t *testing.T) {
		exp, err := svc.CreateExpense(ctx, k.bultos("1", "500"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.UpdatePresentation(ctx, k.bulto, app.PresentationInput{Name: "Bulto", Qty: qty("20"), UnitID: unitID(t, k.st, "kg")}); err != nil {
			t.Fatal(err)
		}
		var pack string
		if err := k.st.Pool.QueryRow(context.Background(), `select pack_qty_in_base::text from expense_items where expense_id = $1`, exp).Scan(&pack); err != nil || !qty(pack).Equal(qty("25000")) {
			t.Fatalf("la compra quedó con %s; quiere 25000 (como se compró)", pack)
		}
	})
	t.Run("las sugerencias de otra empresa no se ven, en los tres casos", func(t *testing.T) {
		inTheThreeCases(t, defaultCompanyID, makeCompany(t, k.st, "sugerencias-ajena"), func(t *testing.T, st *store.Store, ctx context.Context) {
			rows, err := app.NewBackofficeService(st, clock).PresentationSuggestions(ctx, "masa")
			if err == nil && len(rows) > 0 {
				t.Fatalf("se vieron empaques de otra empresa: %+v", rows)
			}
		})
	})
}

func TestPresentationsHTTPNeedConfigure(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	cajero := token("emp_cajero_042", "cajero")
	id := strconv.FormatInt(ingredientIn(t, st, defaultCompanyID, "Leche empaques 042", "ml"), 10)
	l := strconv.Itoa(int(unitID(t, st, "l")))
	if w := do(t, r, http.MethodPost, "/api/v1/stock/ingredients/"+id+"/presentations", cajero, []byte(`{"name":"Caja","qty":"1","unitId":`+l+`}`), "application/json"); w.Code != http.StatusForbidden {
		t.Errorf("cajero crea = %d, quiere 403", w.Code)
	}
	if w := do(t, r, http.MethodGet, "/api/v1/stock/presentation-suggestions?unitKind=volumen", cajero, nil, ""); w.Code != http.StatusForbidden {
		t.Errorf("cajero sugerencias = %d, quiere 403", w.Code)
	}
	admin := token("emp_admin_042", "admin")
	if w := do(t, r, http.MethodGet, "/api/v1/stock/presentation-suggestions?unitKind=volumen", admin, nil, ""); w.Code != http.StatusOK {
		t.Errorf("admin sugerencias = %d %s", w.Code, w.Body.String())
	}
}

// AUTOGUARDADO (spec 043, R3): la ficha manda un campo a la vez. Un campo que no viene se conserva y
// uno que viene en null se limpia; antes la petición traía la ficha entera y un campo omitido se
// borraba. La respuesta trae la versión nueva: sin ella, el guardado siguiente daría 409 contra sí
// mismo.
func TestSavingOneFieldOfAnArticle(t *testing.T) {
	t.Parallel()
	r, st, token := newStockAPI(t)
	admin := token("campo_admin", "admin")
	id := ingredientIn(t, st, defaultCompanyID, "Café de olla", "g")
	ctx := context.Background()
	var cat, place int64
	if err := st.Pool.QueryRow(ctx, `insert into ingredient_categories (company_id, name) values ($1, 'Abarrotes 043') returning id`, defaultCompanyID).Scan(&cat); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into count_places (company_id, name, position) values ($1, 'Bodega 043', 99) returning id`, defaultCompanyID).Scan(&place); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `update ingredients set category_id = $2, count_place_id = $3, min_stock = 100 where id = $1`, id, cat, place); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/stock/articles/" + strconv.FormatInt(id, 10)
	ficha := func() map[string]any {
		w := do(t, r, http.MethodGet, path, admin, nil, "")
		var m map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		return m
	}
	patch := func(body string) (int, map[string]any) {
		w := do(t, r, http.MethodPatch, path, admin, []byte(body), "application/json")
		var m map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return w.Code, m
	}

	v := ficha()["updatedAt"].(string)
	code, res := patch(`{"expectedUpdatedAt":"` + v + `","minStock":"750"}`)
	if code != http.StatusOK || res["updatedAt"] == nil || res["updatedAt"] == v {
		t.Fatalf("solo el mínimo = %d %v; quiere 200 con la versión nueva", code, res)
	}
	f := ficha()
	if f["name"] != "Café de olla" || f["categoryId"] != float64(cat) || f["countPlaceId"] != float64(place) || f["minStock"] != "750" {
		t.Fatalf("mandar solo el mínimo tocó otros campos: %v", f)
	}
	var before, after string
	if err := st.Pool.QueryRow(ctx, `select before::text, after::text from audit_events where action = 'ingredient.updated' and entity_id = $1 order by id desc limit 1`, id).Scan(&before, &after); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(after, "name") || strings.Contains(after, "categoryId") || !strings.Contains(after, "minStock") {
		t.Fatalf("el evento trae más que lo que cambió: %s", after)
	}

	t.Run("el guardado siguiente usa la versión que devolvió el anterior", func(t *testing.T) {
		code, _ := patch(`{"expectedUpdatedAt":"` + res["updatedAt"].(string) + `","name":"Café de olla molido"}`)
		if code != http.StatusOK {
			t.Fatalf("= %d; el segundo guardado chocó contra el primero", code)
		}
		if f := ficha(); f["name"] != "Café de olla molido" || f["minStock"] != "750" {
			t.Fatalf("ficha = %v", f)
		}
	})
	t.Run("null explícito limpia ese campo y solo ese", func(t *testing.T) {
		code, _ := patch(`{"expectedUpdatedAt":"` + ficha()["updatedAt"].(string) + `","categoryId":null}`)
		if code != http.StatusOK {
			t.Fatalf("= %d", code)
		}
		if f := ficha(); f["categoryId"] != nil || f["countPlaceId"] != float64(place) || f["minStock"] != "750" {
			t.Fatalf("limpiar la categoría tocó otros campos: %v", f)
		}
	})
	t.Run("la ficha entera, como la manda la pantalla anterior, sigue funcionando", func(t *testing.T) {
		body := `{"expectedUpdatedAt":"` + ficha()["updatedAt"].(string) + `","name":"Café","categoryId":null,"placeId":null,"minStock":"50"}`
		if code, _ := patch(body); code != http.StatusOK {
			t.Fatalf("= %d", code)
		}
		if f := ficha(); f["name"] != "Café" || f["countPlaceId"] != nil || f["minStock"] != "50" {
			t.Fatalf("ficha = %v", f)
		}
	})
	t.Run("otra empresa no edita la ficha, en los tres casos", func(t *testing.T) {
		inTheThreeCases(t, defaultCompanyID, makeCompany(t, st, "campo-ajeno"), func(t *testing.T, st *store.Store, ctx context.Context) {
			other := app.NewBackofficeService(st, clock)
			if _, err := other.UpdateArticle(ctx, id, app.ArticleUpdate{ExpectedUpdatedAt: time.Now(), Name: new("Ajeno")}); err == nil {
				t.Fatal("otra empresa editó la ficha")
			}
		})
		if f := ficha(); f["name"] != "Café" {
			t.Fatalf("la ficha cambió: %v", f)
		}
	})
}
