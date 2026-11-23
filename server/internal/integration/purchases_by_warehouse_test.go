//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// COMPRAS CON DESTINO (spec 094, historia 4): cada renglón dice a qué almacén entra.

// Un ticket de 8 renglones, 2 a «Bodega» y 1 que no es del negocio: el local sube por 5, la bodega por
// 2 y el excluido no entra a ninguno.
func TestAPurchaseSplitsBetweenWarehouses(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	local, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	g := unitID(t, k.st, "g")
	var ings []int64
	var items []app.ExpenseItemInput
	for i := range 8 {
		ing := ingredientIn(t, k.st, defaultCompanyID, fmt.Sprintf("Insumo del ticket %d", i), "g")
		ings = append(ings, ing)
		q := dd("1000")
		it := app.ExpenseItemInput{ItemType: "ingrediente", IngredientID: &ing, Description: fmt.Sprintf("Renglón %d", i),
			Quantity: q, UnitID: &g, QtyReceived: &q, Amount: dd("10")}
		switch i {
		case 5, 6:
			it.WarehouseID = &bodega
		case 7:
			it.Excluded = true
		}
		items = append(items, it)
	}
	if _, err := k.svc.CreateExpense(k.ctx, app.ExpenseInput{ReceivedAt: "2026-07-20", CategoryID: k.category, Amount: dd("70"),
		Status: domain.ExpensePendiente, Description: "Ticket repartido", UserID: k.user, Items: items}); err != nil {
		t.Fatal(err)
	}
	for i, ing := range ings {
		inLocal := onHandIn(t, k.st, local, "ingredient_id", ing)
		inBodega := onHandIn(t, k.st, bodega, "ingredient_id", ing)
		switch {
		case i < 5 && (!inLocal.Equal(dd("1000")) || !inBodega.IsZero()):
			t.Errorf("renglón %d (al local): local %s, bodega %s", i, inLocal, inBodega)
		case (i == 5 || i == 6) && (!inBodega.Equal(dd("1000")) || !inLocal.IsZero()):
			t.Errorf("renglón %d (a la bodega): local %s, bodega %s", i, inLocal, inBodega)
		case i == 7 && (!inLocal.IsZero() || !inBodega.IsZero()):
			t.Errorf("el excluido entró: local %s, bodega %s", inLocal, inBodega)
		}
	}
}

// Cambiar el destino de un renglón ya recibido saca del almacén anterior y mete al nuevo, por
// diferencia: lo aplicado del renglón no cambia y ningún movimiento se edita.
func TestChangingTheDestinationMovesWhatArrived(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	local, bodega := comoEnLaSegundaSubida(t, k.st, defaultCompanyID)
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("2", "1000"))
	if err != nil {
		t.Fatal(err)
	}
	item, applied := k.itemOf(t, id)
	line := app.StockLineInput{ID: &item, SeenApplied: &applied, ItemType: "ingrediente", IngredientID: &k.azucar,
		Quantity: dd("2"), Received: dd("2"), PresentationID: &k.bulto, Amount: dd("1000"), Description: "Azúcar",
		WarehouseID: &bodega}
	if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{line}, k.user); err != nil {
		t.Fatalf("cambiar el destino a la bodega: %v", err)
	}
	if v := onHandIn(t, k.st, local, "ingredient_id", k.azucar); !v.IsZero() {
		t.Fatalf("el local quedó con %s: lo recibido no salió", v)
	}
	if v := onHandIn(t, k.st, bodega, "ingredient_id", k.azucar); !v.Equal(dd("50000")) {
		t.Fatalf("la bodega quedó con %s; quiere 50,000 g", v)
	}
	if _, after := k.itemOf(t, id); !after.Equal(applied) {
		t.Fatalf("lo aplicado del renglón cambió de %s a %s al moverlo de almacén", applied, after)
	}
	// Quitar el renglón lo saca de donde está ahora, no del local.
	other := app.StockLineInput{ItemType: "ingrediente", IngredientID: &k.azucar, Quantity: dd("1"), Received: dd("0"),
		PresentationID: &k.bulto, Amount: dd("1000"), Description: "Otro"}
	if _, err := k.svc.SaveStockLines(k.ctx, id, []app.StockLineInput{other}, k.user); err != nil {
		t.Fatalf("quitar el renglón: %v", err)
	}
	if v := onHandIn(t, k.st, bodega, "ingredient_id", k.azucar); !v.IsZero() {
		t.Fatalf("la bodega quedó con %s después de quitar el renglón", v)
	}
	if v := onHandIn(t, k.st, local, "ingredient_id", k.azucar); !v.IsZero() {
		t.Fatalf("el local quedó con %s después de quitar el renglón", v)
	}
}

// Un almacén de otra empresa, de otra sucursal o archivado no recibe compras.
func TestAPurchaseToAForeignWarehouseIsRejected(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	other := makeCompany(t, k.st, "compra-a-almacen-ajeno")
	var foreign int64
	if err := k.st.Pool.QueryRow(context.Background(), `select id from warehouses where company_id = $1`, other).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	in := k.bultos("1", "500")
	in.Items[0].WarehouseID = &foreign
	if _, err := k.svc.CreateExpense(k.ctx, in); !errors.Is(err, domain.ErrWarehouseNotFound) {
		t.Fatalf("comprar al almacén de otra empresa = %v; quiere «no existe»", err)
	}
}
