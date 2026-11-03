//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// #182: un artículo que se guarda por pieza y llega en «Caja de 12 latas». Capturar 1 caja sumaba 1
// pieza y dejaba el costo en el precio de la caja por pieza: el atajo «pieza → pieza» de BaseQty le
// ganaba a la presentación. Cada camino que recibe mercancía lo prueba por su cuenta, porque todos
// convierten con la misma regla y uno que se quede atrás vuelve a sumar cajas como piezas.
func TestAPresentationOnAPieceArticleAddsWhatItHolds(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	ctx := context.Background()
	// Una caja de 12 a $248: 12 piezas a $20.666667 cada una (la escala de unit_cost).
	wantCost := domain.UnitCost(dd("248"), dd("12"))

	refresco := func(t *testing.T, name string) (ing, caja int64) {
		t.Helper()
		ing = ingredientIn(t, k.st, defaultCompanyID, name, "pieza")
		if err := k.st.Pool.QueryRow(ctx, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base)
			values ($1, $2, 'Caja de 12 latas', 12) returning id`, defaultCompanyID, ing).Scan(&caja); err != nil {
			t.Fatal(err)
		}
		return ing, caja
	}
	check := func(t *testing.T, ing int64) {
		t.Helper()
		if got := onHandIngredient(t, k.st, ing); !got.Equal(dd("12")) {
			t.Fatalf("1 caja de 12 sumó %s piezas, quiere 12", got)
		}
		var cost decimal.Decimal
		if err := k.st.Pool.QueryRow(ctx, `select current_cost from ingredients where id = $1`, ing).Scan(&cost); err != nil {
			t.Fatal(err)
		}
		if !cost.Equal(wantCost) {
			t.Fatalf("costo por pieza %s, quiere %s (el importe de la caja entre 12)", cost, wantCost)
		}
	}

	t.Run("crear el gasto ya recibido", func(t *testing.T) {
		ing, caja := refresco(t, "Refresco 182 crear")
		one := dd("1")
		if _, err := k.svc.CreateExpense(k.ctx, app.ExpenseInput{
			ReceivedAt: k.day(0), CategoryID: k.category, Amount: dd("248"), Status: domain.ExpensePendiente,
			Description: "Refrescos", UserID: k.user,
			Items: []app.ExpenseItemInput{{ItemType: "ingrediente", IngredientID: &ing, Description: "Refresco",
				Quantity: one, PresentationID: &caja, QtyReceived: &one, Amount: dd("248")}},
		}); err != nil {
			t.Fatal(err)
		}
		check(t, ing)
	})

	t.Run("editar un gasto con «Ya llegó»", func(t *testing.T) {
		ing, caja := refresco(t, "Refresco 182 editar")
		id := k.paidWithoutGoods(t, "248", k.day(-1))
		res := k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{{ItemType: "ingrediente", IngredientID: &ing,
			Description: "Refresco", Quantity: dd("1"), PresentationID: &caja, Amount: dd("248"), ArrivedOn: k.day(0)}}}, nil)
		// La vista previa es la misma transacción: si ahí sube 1, la pantalla dice 1 aunque se arregle abajo.
		if len(res.Preview.Inventory) != 1 || !res.Preview.Inventory[0].After.Sub(res.Preview.Inventory[0].Before).Equal(dd("12")) {
			t.Fatalf("la vista previa no mostró las 12 piezas entrando: %+v", res.Preview.Inventory)
		}
		check(t, ing)
	})

	t.Run("agregar por recibir y registrar la entrega", func(t *testing.T) {
		ing, caja := refresco(t, "Refresco 182 entrega")
		id := k.paidWithoutGoods(t, "248", k.day(-1))
		k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{{ItemType: "ingrediente", IngredientID: &ing,
			Description: "Refresco", Quantity: dd("1"), PresentationID: &caja, Amount: dd("248")}}}, nil)
		k.confirmDelivery(t, id, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: k.items(t, id)["Refresco"], Qty: dd("1")}}}, nil)
		check(t, ing)
	})

	t.Run("recibir desde «Por recibir»", func(t *testing.T) {
		ing, caja := refresco(t, "Refresco 182 por recibir")
		id, err := k.svc.CreateExpense(k.ctx, app.ExpenseInput{
			ExpenseDay: k.day(0), CategoryID: k.category, Amount: dd("248"), Status: domain.ExpensePendiente,
			Description: "Refrescos", UserID: k.user,
			Items: []app.ExpenseItemInput{{ItemType: "ingrediente", IngredientID: &ing, Description: "Refresco",
				Quantity: dd("1"), PresentationID: &caja, Amount: dd("248")}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := k.svc.ReceiveItem(k.ctx, id, k.items(t, id)["Refresco"], app.ReceiveItemInput{Qty: dd("1")}, k.user); err != nil {
			t.Fatal(err)
		}
		check(t, ing)
	})

	t.Run("«Llegó algo más»", func(t *testing.T) {
		ing, caja := refresco(t, "Refresco 182 algo más")
		id := k.paidWithoutGoods(t, "10", k.day(-1))
		if err := k.svc.AddArrivedItem(k.ctx, id, app.ArrivedItemInput{ItemType: "ingrediente", IngredientID: &ing,
			Description: "Refresco", Quantity: dd("1"), PresentationID: &caja, Amount: dd("248")}, k.user); err != nil {
			t.Fatal(err)
		}
		check(t, ing)
	})
}
