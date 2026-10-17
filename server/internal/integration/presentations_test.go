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

// EMPAQUES POR INSUMO (spec 038, historia 2): «Caja = 12 L» se captura con número y unidad, se
// guarda en la unidad del insumo, y desactivarlo no reescribe compras pasadas.

// asTenant devuelve el servicio de almacén bajo el rol de la app con la empresa fijada.
func asTenant(t *testing.T, company int64) (*app.BackofficeService, context.Context) {
	t.Helper()
	as := appRoleStore(t)
	ctx, release, err := as.AcquireTenant(context.Background(), company)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	return app.NewBackofficeService(as, nil), ctx
}

func TestPresentationsAreCreatedInTheIngredientUnit(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	leche := ingredientIn(t, st, defaultCompanyID, "Leche empaque", "ml")
	svc, ctx := asTenant(t, defaultCompanyID)

	caja, err := svc.CreatePresentation(ctx, leche, app.PresentationInput{Name: "Caja", Qty: decimal.RequireFromString("12"), UnitID: unitID(t, st, "l")})
	if err != nil {
		t.Fatalf("crear «Caja = 12 L»: %v", err)
	}
	if !caja.QtyInBase.Equal(decimal.NewFromInt(12000)) {
		t.Errorf("la caja trae %s ml; quiere 12000 (12 L en la unidad del insumo)", caja.QtyInBase)
	}

	// Mismo nombre con otras mayúsculas y espacios: 409, no un segundo «caja».
	if _, err := svc.CreatePresentation(ctx, leche, app.PresentationInput{Name: "  CAJA ", Qty: decimal.RequireFromString("6"), UnitID: unitID(t, st, "l")}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("nombre repetido = %v, quiere ErrConflict", err)
	}
	// «25» en kilos para algo que se mide en mililitros: se rechaza, no se convierte en silencio.
	if _, err := svc.CreatePresentation(ctx, leche, app.PresentationInput{Name: "Bulto", Qty: decimal.RequireFromString("25"), UnitID: unitID(t, st, "kg")}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("kilos de algo que se mide en ml = %v, quiere ErrValidation", err)
	}

	// Desactivarla la saca de lo que se ofrece, pero sigue en la lista; y libera el nombre.
	off := false
	if _, err := svc.UpdatePresentation(ctx, caja.ID, app.PresentationInput{Name: "Caja", Qty: decimal.RequireFromString("12"), UnitID: unitID(t, st, "l"), Active: &off}); err != nil {
		t.Fatalf("desactivar: %v", err)
	}
	list, err := svc.Presentations(ctx, &leche)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Active {
		t.Fatalf("lista tras desactivar = %+v; quiere la caja inactiva", list)
	}
	if _, err := svc.CreatePresentation(ctx, leche, app.PresentationInput{Name: "Caja", Qty: decimal.RequireFromString("10"), UnitID: unitID(t, st, "l")}); err != nil {
		t.Fatalf("una caja nueva tras desactivar la vieja: %v", err)
	}
}

func TestApproximatePresentationAndWholePieces(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	aguacate := ingredientIn(t, st, defaultCompanyID, "Aguacate empaque", "pieza")
	svc, ctx := asTenant(t, defaultCompanyID)

	if _, err := svc.CreatePresentation(ctx, aguacate, app.PresentationInput{Name: "Bolsa", Qty: decimal.RequireFromString("12.5"), UnitID: unitID(t, st, "pieza")}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("12.5 piezas = %v, quiere ErrValidation (regla 16)", err)
	}
	kilo, err := svc.CreatePresentation(ctx, aguacate, app.PresentationInput{Name: "Kilo", Qty: decimal.RequireFromString("4"), UnitID: unitID(t, st, "kg"), Approximate: true})
	if err != nil {
		t.Fatalf("empaque aproximado «Kilo ≈ 4 piezas»: %v", err)
	}
	if !kilo.Approximate || !kilo.QtyInBase.Equal(decimal.NewFromInt(4)) {
		t.Errorf("aproximado = %+v", kilo)
	}
}

// Los empaques de una empresa no se ven ni se tocan desde otra, en los tres casos donde RLS ya
// falló en este proyecto.
func TestPresentationsAreIsolated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	other := makeCompany(t, st, "empaques-otra")
	leche := ingredientIn(t, st, defaultCompanyID, "Leche aislada", "ml")
	var id int64
	if err := st.Pool.QueryRow(context.Background(), `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base)
		values ($1, $2, 'Caja', 12000) returning id`, defaultCompanyID, leche).Scan(&id); err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		svc := app.NewBackofficeService(as, nil)
		list, err := svc.Presentations(ctx, nil)
		if err != nil {
			t.Fatalf("listar: %v", err)
		}
		if len(list) != 0 {
			t.Fatalf("vio %d empaques de otra empresa", len(list))
		}
		off := false
		if _, err := svc.UpdatePresentation(ctx, id, app.PresentationInput{Name: "Robada", Qty: decimal.NewFromInt(1), UnitID: unitID(t, st, "l"), Active: &off}); err == nil {
			t.Fatal("editó un empaque de otra empresa")
		}
		if _, err := svc.CreatePresentation(ctx, leche, app.PresentationInput{Name: "Ajena", Qty: decimal.NewFromInt(1), UnitID: unitID(t, st, "l")}); err == nil {
			t.Fatal("creó un empaque sobre un insumo de otra empresa")
		}
	})
}

// Revisión de código del 038: editar el nombre o el contenido de un empaque que ya no llega, sin
// volver a mandar su estado, lo reactivaba; volvía a ofrecerse en la captura sin que nadie lo pidiera.
func TestEditingARetiredPresentationKeepsItRetired(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	leche := ingredientIn(t, st, defaultCompanyID, "Leche retirada", "ml")
	svc, ctx := asTenant(t, defaultCompanyID)
	l := unitID(t, st, "l")
	caja, err := svc.CreatePresentation(ctx, leche, app.PresentationInput{Name: "Caja", Qty: decimal.RequireFromString("12"), UnitID: l})
	if err != nil {
		t.Fatal(err)
	}
	off := false
	if _, err := svc.UpdatePresentation(ctx, caja.ID, app.PresentationInput{Name: "Caja", Qty: decimal.RequireFromString("12"), UnitID: l, Active: &off}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.UpdatePresentation(ctx, caja.ID, app.PresentationInput{Name: "Caja grande", Qty: decimal.RequireFromString("12"), UnitID: l})
	if err != nil {
		t.Fatal(err)
	}
	if got.Active {
		t.Error("editar el nombre reactivó un empaque que ya no llega")
	}
}
