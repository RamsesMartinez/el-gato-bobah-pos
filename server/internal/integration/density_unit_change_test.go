//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// CAMBIAR ENTRE PESO Y LÍQUIDO CON LA DENSIDAD (spec 058, US3). La leche condensada se dio de alta en
// g y se usa en ml.

func TestChangingFromGramsToMlUsesTheDensity(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	owner := context.Background()
	leche := ingredientIn(t, k.st, defaultCompanyID, "Leche condensada cambio", "g")
	for range 13 {
		stockAdjust(t, k.st, defaultCompanyID, leche, "100")
	}
	if _, err := k.st.Pool.Exec(owner, `update ingredients set current_cost = 0.1 where id = $1`, leche); err != nil {
		t.Fatal(err)
	}
	var bolsa int64
	if err := k.st.Pool.QueryRow(owner, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, tare_g)
		values ($1, $2, 'Lata', 390, 25) returning id`, defaultCompanyID, leche).Scan(&bolsa); err != nil {
		t.Fatal(err)
	}
	ml := unitID(t, k.st, "ml")

	t.Run("sin densidad no se puede", func(t *testing.T) {
		_, err := k.svc.PreviewUnitChange(ctx, leche, app.UnitChangeRequest{BaseUnitID: ml})
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("= %v; quería 400 «primero captura la densidad»", err)
		}
	})

	if _, err := k.svc.SetDensity(ctx, leche, app.DensityInput{Method: "manual", Density: decp("1.3")}); err != nil {
		t.Fatal(err)
	}

	t.Run("la vista previa dice la densidad y la equivalencia que salen de ella", func(t *testing.T) {
		prev, err := k.svc.PreviewUnitChange(ctx, leche, app.UnitChangeRequest{BaseUnitID: ml})
		if err != nil {
			t.Fatal(err)
		}
		if prev.Density == nil || !prev.Density.Equal(dd("1.3")) || !prev.Factor.Equal(dd("0.769231")) {
			t.Fatalf("densidad %v, equivalencia %s; quería 1.3 y 0.769231", prev.Density, prev.Factor)
		}
	})

	t.Run("una equivalencia que no sale de la densidad vigente se rechaza", func(t *testing.T) {
		err := k.svc.ChangeUnit(ctx, leche, app.UnitChangeRequest{BaseUnitID: ml, Factor: dd("1")})
		if !errors.Is(err, domain.ErrDensityChanged) {
			t.Fatalf("= %v; quería 409 DENSITY_CHANGED", err)
		}
	})

	if err := k.svc.ChangeUnit(ctx, leche, app.UnitChangeRequest{BaseUnitID: ml, Factor: dd("0.769231")}); err != nil {
		t.Fatal(err)
	}

	t.Run("existencia, costo y empaque quedan en ml, con la densidad y la tara intactas", func(t *testing.T) {
		movs, level := sumOf(t, k.st, leche)
		if !level.Equal(movs) || level.Sub(dd("1000")).Abs().GreaterThan(dd("0.01")) {
			t.Fatalf("existencia %s ml (movimientos %s); quería ≈ 1000", level, movs)
		}
		var cost, pres decimal.Decimal
		var tare, density *string
		if err := k.st.Pool.QueryRow(owner, `select i.current_cost, p.qty_in_base, p.tare_g::text, i.density_g_per_ml::text
			from ingredients i join ingredient_presentations p on p.ingredient_id = i.id where i.id = $1`, leche).
			Scan(&cost, &pres, &tare, &density); err != nil {
			t.Fatal(err)
		}
		if cost.Sub(dd("0.13")).Abs().GreaterThan(dd("0.000001")) {
			t.Fatalf("costo %s por ml; quería 0.13 ($0.10 por g × 1.30)", cost)
		}
		if pres.Sub(dd("300")).Abs().GreaterThan(dd("0.01")) {
			t.Fatalf("la lata trae %s ml; quería 300 (390 g ÷ 1.30)", pres)
		}
		if tare == nil || density == nil {
			t.Fatalf("se perdió la tara (%v) o la densidad (%v)", tare, density)
		}
		var after string
		if err := k.st.Pool.QueryRow(owner, `select after->>'density' from audit_events where action = 'ingredient.unit_changed'
			and entity_id = $1 order by id desc limit 1`, leche).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if dd(after).Cmp(dd("1.3")) != 0 {
			t.Fatalf("la bitácora del cambio no dice la densidad: %q", after)
		}
	})

	t.Run("rehacer la pesada después no recalcula lo convertido", func(t *testing.T) {
		_, before := sumOf(t, k.st, leche)
		if _, err := k.svc.SetDensity(ctx, leche, app.DensityInput{Method: "manual", Density: decp("1.25")}); err != nil {
			t.Fatal(err)
		}
		if _, after := sumOf(t, k.st, leche); !after.Equal(before) {
			t.Fatalf("la existencia pasó de %s a %s", before, after)
		}
	})
}

// Con compras, el cambio reemplaza: el artículo nuevo nace con la densidad y los empaques con su tara.
func TestReplacingAnArticleBetweenGramsAndMlKeepsDensityAndTare(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	owner := context.Background()
	jarabe := ingredientIn(t, k.st, defaultCompanyID, "Jarabe reemplazo", "g")
	if _, err := k.st.Pool.Exec(owner, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, tare_g)
		values ($1, $2, 'Botella', 1300, 80)`, defaultCompanyID, jarabe); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(owner, `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity, unit_cost)
		values ($1, 'ingrediente', $2, 'compra', 2600, 0.05)`, defaultCompanyID, jarabe); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.SetDensity(ctx, jarabe, app.DensityInput{Method: "manual", Density: decp("1.3")}); err != nil {
		t.Fatal(err)
	}
	ml := unitID(t, k.st, "ml")
	prev, err := k.svc.PreviewUnitChange(ctx, jarabe, app.UnitChangeRequest{BaseUnitID: ml})
	if err != nil {
		t.Fatal(err)
	}
	if prev.Path != domain.UnitPathReplace {
		t.Fatalf("camino = %s; con compras debe reemplazar", prev.Path)
	}
	if err := k.svc.ChangeUnit(ctx, jarabe, app.UnitChangeRequest{BaseUnitID: ml}); err != nil {
		t.Fatal(err)
	}
	var newID int64
	if err := k.st.Pool.QueryRow(owner, `select new_ingredient_id from ingredient_replacements where old_ingredient_id = $1`, jarabe).Scan(&newID); err != nil {
		t.Fatal(err)
	}
	var density, tare *string
	if err := k.st.Pool.QueryRow(owner, `select i.density_g_per_ml::text, p.tare_g::text from ingredients i
		join ingredient_presentations p on p.ingredient_id = i.id where i.id = $1`, newID).Scan(&density, &tare); err != nil {
		t.Fatal(err)
	}
	if density == nil || tare == nil {
		t.Fatalf("el artículo nuevo nació sin densidad (%v) o su empaque sin tara (%v)", density, tare)
	}
	if got := onHandIngredient(t, k.st, newID); got.Sub(dd("2000")).Abs().GreaterThan(dd("0.01")) {
		t.Fatalf("el nuevo tiene %s ml; quería ≈ 2000 (2,600 g ÷ 1.30)", got)
	}
}
