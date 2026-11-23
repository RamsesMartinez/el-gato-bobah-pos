//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// La cotización de una cuenta que todavía no existe (spec 090): la vista previa de order_draft_create.
// Lo que importa es que diga la MISMA cifra que la cuenta dirá al crearse, y que rechace lo mismo.

type quoteFixture struct {
	uber         int16
	conPropio    int64 // precio propio en Uber: 150
	sinPropio    int64 // sin precio propio: 80 × 1.35 = 108
	extra        int64 // +10 en el producto sin propio
	productoOtra int64
}

func seedQuote(t *testing.T, k *draftsKit) quoteFixture {
	t.Helper()
	ctx := context.Background()
	f := quoteFixture{uber: platformID(t, k.st, defaultCompanyID, "Uber Eats")}
	f.conPropio = makeProduct(t, k.st, "Ramen con precio propio", pesos("100"), false)
	f.sinPropio = makeProduct(t, k.st, "Ramen sin precio propio", pesos("80"), false)
	ex := func(sql string, args ...any) {
		t.Helper()
		if _, err := k.st.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	ex(`update delivery_platforms set price_markup_pct = 35 where id = $1`, f.uber)
	ex(`insert into product_platform_prices (company_id, product_id, platform_id, price, updated_by) values ($1, $2, $3, 150, $4)`,
		defaultCompanyID, f.conPropio, f.uber, k.user)
	var g int64
	if err := k.st.Pool.QueryRow(ctx, `insert into modifier_groups (name, default_max_select) values ('Toppings cotizados', 3) returning id`).Scan(&g); err != nil {
		t.Fatal(err)
	}
	if err := k.st.Pool.QueryRow(ctx, `insert into modifier_options (group_id, name, price_delta) values ($1, 'Huevo cotizado', 10) returning id`, g).Scan(&f.extra); err != nil {
		t.Fatal(err)
	}
	ex(`insert into product_modifier_groups (product_id, group_id) values ($1, $2)`, f.sinPropio, g)
	otra := makeCompany(t, k.st, "cotiza-ajena")
	if err := k.st.Pool.QueryRow(ctx, `insert into categories (company_id, name) values ($1, 'cat-ajena') returning id`, otra).Scan(new(int64)); err != nil {
		t.Fatal(err)
	}
	if err := k.st.Pool.QueryRow(ctx, `insert into products (company_id, name, category_id, price)
		select $1, 'Producto ajeno', id, 40 from categories where company_id = $1 limit 1 returning id`, otra).Scan(&f.productoOtra); err != nil {
		t.Fatal(err)
	}
	return f
}

func uberHeader(f quoteFixture, discount string) *app.DraftHeaderPatch {
	st := "domicilio"
	h := &app.DraftHeaderPatch{ServiceType: &st, PlatformID: app.Field[int16]{Set: true, Value: &f.uber},
		PlatformOrderRef: app.Field[string]{Set: true, Value: new("UB-QUOTE-1")}}
	if discount != "" {
		h.Discount = app.Field[app.DraftDiscount]{Set: true, Value: &app.DraftDiscount{Amount: new(pesos(discount))}}
	}
	return h
}

func quoteLines(f quoteFixture) []app.DraftLineCmd {
	sin := addOf(f.sinPropio, "2")
	sin.Modifiers = []domain.DraftModifier{{OptionID: f.extra, Qty: 1}}
	return []app.DraftLineCmd{addOf(f.conPropio, "1"), sin}
}

func TestDraftQuoteMatchesTheDraftItBecomes(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	f := seedQuote(t, k)
	drafts := func() int64 {
		var n int64
		_ = k.st.Pool.QueryRow(context.Background(), `select count(*) from order_drafts`).Scan(&n)
		return n
	}
	before := drafts()

	q, err := k.drafts.Quote(k.ctx, uberHeader(f, "20"), quoteLines(f), k.user)
	if err != nil {
		t.Fatalf("Quote: %v", err)
	}
	if drafts() != before {
		t.Fatal("cotizar creó una cuenta")
	}
	// 150 + 2 × (108 + 13.50) = 393; menos 20 = 373. El extra también lleva el margen.
	if !q.Subtotal.Equal(pesos("393")) || !q.DiscountTotal.Equal(pesos("20")) || !q.Total.Equal(pesos("373")) {
		t.Fatalf("subtotal %s, descuento %s, total %s; esperaba 393, 20, 373", q.Subtotal, q.DiscountTotal, q.Total)
	}
	if !q.MarkupPct.Equal(decimal.NewFromInt(35)) {
		t.Fatalf("margen %s", q.MarkupPct)
	}
	src := map[int64]app.QuoteLineView{}
	for _, l := range q.Lines {
		src[l.ProductID] = l
	}
	if l := src[f.conPropio]; l.PriceSource != domain.PriceFromPlatform || !l.UnitPrice.Equal(pesos("150")) || !l.BasePrice.Equal(pesos("100")) {
		t.Fatalf("con precio propio: %+v", l)
	}
	l := src[f.sinPropio]
	if l.PriceSource != domain.PriceFromMarkup || !l.UnitPrice.Equal(pesos("108")) || !l.BasePrice.Equal(pesos("80")) {
		t.Fatalf("sin precio propio: %+v", l)
	}
	if len(l.Modifiers) != 1 || l.Modifiers[0].PriceSource != domain.PriceFromMarkup || !l.Modifiers[0].BasePrice.Equal(pesos("10")) {
		t.Fatalf("extra: %+v", l.Modifiers)
	}

	// La cuenta creada con lo mismo dice la misma cifra, al centavo.
	v, _, err := k.drafts.Create(k.ctx, app.CreateDraftCmd{ID: uuid.New(), Header: uberHeader(f, "20"), Lines: quoteLines(f), Actor: k.user})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !v.Total.Equal(q.Total) || !v.Subtotal.Equal(q.Subtotal) {
		t.Fatalf("la cuenta dice %s/%s y la cotización %s/%s", v.Subtotal, v.Total, q.Subtotal, q.Total)
	}
}

func TestDraftQuoteRejectsWhatCreateRejects(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	f := seedQuote(t, k)
	apagado := makeProduct(t, k.st, "Ramen apagado", pesos("70"), false)
	if _, err := k.st.Pool.Exec(context.Background(), `update products set is_active = false where id = $1`, apagado); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		header *app.DraftHeaderPatch
		lines  []app.DraftLineCmd
	}{
		{"sin renglones", uberHeader(f, ""), nil},
		{"producto que no se vende", nil, []app.DraftLineCmd{addOf(apagado, "1")}},
		{"producto de otra empresa", nil, []app.DraftLineCmd{addOf(f.productoOtra, "1")}},
		{"descuento mayor que la venta", uberHeader(f, "5000"), quoteLines(f)},
		{"cantidad cero", nil, []app.DraftLineCmd{addOf(f.conPropio, "0")}},
	}
	for _, c := range cases {
		_, qerr := k.drafts.Quote(k.ctx, c.header, c.lines, k.user)
		_, _, cerr := k.drafts.Create(k.ctx, app.CreateDraftCmd{ID: uuid.New(), Header: c.header, Lines: c.lines, Actor: k.user})
		if qerr == nil || cerr == nil {
			t.Errorf("%s: cotizar %v, crear %v; los dos debían rechazar", c.name, qerr, cerr)
		}
	}
}

func TestDraftQuoteIsolation(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	f := seedQuote(t, k)
	other := makeCompany(t, k.st, "cotiza-aislada")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewDraftsService(st, app.NewOrdersService(st, clock))
		if _, err := svc.Quote(ctx, nil, []app.DraftLineCmd{addOf(f.conPropio, "1")}, k.user); err == nil {
			t.Fatal("se cotizó un producto de otra empresa")
		}
	})
}

func TestDraftQuoteRoute(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	r, _, token := draftsAPI(t, st)
	_, tok := token("cajero_cotiza", "cajero")
	prod := makeProduct(t, st, "Taro cotizado", pesos("55"), false)
	rec := do(t, r, http.MethodPost, "/api/v1/pos/drafts/quote", tok, jsonBody(t, map[string]any{
		"lines": []map[string]any{{"opId": uuid.New().String(), "productId": prod, "qty": "2"}},
	}), "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /pos/drafts/quote: %d %s", rec.Code, rec.Body.String())
	}
	var q struct {
		Total string `json:"total"`
		Lines []struct {
			PriceSource string `json:"priceSource"`
		} `json:"lines"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &q); err != nil || q.Total != "110" || len(q.Lines) != 1 || q.Lines[0].PriceSource != domain.PriceFromCounter {
		t.Fatalf("cotización: %s (%v)", rec.Body.String(), err)
	}
}
