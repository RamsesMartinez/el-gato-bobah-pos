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

// COMPRAS ENTRE PESO Y LÍQUIDO (spec 058, US2). El galón de salsa dice 4.3 kg y se guarda en ml.

type densityPurchaseKit struct {
	*purchaseKit
	salsa int64
	kg    int16
}

func newDensityPurchaseKit(t *testing.T) *densityPurchaseKit {
	t.Helper()
	k := &densityPurchaseKit{purchaseKit: newPurchaseKit(t)}
	k.ctx = app.WithActorUser(k.ctx, k.user)
	k.salsa = ingredientIn(t, k.st, defaultCompanyID, "Salsa galón", "ml")
	k.kg = unitID(t, k.st, "kg")
	return k
}

func (k *densityPurchaseKit) setDensity(t *testing.T, ing int64, d string) {
	t.Helper()
	if _, err := k.svc.SetDensity(k.ctx, ing, app.DensityInput{Method: "manual", Density: decp(d)}); err != nil {
		t.Fatal(err)
	}
}

// galon es un gasto de `kilos` kg de salsa; received nil = todavía no llega.
func (k *densityPurchaseKit) galon(kilos string, received *decimal.Decimal) app.ExpenseInput {
	in := app.ExpenseInput{
		CategoryID: k.category, Amount: dd("430"), Status: domain.ExpensePendiente, Description: "Salsas", UserID: k.user,
		Items: []app.ExpenseItemInput{{
			ItemType: "ingrediente", IngredientID: &k.salsa, Description: "Salsa chocolate galón", Quantity: dd(kilos),
			UnitID: &k.kg, QtyReceived: received, Amount: dd("430"),
		}},
	}
	if received != nil {
		in.ReceivedAt = "2026-07-20"
	}
	return in
}

func fixedDensity(t *testing.T, k *densityPurchaseKit, item int64) *string {
	t.Helper()
	var v *string
	if err := k.st.Pool.QueryRow(context.Background(), `select density_g_per_ml::text from expense_items where id = $1`, item).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestABuyInKilosOfSomethingKeptInMlConvertsWithItsDensity(t *testing.T) {
	t.Parallel()
	k := newDensityPurchaseKit(t)

	t.Run("sin densidad se rechaza nombrando el artículo", func(t *testing.T) {
		four := dd("4.3")
		_, err := k.svc.CreateExpense(k.ctx, k.galon("4.3", &four))
		var mismatch domain.UnitKindMismatch
		if !errors.As(err, &mismatch) || mismatch.IngredientID != k.salsa {
			t.Fatalf("= %v; quería UNIT_KIND_MISMATCH con el artículo %d", err, k.salsa)
		}
	})

	k.setDensity(t, k.salsa, "1.1")
	four := dd("4.3")
	id, err := k.svc.CreateExpense(k.ctx, k.galon("4.3", &four))
	if err != nil {
		t.Fatal(err)
	}
	item, _ := k.itemOf(t, id)

	t.Run("entra 4,300 g ÷ 1.10 = 3,909.0909 ml y el costo es por ml", func(t *testing.T) {
		if got := onHandIngredient(t, k.st, k.salsa); !got.Equal(dd("3909.0909")) {
			t.Fatalf("existencia = %s ml; quería 3909.0909", got)
		}
		var cost decimal.Decimal
		if err := k.st.Pool.QueryRow(context.Background(), `select current_cost from ingredients where id = $1`, k.salsa).Scan(&cost); err != nil {
			t.Fatal(err)
		}
		if !cost.Equal(dd("0.11")) {
			t.Fatalf("costo = %s por ml; quería 0.11 ($430 ÷ 3,909.0909 ml)", cost)
		}
		if v := fixedDensity(t, k, item); v == nil || dd(*v).Cmp(dd("1.1")) != 0 {
			t.Fatalf("el renglón no fijó la densidad con que entró: %v", v)
		}
	})

	t.Run("rehacer la pesada no mueve lo recibido, y corregirlo después usa la densidad del renglón", func(t *testing.T) {
		k.setDensity(t, k.salsa, "1.08")
		if got := onHandIngredient(t, k.st, k.salsa); !got.Equal(dd("3909.0909")) {
			t.Fatalf("rehacer la pesada movió la existencia a %s", got)
		}
		item, applied := k.itemOf(t, id)
		fixed := app.StockLineInput{ID: &item, SeenApplied: &applied, ItemType: "ingrediente", IngredientID: &k.salsa,
			Quantity: dd("4.3"), Received: dd("4"), UnitID: &k.kg, Amount: dd("430"), Description: "Salsa chocolate galón"}
		if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{fixed}, k.user); err != nil {
			t.Fatalf("corregir lo recibido: %v", err)
		}
		// 4,000 g ÷ 1.10 = 3,636.3636: bajó exactamente 300 g ÷ 1.10, no con la densidad nueva.
		if got := onHandIngredient(t, k.st, k.salsa); !got.Equal(dd("3636.3636")) {
			t.Fatalf("existencia = %s; quería 3636.3636 (con 1.08 sería 3703.7037)", got)
		}
	})

	t.Run("anular devuelve exactamente lo que entró", func(t *testing.T) {
		if err := k.svc.CancelExpense(k.ctx, id, "", k.user); err != nil {
			t.Fatal(err)
		}
		if got := onHandIngredient(t, k.st, k.salsa); !got.IsZero() {
			t.Fatalf("tras anular quedan %s ml", got)
		}
	})
}

func TestALineNotYetReceivedUsesTheDensityInForceWhenItArrives(t *testing.T) {
	t.Parallel()
	k := newDensityPurchaseKit(t)
	k.setDensity(t, k.salsa, "1.1")
	id, err := k.svc.CreateExpense(k.ctx, k.galon("4.3", nil))
	if err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, id)
	if v := fixedDensity(t, k, item); v != nil {
		t.Fatalf("un renglón que no ha sumado nada fijó %s", *v)
	}
	k.setDensity(t, k.salsa, "1.25")
	arrived := app.StockLineInput{ID: &item, SeenApplied: &applied, ItemType: "ingrediente", IngredientID: &k.salsa,
		Quantity: dd("4.3"), Received: dd("4.3"), UnitID: &k.kg, Amount: dd("430"), Description: "Salsa chocolate galón"}
	if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{arrived}, k.user); err != nil {
		t.Fatal(err)
	}
	if got := onHandIngredient(t, k.st, k.salsa); !got.Equal(dd("3440")) {
		t.Fatalf("existencia = %s; quería 3440 (4,300 g ÷ 1.25)", got)
	}
	if v := fixedDensity(t, k, item); v == nil || dd(*v).Cmp(dd("1.25")) != 0 {
		t.Fatalf("al llegar debió fijar 1.25; quedó %v", v)
	}
}

func TestChangingTheArticleOfALineForgetsItsFixedDensity(t *testing.T) {
	t.Parallel()
	k := newDensityPurchaseKit(t)
	k.setDensity(t, k.salsa, "1.1")
	miel := ingredientIn(t, k.st, defaultCompanyID, "Miel galón", "ml")
	k.setDensity(t, miel, "1.4")
	four := dd("4.2")
	id, err := k.svc.CreateExpense(k.ctx, k.galon("4.2", &four))
	if err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, id)
	fixed := app.StockLineInput{ID: &item, SeenApplied: &applied, ItemType: "ingrediente", IngredientID: &miel,
		Quantity: dd("4.2"), Received: dd("4.2"), UnitID: &k.kg, Amount: dd("430"), Description: "Era miel"}
	if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{fixed}, k.user); err != nil {
		t.Fatal(err)
	}
	if got := onHandIngredient(t, k.st, miel); !got.Equal(dd("3000")) {
		t.Fatalf("la miel tiene %s ml; quería 3000 (4,200 g ÷ 1.4, su densidad y no la de la salsa)", got)
	}
	if got := onHandIngredient(t, k.st, k.salsa); !got.IsZero() {
		t.Fatalf("a la salsa le quedaron %s ml", got)
	}
}

func TestEditingAnExpenseSeesTheDensityInItsFingerprint(t *testing.T) {
	t.Parallel()
	k := newDensityPurchaseKit(t)
	k.setDensity(t, k.salsa, "1.1")
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("1", "500"))
	if err != nil {
		t.Fatal(err)
	}
	var day string
	if err := k.st.Pool.QueryRow(context.Background(), `select expense_date::text from expenses where id = $1`, id).Scan(&day); err != nil {
		t.Fatal(err)
	}
	in := app.EditInput{CanReceive: true, Add: []app.EditLineInput{{
		ItemType: "ingrediente", IngredientID: &k.salsa, Description: "Salsa extra", Quantity: dd("2.2"), UnitID: &k.kg,
		Amount: dd("100"), ArrivedOn: day,
	}}}
	pre, err := k.svc.EditExpense(k.ctx, id, in, k.user, true)
	if err != nil {
		t.Fatal(err)
	}
	k.setDensity(t, k.salsa, "1.0")
	in.Fingerprint = pre.Preview.Fingerprint
	if _, err := k.svc.EditExpense(k.ctx, id, in, k.user, false); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("confirmar con otra densidad que la de la vista previa = %v; quería 409", err)
	}
	if got := onHandIngredient(t, k.st, k.salsa); !got.IsZero() {
		t.Fatalf("la confirmación rechazada movió la salsa a %s", got)
	}
}
