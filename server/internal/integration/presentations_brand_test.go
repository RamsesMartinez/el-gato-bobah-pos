//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

func strp(s string) *string { return &s }

// La marca en la presentación (spec 056, US4): opcional, distingue «Caja · Marca A» de «Caja · Marca B»
// en el mismo insumo, y la compra guarda la marca con la que se compró aunque después se edite.
func TestPresentationBrand(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	leche := ingredientIn(t, st, defaultCompanyID, "Leche marca", "ml")
	svc, tctx := asTenant(t, defaultCompanyID)
	liters := unitID(t, st, "l")
	caja := func(brand *string) app.PresentationInput {
		return app.PresentationInput{Name: "Caja", Qty: qty("12"), UnitID: liters, Brand: brand}
	}

	a, err := svc.CreatePresentation(tctx, leche, caja(strp("Marca A")))
	if err != nil || a.Brand == nil || *a.Brand != "Marca A" {
		t.Fatalf("Caja · Marca A = %+v %v", a, err)
	}
	b, err := svc.CreatePresentation(tctx, leche, caja(strp("  Marca B ")))
	if err != nil || *b.Brand != "Marca B" {
		t.Fatalf("otra «Caja» con otra marca debe convivir: %+v %v", b, err)
	}
	if _, err := svc.CreatePresentation(tctx, leche, caja(strp("marca a"))); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("la misma marca otra vez = %v; quiere 409", err)
	}
	plain, err := svc.CreatePresentation(tctx, leche, caja(strp("   ")))
	if err != nil || plain.Brand != nil {
		t.Fatalf("solo espacios es sin marca: %+v %v", plain, err)
	}
	if _, err := svc.CreatePresentation(tctx, leche, caja(nil)); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("dos «Caja» sin marca = %v; quiere 409", err)
	}
	if _, err := svc.CreatePresentation(tctx, leche, app.PresentationInput{Name: "Bolsa", Qty: qty("1"), UnitID: liters,
		Brand: strp(strings.Repeat("x", 41))}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("una marca de 41 letras = %v; quiere 400", err)
	}

	t.Run("editar sin mandar la marca la conserva; mandarla vacía la quita", func(t *testing.T) {
		got, err := svc.UpdatePresentation(tctx, a.ID, caja(nil))
		if err != nil || got.Brand == nil || *got.Brand != "Marca A" {
			t.Fatalf("sin marca en el cuerpo = %+v %v; quiere que se quede «Marca A»", got, err)
		}
	})

	t.Run("la compra guarda la marca de ese momento", func(t *testing.T) {
		admin := makeUser(t, st, "marca_admin", "admin")
		_, item := purchaseIn(t, st, defaultCompanyID, admin, leche)
		if _, err := st.Pool.Exec(ctx, `update expense_items set presentation_id = $2 where id = $1`, item, b.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.UpdatePresentation(tctx, b.ID, caja(strp("Marca C"))); err != nil {
			t.Fatal(err)
		}
		var brand *string
		if err := st.Pool.QueryRow(ctx, `select presentation_brand from expense_items where id = $1`, item).Scan(&brand); err != nil {
			t.Fatal(err)
		}
		if brand == nil || *brand != "Marca B" {
			t.Fatalf("la compra quedó con %v; quiere «Marca B», la de cuando se compró", brand)
		}
	})
}

// Buscar en Artículos encuentra el insumo por la marca de su empaque (prueba con usuario nuevo: «lala»
// daba «Nada coincide» con una «Caja · Lala»), y la lista trae la marca de cada empaque.
func TestArticlesAreFoundByPresentationBrand(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	leche := ingredientIn(t, st, defaultCompanyID, "Leche busca marca", "ml")
	svc, tctx := asTenant(t, defaultCompanyID)
	if _, err := svc.CreatePresentation(tctx, leche, app.PresentationInput{Name: "Caja", Qty: qty("12"), UnitID: unitID(t, st, "l"), Brand: strp("Lala")}); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.ListArticles(tctx, app.ArticleFilter{Q: "lala"})
	if err != nil || len(rows) != 1 || rows[0].ID != leche {
		t.Fatalf("buscar «lala» = %+v %v", rows, err)
	}
	if len(rows[0].Presentations) != 1 || rows[0].Presentations[0].Brand == nil || *rows[0].Presentations[0].Brand != "Lala" {
		t.Fatalf("la lista no trae la marca del empaque: %+v", rows[0].Presentations)
	}
}
