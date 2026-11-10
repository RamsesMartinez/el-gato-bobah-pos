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

// LA VISTA PREVIA DE UNA COMPRA (spec 073, R2): corre la captura completa y la revierte. Lo que se
// prueba es que no deja NADA (un renglón, un movimiento, un costo, el aprendizaje del proveedor o un
// evento serían una compra que nadie confirmó) y que el costo que anuncia es el que la compra deja.
func TestPurchasePreviewLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	ctx := app.WithActorUser(k.ctx, k.user)
	if _, err := k.svc.CreateExpense(ctx, k.bultos("1", "1000")); err != nil { // 25 kg a $0.04
		t.Fatal(err)
	}
	count := func(sql string) int {
		var n int
		if err := k.st.Pool.QueryRow(context.Background(), sql).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	snapshot := func() [5]int {
		return [5]int{count(`select count(*) from expenses`), count(`select count(*) from expense_items`),
			count(`select count(*) from stock_movements`), count(`select count(*) from audit_events`),
			count(`select count(*) from supplier_items`)}
	}
	before, costBefore, onHand := snapshot(), k.ingredientCost(t), k.onHand(t)

	// Dos renglones del mismo insumo: el costo es por insumo y el de la compra completa.
	in := k.bultos("1", "1500") // $0.06 el gramo
	second := in.Items[0]
	second.Amount = dd("2500") // $0.10 el gramo
	in.Items = append(in.Items, second)
	in.Amount = dd("4000")
	in.ReceivedAt = "2026-07-21"
	got, err := k.svc.PreviewPurchase(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if after := snapshot(); after != before {
		t.Fatalf("la vista previa dejó rastro: antes %v, después %v", before, after)
	}
	if !k.ingredientCost(t).Equal(costBefore) || !k.onHand(t).Equal(onHand) {
		t.Fatal("la vista previa movió el costo o la existencia")
	}
	if len(got.Lines) != 2 || len(got.Costs) != 1 {
		t.Fatalf("dos renglones y un insumo: %+v", got)
	}
	c := got.Costs[0]
	if !c.CostBefore.Equal(dd("0.04")) || !c.CostAfter.Equal(dd("0.1")) || c.KeptByNewerPurchase {
		t.Fatalf("último precio de la compra completa: antes %s, después %s", c.CostBefore, c.CostAfter)
	}
	if !got.Lines[0].UnitCost.Equal(dd("0.06")) || !got.Lines[1].UnitCost.Equal(dd("0.1")) {
		t.Fatalf("costo por renglón: %s, %s", got.Lines[0].UnitCost, got.Lines[1].UnitCost)
	}

	// Lo que anuncia es lo que deja: la misma compra, aplicada.
	if _, err := k.svc.CreateExpense(ctx, in); err != nil {
		t.Fatal(err)
	}
	if !k.ingredientCost(t).Equal(c.CostAfter) {
		t.Fatalf("la compra dejó %s y la vista previa anunció %s", k.ingredientCost(t), c.CostAfter)
	}

	t.Run("una compra capturada tarde no pone el último precio y lo dice", func(t *testing.T) {
		late := k.bultos("1", "5000")
		late.ReceivedAt = "2026-07-01"
		p, err := k.svc.PreviewPurchase(ctx, late)
		if err != nil {
			t.Fatal(err)
		}
		if !p.Costs[0].KeptByNewerPurchase || !p.Costs[0].CostAfter.Equal(p.Costs[0].CostBefore) {
			t.Fatalf("compra tardía: %+v", p.Costs[0])
		}
	})

	t.Run("no da de alta artículos", func(t *testing.T) {
		in := k.bultos("1", "100")
		in.Items[0].IngredientID, in.Items[0].PresentationID = nil, nil
		in.Items[0].NewArticle = &app.NewArticle{Name: "Insumo de vista previa", BaseUnitID: unitID(t, k.st, "g")}
		if _, err := k.svc.PreviewPurchase(ctx, in); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("un artículo nuevo en la vista previa: %v", err)
		}
	})

	t.Run("sin mercancía: arreglos vacíos, no null", func(t *testing.T) {
		p, err := k.svc.PreviewPurchase(ctx, app.ExpenseInput{CategoryID: k.category, Amount: dd("50"),
			Status: domain.ExpensePendiente, UserID: k.user})
		if err != nil || p.Lines == nil || p.Costs == nil {
			t.Fatalf("%+v %v", p, err)
		}
	})
}

// Un insumo unido a otro falla en la vista previa igual que al guardar (revisión A1): una vista previa
// que dijera «bien» para una compra que el commit rechaza es peor que no tenerla.
func TestPurchasePreviewOfAMergedIngredientFailsLikeTheSave(t *testing.T) {
	t.Parallel()
	mk := newMergeKit(t)
	mk.merge(t, mk.a, mk.req(mk.b))
	var cat int64
	if err := mk.st.Pool.QueryRow(context.Background(), `insert into expense_categories (company_id, name, financial_group)
		values ($1, 'Compras unidas', 'operacional') returning id`, defaultCompanyID).Scan(&cat); err != nil {
		t.Fatal(err)
	}
	q := dd("100")
	in := app.ExpenseInput{ReceivedAt: "2026-07-20", CategoryID: cat, Amount: dd("10"), Status: domain.ExpensePendiente,
		UserID: mk.admin, Items: []app.ExpenseItemInput{{ItemType: "ingrediente", IngredientID: &mk.b, Description: "Leche B",
			Quantity: q, UnitID: ptrUnit(unitID(t, mk.st, "g")), QtyReceived: &q, Amount: dd("10")}}}
	_, saveErr := mk.svc.CreateExpense(mk.ctx, in)
	_, prevErr := mk.svc.PreviewPurchase(mk.ctx, in)
	if saveErr == nil || prevErr == nil {
		t.Fatalf("comprar un insumo unido: al guardar %v, en la vista previa %v", saveErr, prevErr)
	}
	if errors.Is(saveErr, domain.ErrConflict) != errors.Is(prevErr, domain.ErrConflict) {
		t.Fatalf("la vista previa y el guardado no dicen lo mismo: %v / %v", prevErr, saveErr)
	}
}

// Aislamiento de la vista previa: lee insumos, existencias, costo y empaques de la empresa.
func TestPurchasePreviewIsolation(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	other := makeCompany(t, k.st, "compra-ajena")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		_, err := app.NewBackofficeService(st, nil).PreviewPurchase(ctx, k.bultos("1", "100"))
		if err == nil {
			t.Fatal("la vista previa leyó el insumo de otra empresa")
		}
	})
	_ = decimal.Zero
}
