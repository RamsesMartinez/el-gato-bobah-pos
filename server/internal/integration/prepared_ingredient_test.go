//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// UN INSUMO QUE SE PREPARA EN EL LOCAL: qué lleva y cuánto rinde, sin ciclos (spec 028, historia 5).
//
// Hasta ahora solo la carga de FUDO los creaba, y no había dónde verlos ni corregirlos: uno mal
// cargado descontaba mal para siempre.
func TestCapturingAPreparedIngredient(t *testing.T) {
	owner := newTestStore(t)
	ctx := context.Background()
	admin := makeUser(t, owner, "admin_jarabe", "admin")
	sugar := makeIngredient(t, owner, "Azúcar preparado")
	water := makeIngredient(t, owner, "Agua preparado")
	syrup := makeIngredient(t, owner, "Jarabe preparado")
	g := unitID(t, owner, "g")
	other := makeCompany(t, owner, "empresa-otra-jarabe")

	st := appRoleStore(t)
	tctx, release, err := st.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	svc := app.NewAdminService(st)
	item := func(ing int64, qty string) app.CompositionInputItem {
		return app.CompositionInputItem{IngredientID: ing, Quantity: decimal.RequireFromString(qty), UnitID: g}
	}
	yield := func(s string) *decimal.Decimal {
		v := decimal.RequireFromString(s)
		return &v
	}

	if err := svc.SaveComposition(tctx, app.CompositionOfIngredient, syrup,
		app.CompositionRequest{Items: []app.CompositionInputItem{item(sugar, "500"), item(water, "500")}, Yield: yield("1000")}, admin); err != nil {
		t.Fatal(err)
	}
	v, err := svc.Composition(tctx, app.CompositionOfIngredient, syrup)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Items) != 2 || v.Yield == nil || !v.Yield.Equal(decimal.RequireFromString("1000")) || v.YieldUnitCode != "g" || v.Status != "confirmed" {
		t.Fatalf("insumo preparado guardado: %+v", v)
	}

	t.Run("rechaza un ciclo indirecto", func(t *testing.T) {
		err := svc.SaveComposition(tctx, app.CompositionOfIngredient, sugar,
			app.CompositionRequest{Items: []app.CompositionInputItem{item(syrup, "1")}, Yield: yield("1")}, admin)
		if !errors.Is(err, domain.ErrCompositionCycle) {
			t.Fatalf("el azúcar no puede llevar jarabe, que lleva azúcar: %v", err)
		}
	})
	t.Run("rechaza que se lleve a sí mismo", func(t *testing.T) {
		err := svc.SaveComposition(tctx, app.CompositionOfIngredient, syrup,
			app.CompositionRequest{Items: []app.CompositionInputItem{item(syrup, "1")}, Yield: yield("1")}, admin)
		if !errors.Is(err, domain.ErrCompositionCycle) {
			t.Fatalf("debe rechazarse como ciclo: %v", err)
		}
	})
	t.Run("rechaza insumos sin rendimiento", func(t *testing.T) {
		err := svc.SaveComposition(tctx, app.CompositionOfIngredient, sugar,
			app.CompositionRequest{Items: []app.CompositionInputItem{item(water, "1")}}, admin)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("debe rechazarse: %v", err)
		}
	})

	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewAdminService(st)
		if _, err := svc.Composition(ctx, app.CompositionOfIngredient, syrup); err == nil {
			t.Fatal("se leyó el insumo de otra empresa")
		}
		if err := svc.SaveComposition(ctx, app.CompositionOfIngredient, syrup, app.CompositionRequest{}, admin); err == nil {
			t.Fatal("se borró el insumo preparado de otra empresa")
		}
	})

	t.Run("vuelve a comprarse hecho", func(t *testing.T) {
		if err := svc.SaveComposition(tctx, app.CompositionOfIngredient, syrup, app.CompositionRequest{}, admin); err != nil {
			t.Fatal(err)
		}
		v, err := svc.Composition(tctx, app.CompositionOfIngredient, syrup)
		if err != nil {
			t.Fatal(err)
		}
		if len(v.Items) != 0 || v.Yield != nil {
			t.Fatalf("debe quedar como insumo que se compra: %+v", v)
		}
	})
}
