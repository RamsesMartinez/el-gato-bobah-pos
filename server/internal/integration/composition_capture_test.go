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
	t.Parallel()
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
			app.CompositionRequest{Items: []app.CompositionInputItem{item(milk, "250", g), item(sugar, "5", g)}}, admin); err != nil {
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
		if err := svc.SaveComposition(tctx, app.CompositionOfOption, cokeExtra, app.CompositionRequest{LinkedProductID: &soda}, admin); err != nil {
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

	pkg := func(parts ...int64) app.CompositionRequest {
		r := app.CompositionRequest{}
		for i := 0; i < len(parts); i += 2 {
			r.Components = append(r.Components, app.CompositionComponent{ProductID: parts[i], Quantity: int(parts[i+1])})
		}
		return r
	}
	party := makeProduct(t, owner, "Paquete fiesta captura", decimal.RequireFromString("150"), false)

	t.Run("un producto se arma como paquete con sus productos y cantidades", func(t *testing.T) {
		if err := svc.SaveComposition(tctx, app.CompositionOfProduct, party, pkg(frappe, 1, soda, 2), admin); err != nil {
			t.Fatal(err)
		}
		v, err := svc.Composition(tctx, app.CompositionOfProduct, party)
		if err != nil {
			t.Fatal(err)
		}
		if len(v.Components) != 2 || v.Components[1].ProductName != "Coca captura" || v.Components[1].Quantity != 2 || v.Status != "confirmed" {
			t.Fatalf("paquete guardado: %+v", v)
		}
	})

	t.Run("un extra puede ser un paquete", func(t *testing.T) {
		if err := svc.SaveComposition(tctx, app.CompositionOfOption, cokeExtra, app.CompositionRequest{LinkedProductID: &party}, admin); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("un paquete que se captura con insumos deja de ser paquete", func(t *testing.T) {
		if err := svc.SaveComposition(tctx, app.CompositionOfProduct, combo, pkg(frappe, 1), admin); err != nil {
			t.Fatal(err)
		}
		if err := svc.SaveComposition(tctx, app.CompositionOfProduct, combo,
			app.CompositionRequest{Items: []app.CompositionInputItem{item(milk, "200", g)}}, admin); err != nil {
			t.Fatal(err)
		}
		v, err := svc.Composition(tctx, app.CompositionOfProduct, combo)
		if err != nil {
			t.Fatal(err)
		}
		var slots int
		if err := owner.Pool.QueryRow(ctx, `select count(*) from combo_slots where combo_id = $1`, combo).Scan(&slots); err != nil {
			t.Fatal(err)
		}
		if len(v.Components) != 0 || len(v.Items) != 1 || slots != 0 {
			t.Fatalf("quedan restos del paquete: %+v, %d huecos", v, slots)
		}
	})

	rejections := []struct {
		name string
		kind app.CompositionKind
		id   int64
		req  app.CompositionRequest
	}{
		{"insumo de otra empresa", app.CompositionOfProduct, frappe, app.CompositionRequest{Items: []app.CompositionInputItem{item(foreignIngredient, "1", g)}}},
		{"unidad de otro tipo", app.CompositionOfProduct, frappe, app.CompositionRequest{Items: []app.CompositionInputItem{item(milk, "1", ml)}}},
		{"cantidad que redondea a cero", app.CompositionOfProduct, frappe, app.CompositionRequest{Items: []app.CompositionInputItem{item(milk, "0.00003", g)}}},
		{"producto con existencias propias con insumos", app.CompositionOfProduct, soda, app.CompositionRequest{Items: []app.CompositionInputItem{item(milk, "1", g)}}},
		{"producto con existencias propias como paquete", app.CompositionOfProduct, soda, pkg(frappe, 1)},
		{"paquete dentro de un paquete", app.CompositionOfProduct, frappe, pkg(party, 1)},
		{"paquete que se lleva a sí mismo", app.CompositionOfProduct, party, pkg(party, 1)},
		{"producto que va en un paquete y se quiere volver paquete", app.CompositionOfProduct, frappe, pkg(soda, 1)},
		{"un producto ligado a otro producto", app.CompositionOfProduct, frappe, app.CompositionRequest{LinkedProductID: &soda}},
	}
	for _, c := range rejections {
		t.Run("rechaza "+c.name, func(t *testing.T) {
			if err := svc.SaveComposition(tctx, c.kind, c.id, c.req, admin); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("debe rechazarse como validación: %v", err)
			}
		})
	}

	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewAdminService(st)
		if _, err := svc.Composition(ctx, app.CompositionOfOption, extra); err == nil {
			t.Fatal("la composición de otra empresa se leyó")
		}
		if err := svc.SaveComposition(ctx, app.CompositionOfOption, extra, app.CompositionRequest{}, admin); err == nil {
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
