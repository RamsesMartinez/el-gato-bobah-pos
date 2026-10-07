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

// CAPTURAR QUÉ LLEVA UN PRODUCTO O UN EXTRA (spec 028, historia 1), bajo el rol de la app.

func unitID(t *testing.T, st *store.Store, code string) int16 {
	t.Helper()
	var id int16
	if err := st.Pool.QueryRow(context.Background(), `select id from units where code = $1`, code).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCapturingACompositionUnderTheAppRole(t *testing.T) {
	owner := newTestStore(t)
	ctx := context.Background()
	admin := makeUser(t, owner, "admin_composicion", "admin")
	milk := makeIngredient(t, owner, "Leche deslactosada captura")
	sugar := makeIngredient(t, owner, "Azúcar captura")
	soda := makeProduct(t, owner, "Coca captura", decimal.RequireFromString("30"), true)
	frappe := makeProduct(t, owner, "Frappé captura", decimal.RequireFromString("70"), false)
	combo := makeProduct(t, owner, "Paquete captura", decimal.RequireFromString("90"), false)
	if _, err := owner.Pool.Exec(ctx, `update products set type = 'combo' where id = $1`, combo); err != nil {
		t.Fatal(err)
	}
	extra := opcionConTope(t, owner, "Extras captura", "Leche deslactosada", decimal.RequireFromString("10"), 1)
	cokeExtra := opcionConTope(t, owner, "Extras captura", "Coca del combo", decimal.RequireFromString("20"), 1)
	g, ml := unitID(t, owner, "g"), unitID(t, owner, "ml")

	other := makeCompany(t, owner, "empresa-otra-captura")
	var foreignIngredient int64
	if err := owner.Pool.QueryRow(ctx, `
		insert into ingredients (company_id, name, base_unit_id) select $1, 'Insumo ajeno', id from units where code = 'g' returning id`,
		other).Scan(&foreignIngredient); err != nil {
		t.Fatal(err)
	}

	st := appRoleStore(t)
	tctx, release, err := st.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	svc := app.NewAdminService(st)
	item := func(ing int64, qty string, unit int16) app.CompositionInputItem {
		return app.CompositionInputItem{IngredientID: ing, Quantity: decimal.RequireFromString(qty), UnitID: unit}
	}

	t.Run("dos insumos se guardan confirmados por quien los capturó", func(t *testing.T) {
		if err := svc.SaveComposition(tctx, app.CompositionOfOption, extra,
			[]app.CompositionInputItem{item(milk, "250", g), item(sugar, "5", g)}, nil, admin); err != nil {
			t.Fatal(err)
		}
		v, err := svc.Composition(tctx, app.CompositionOfOption, extra)
		if err != nil {
			t.Fatal(err)
		}
		if len(v.Items) != 2 || v.Status != "confirmed" || v.ConfirmedBy == "" || v.ConfirmedAt == nil {
			t.Fatalf("composición guardada: %+v", v)
		}
	})

	t.Run("un extra que es un producto del catálogo", func(t *testing.T) {
		if err := svc.SaveComposition(tctx, app.CompositionOfOption, cokeExtra, nil, &soda, admin); err != nil {
			t.Fatal(err)
		}
		v, err := svc.Composition(tctx, app.CompositionOfOption, cokeExtra)
		if err != nil {
			t.Fatal(err)
		}
		if v.LinkedProductID == nil || *v.LinkedProductID != soda || v.LinkedProductName != "Coca captura" {
			t.Fatalf("el extra debe quedar ligado a la Coca: %+v", v)
		}
	})

	t.Run("lo estimado se confirma una vez, con quién y cuándo", func(t *testing.T) {
		if _, err := owner.Pool.Exec(ctx, `
			update modifier_options set composition_status = 'estimated', composition_confirmed_by = null,
			       composition_confirmed_at = null where id = $1`, extra); err != nil {
			t.Fatal(err)
		}
		if v, _ := svc.Composition(tctx, app.CompositionOfOption, extra); v.Status != "estimated" {
			t.Fatalf("debe decir estimada: %+v", v)
		}
		if err := svc.ConfirmComposition(tctx, app.CompositionOfOption, extra, admin); err != nil {
			t.Fatal(err)
		}
		if v, _ := svc.Composition(tctx, app.CompositionOfOption, extra); v.Status != "confirmed" || v.ConfirmedBy == "" {
			t.Fatalf("debe quedar confirmada con quién: %+v", v)
		}
		if err := svc.ConfirmComposition(tctx, app.CompositionOfOption, extra, admin); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("confirmar lo ya confirmado se dice, no se ignora: %v", err)
		}
	})

	t.Run("lo que no lleva nada se confirma así y sale de «sin capturar»", func(t *testing.T) {
		noIce := opcionConTope(t, owner, "Extras captura", "Sin hielo captura", decimal.Zero, 1)
		pending := func() int {
			page, err := svc.ListModifierOptions(tctx, "", "Sin hielo captura", "none", 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			return len(page.Items)
		}
		if pending() != 1 {
			t.Fatal("un extra sin composición sale en «sin capturar»")
		}
		if err := svc.ConfirmComposition(tctx, app.CompositionOfOption, noIce, admin); err != nil {
			t.Fatal(err)
		}
		if pending() != 0 {
			t.Fatal("confirmado sin composición ya no está pendiente")
		}
	})

	rejections := []struct {
		name   string
		kind   app.CompositionKind
		id     int64
		items  []app.CompositionInputItem
		linked *int64
	}{
		{"insumo de otra empresa", app.CompositionOfProduct, frappe, []app.CompositionInputItem{item(foreignIngredient, "1", g)}, nil},
		{"unidad de otro tipo", app.CompositionOfProduct, frappe, []app.CompositionInputItem{item(milk, "1", ml)}, nil},
		{"extra ligado a un paquete", app.CompositionOfOption, cokeExtra, nil, &combo},
		{"producto con existencias propias", app.CompositionOfProduct, soda, []app.CompositionInputItem{item(milk, "1", g)}, nil},
		{"paquete con insumos", app.CompositionOfProduct, combo, []app.CompositionInputItem{item(milk, "1", g)}, nil},
	}
	for _, c := range rejections {
		t.Run("rechaza "+c.name, func(t *testing.T) {
			if err := svc.SaveComposition(tctx, c.kind, c.id, c.items, c.linked, admin); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("debe rechazarse como validación: %v", err)
			}
		})
	}

	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewAdminService(st)
		if _, err := svc.Composition(ctx, app.CompositionOfOption, extra); err == nil {
			t.Fatal("la composición de otra empresa se leyó")
		}
		if err := svc.SaveComposition(ctx, app.CompositionOfOption, extra, nil, nil, admin); err == nil {
			t.Fatal("la composición de otra empresa se borró")
		}
		if err := svc.ConfirmComposition(ctx, app.CompositionOfOption, extra, admin); err == nil {
			t.Fatal("la composición de otra empresa se confirmó")
		}
	})
	var still int
	if err := owner.Pool.QueryRow(ctx,
		`select count(*) from modifier_options where id = $1 and recipe_id is not null`, extra).Scan(&still); err != nil || still != 1 {
		t.Fatalf("la composición de la empresa dueña debe seguir ahí: %d, %v", still, err)
	}
}
