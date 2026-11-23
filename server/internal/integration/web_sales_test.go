//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LA REGLA ÚNICA DE LA WEB (spec 092, FR-005 y FR-012): qué se vende en la web, a qué precio y si
// está agotado. El menú público (#316) la usa sin volver a decidirla; el resumen de Ajustes la cuenta.
func TestWebOffer(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	admin, actx := adminAsApp(t, st, defaultCompanyID, "web")
	web := app.NewWebSalesService(appRoleStore(t))

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

	offer, err := web.Offer(actx)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]app.WebOfferItem{}
	for _, o := range offer {
		byID[o.ProductID] = o
	}
	if _, ok := byID[hidden]; ok {
		t.Fatal("un producto apagado para la web salió en la web")
	}
	if o := byID[plain]; !o.Price.Equal(dec("20")) || o.SoldOut {
		t.Fatalf("sin precio propio debía heredar 20 y estar disponible: %+v", o)
	}
	if o := byID[priced]; !o.Price.Equal(dec("45")) {
		t.Fatalf("con precio propio debía ser 45: %+v", o)
	}
	if o, ok := byID[soldOut]; !ok || !o.SoldOut {
		t.Fatalf("un producto agotado debe salir marcado, no oculto: %+v", o)
	}

	sum, err := web.Summary(actx)
	if err != nil {
		t.Fatal(err)
	}
	if sum.OnWeb != len(offer) || sum.WithWebPrice != 1 || sum.Total != sum.OnWeb+1 {
		t.Fatalf("resumen = %+v con %d en la web", sum, len(offer))
	}

	inTheThreeCases(t, defaultCompanyID, makeCompany(t, st, "web-b"), func(t *testing.T, ast *store.Store, ctx context.Context) {
		items, err := app.NewWebSalesService(ast).Offer(ctx)
		if err != nil {
			return
		}
		for _, o := range items {
			if o.ProductID == plain || o.ProductID == priced {
				t.Fatal("una sesión ajena (o sin empresa) ve la oferta web de otra empresa")
			}
		}
	})
}

// El menú público pinta lo que la oferta web trae (spec 097): nombre, descripción, foto, categoría y
// orden salen de la MISMA consulta que decide qué se vende, y un producto de una categoría apagada
// no se ofrece (en el POS tampoco se ve). El resumen de Ajustes cuenta sobre la misma base.
func TestWebOfferCarriesWhatThePublicMenuPaints(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	_, actx := adminAsApp(t, st, defaultCompanyID, "web-menu")
	web := app.NewWebSalesService(appRoleStore(t))
	ctx := context.Background()

	shown := makeProduct(t, st, "Horchata", dec("35"), false)
	if _, err := st.Pool.Exec(ctx, `update products set description = 'De arroz', image_url = 'https://media.test/1/products/1/x-1200.webp' where id = $1`, shown); err != nil {
		t.Fatal(err)
	}
	gone := makeProduct(t, st, "De una categoría apagada", dec("10"), false)
	var off int64
	if err := st.Pool.QueryRow(ctx, `insert into categories (company_id, name, is_active) values ($1, 'Apagada', false) returning id`, defaultCompanyID).Scan(&off); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `update products set category_id = $1 where id = $2`, off, gone); err != nil {
		t.Fatal(err)
	}

	before, err := web.Summary(actx)
	if err != nil {
		t.Fatal(err)
	}
	offer, err := web.Offer(actx)
	if err != nil {
		t.Fatal(err)
	}
	var got *app.WebOfferItem
	for i := range offer {
		if offer[i].ProductID == gone {
			t.Fatal("un producto de una categoría apagada salió en la web")
		}
		if offer[i].ProductID == shown {
			got = &offer[i]
		}
	}
	if got == nil || got.Name != "Horchata" || got.Description == nil || *got.Description != "De arroz" ||
		got.ImageURL == nil || got.CategoryID == 0 {
		t.Fatalf("la oferta no trae lo que el menú pinta: %+v", got)
	}
	if before.OnWeb != len(offer) {
		t.Fatalf("el resumen cuenta %d en la web y la oferta trae %d: miden sobre bases distintas", before.OnWeb, len(offer))
	}
}
