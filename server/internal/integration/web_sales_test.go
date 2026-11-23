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
