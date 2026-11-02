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

// DENSIDAD DE UN ARTÍCULO (spec 058, US1).

func decp(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

func weighing(full, empty, ml string) app.DensityInput {
	return app.DensityInput{Method: "weighing", FullG: decp(full), EmptyG: decp(empty), ContentMl: decp(ml)}
}

func densityOf(t *testing.T, st *store.Store, id int64) (value *string, method *string) {
	t.Helper()
	if err := st.Pool.QueryRow(context.Background(), `select density_g_per_ml::text, density_method from ingredients where id = $1`, id).
		Scan(&value, &method); err != nil {
		t.Fatal(err)
	}
	return value, method
}

func TestCapturingTheDensityOfAnArticle(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	user := makeUser(t, st, "pesadora", "admin")
	salsa := ingredientIn(t, st, defaultCompanyID, "Salsa de chocolate densidad", "ml")
	svc, base := asTenant(t, defaultCompanyID)
	ctx := app.WithActorUser(base, user)

	t.Run("con una pesada queda la densidad, quién y cómo, y la ficha la muestra", func(t *testing.T) {
		got, err := svc.SetDensity(ctx, salsa, weighing("1250", "150", "1000"))
		if err != nil {
			t.Fatal(err)
		}
		if !got.Value.Equal(decimal.RequireFromString("1.1")) || got.Method != "weighing" {
			t.Fatalf("= %+v; quería 1.1 por pesada", got)
		}
		sheet, err := svc.GetArticle(ctx, salsa)
		if err != nil {
			t.Fatal(err)
		}
		if sheet.Density == nil || !sheet.Density.Value.Equal(decimal.RequireFromString("1.1")) || sheet.Density.SetByName == "" {
			t.Fatalf("la ficha no trae la densidad con quién la puso: %+v", sheet.Density)
		}
		if n := auditCount(t, st, "ingredient.density_set", salsa); n != 1 {
			t.Fatalf("eventos de densidad = %d; quería 1", n)
		}
	})

	t.Run("con la etiqueta y escrita a mano también, y rehacerla deja los dos valores en la bitácora", func(t *testing.T) {
		if _, err := svc.SetDensity(ctx, salsa, app.DensityInput{Method: "label", NetG: decp("4300"), ContentMl: decp("3785")}); err != nil {
			t.Fatal(err)
		}
		if v, m := densityOf(t, st, salsa); v == nil || *v != "1.1361" || *m != "label" {
			t.Fatalf("de la etiqueta quedó %v por %v", v, m)
		}
		if _, err := svc.SetDensity(ctx, salsa, app.DensityInput{Method: "manual", Density: decp("1.08")}); err != nil {
			t.Fatal(err)
		}
		var before, after string
		if err := st.Pool.QueryRow(context.Background(), `select before->>'density', after->>'density' from audit_events
			where action = 'ingredient.density_set' and entity_id = $1 order by id desc limit 1`, salsa).Scan(&before, &after); err != nil {
			t.Fatal(err)
		}
		if before != "1.1361" || after != "1.08" {
			t.Fatalf("la bitácora dice %s → %s; quería 1.1361 → 1.08", before, after)
		}
	})

	t.Run("una pesada imposible o un cuerpo raro se rechazan sin tocar la densidad", func(t *testing.T) {
		bad := []app.DensityInput{
			weighing("150", "150", "1000"),
			weighing("1250", "150", "0"),
			weighing("1.25", "0.15", "1000"), // en kg: 0.0011 g/ml
			{Method: "manual", Density: decp("3.5")},
			{Method: "adivinada", Density: decp("1.1")},
			{Method: "manual"},
			{Method: "manual", Density: decp("1.1"), FullG: decp("10")},
		}
		for _, in := range bad {
			if _, err := svc.SetDensity(ctx, salsa, in); !errors.Is(err, domain.ErrValidation) {
				t.Errorf("%+v = %v; quería 400", in, err)
			}
		}
		if v, _ := densityOf(t, st, salsa); v == nil || *v != "1.0800" && *v != "1.08" {
			t.Fatalf("la densidad cambió a %v con una captura rechazada", v)
		}
	})

	t.Run("un artículo en piezas no lleva densidad, y uno archivado tampoco se edita", func(t *testing.T) {
		vasos := ingredientIn(t, st, defaultCompanyID, "Vasos densidad", "pieza")
		if _, err := svc.SetDensity(ctx, vasos, app.DensityInput{Method: "manual", Density: decp("1.1")}); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("en piezas = %v; quería 400", err)
		}
		viejo := ingredientIn(t, st, defaultCompanyID, "Jarabe archivado densidad", "ml")
		if _, err := st.Pool.Exec(context.Background(), `update ingredients set is_active = false where id = $1`, viejo); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.SetDensity(ctx, viejo, app.DensityInput{Method: "manual", Density: decp("1.1")}); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("archivado = %v; quería 409", err)
		}
	})

	t.Run("quitarla deja el evento; quitarla otra vez no hace nada", func(t *testing.T) {
		if err := svc.ClearDensity(ctx, salsa); err != nil {
			t.Fatal(err)
		}
		if v, _ := densityOf(t, st, salsa); v != nil {
			t.Fatalf("quedó %s", *v)
		}
		if err := svc.ClearDensity(ctx, salsa); err != nil {
			t.Fatalf("quitar sin densidad = %v; es idempotente", err)
		}
		if n := auditCount(t, st, "ingredient.density_cleared", salsa); n != 1 {
			t.Fatalf("eventos de quitar = %d; quería 1", n)
		}
	})

	t.Run("con una compra en kg por recibir no se deja quitar", func(t *testing.T) {
		jarabe := ingredientIn(t, st, defaultCompanyID, "Jarabe pendiente densidad", "ml")
		if _, err := svc.SetDensity(ctx, jarabe, app.DensityInput{Method: "manual", Density: decp("1.3")}); err != nil {
			t.Fatal(err)
		}
		_, item := purchaseIn(t, st, defaultCompanyID, user, jarabe)
		if _, err := st.Pool.Exec(context.Background(), `update expense_items set unit_id = (select id from units where code = 'kg') where id = $1`, item); err != nil {
			t.Fatal(err)
		}
		if err := svc.ClearDensity(ctx, jarabe); !errors.Is(err, domain.ErrDensityInUse) {
			t.Fatalf("quitar con la compra pendiente = %v; quería 409 DENSITY_IN_USE", err)
		}
		// Con la densidad ya fija en el renglón, quitarla no le estorba.
		if _, err := st.Pool.Exec(context.Background(), `update expense_items set density_g_per_ml = 1.3 where id = $1`, item); err != nil {
			t.Fatal(err)
		}
		if err := svc.ClearDensity(ctx, jarabe); err != nil {
			t.Fatalf("con la densidad fija en el renglón = %v", err)
		}
	})
}

func TestTheDensityOfAnotherCompanyIsInvisible(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	user := makeUser(t, st, "pesadora-ajena", "admin")
	other := makeCompany(t, st, "densidad-ajena")
	salsa := ingredientIn(t, st, defaultCompanyID, "Salsa tres casos", "ml")
	svc, base := asTenant(t, defaultCompanyID)
	if _, err := svc.SetDensity(app.WithActorUser(base, user), salsa, app.DensityInput{Method: "manual", Density: decp("1.1")}); err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, int64(defaultCompanyID), other, func(t *testing.T, st *store.Store, ctx context.Context) {
		s := app.NewBackofficeService(st, nil)
		if _, err := s.GetArticle(ctx, salsa); err == nil {
			t.Fatal("se leyó la ficha (con su densidad) de otra empresa")
		}
		if _, err := s.SetDensity(app.WithActorUser(ctx, user), salsa, app.DensityInput{Method: "manual", Density: decp("2")}); err == nil {
			t.Fatal("se cambió la densidad de otra empresa")
		}
		if err := s.ClearDensity(app.WithActorUser(ctx, user), salsa); err == nil {
			t.Fatal("se quitó la densidad de otra empresa")
		}
	})
	if v, _ := densityOf(t, st, salsa); v == nil || *v != "1.1000" && *v != "1.1" {
		t.Fatalf("la densidad quedó en %v", v)
	}
}
