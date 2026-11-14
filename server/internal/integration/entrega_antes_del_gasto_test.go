//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Una compra que se pudo crear no se podía editar: crear aceptaba una entrega anterior a la fecha del
// gasto y editarla la rechazaba (domain.ValidDeliveryDate, FR-015 de la 053). La regla única es la de
// la operación real: lo que llegó ayer y se paga con la caja de hoy (cuyo turno fija la fecha del
// gasto) llegó antes del gasto. La entrega solo no puede ser de un día que no ha pasado, en los tres
// caminos: crear, «Llegó todo» y editar.

func (k *editKit) pendingPurchase(t *testing.T, expenseDay, receivedAt string) (int64, error) {
	t.Helper()
	qty := dd("4")
	item := app.ExpenseItemInput{ItemType: "ingrediente", IngredientID: &k.tapas, Description: "Tapas",
		Quantity: qty, UnitID: &k.pieza, Amount: dd("40")}
	if receivedAt != "" {
		item.QtyReceived = &qty
	}
	return k.svc.CreateExpense(k.ctx, app.ExpenseInput{ExpenseDay: expenseDay, ReceivedAt: receivedAt, CategoryID: k.category,
		Amount: dd("40"), Status: domain.ExpensePendiente, Description: "Tapas a crédito", UserID: k.user,
		Items: []app.ExpenseItemInput{item}})
}

// editsLikeItWasCreated edita la compra repitiendo la fecha de entrega con que se creó: si crear la
// aceptó, editar también.
func (k *editKit) editsLikeItWasCreated(t *testing.T, id int64, arrived, path string) {
	t.Helper()
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{CanReceive: true,
		Add: []app.EditLineInput{k.line(k.fajillas, "Fajillas", "2", "10", arrived)}}, k.user, true); err != nil {
		t.Fatalf("una compra que se pudo crear (%s, llegó el %s) no se puede editar: %v", path, arrived, err)
	}
}

func TestAPurchaseThatCouldBeCreatedCanBeEdited(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	yesterday, today := k.day(-1), k.day(0)

	t.Run("crear con la entrega del día anterior al gasto", func(t *testing.T) {
		id, err := k.pendingPurchase(t, today, yesterday)
		if err != nil {
			t.Fatalf("crear: %v", err)
		}
		k.editsLikeItWasCreated(t, id, yesterday, "crear")
	})

	t.Run("«Llegó todo» con el día anterior al gasto", func(t *testing.T) {
		id, err := k.pendingPurchase(t, today, "")
		if err != nil {
			t.Fatal(err)
		}
		if err := k.svc.ReceiveExpense(k.ctx, id, yesterday, k.user); err != nil {
			t.Fatalf("recibir: %v", err)
		}
		k.editsLikeItWasCreated(t, id, yesterday, "recibir")
	})

	t.Run("registrar una entrega del día anterior al gasto", func(t *testing.T) {
		id, err := k.pendingPurchase(t, today, "")
		if err != nil {
			t.Fatal(err)
		}
		item := k.items(t, id)["Tapas"]
		k.confirmDelivery(t, id, app.DeliveryInput{On: yesterday, Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("4")}}}, nil)
	})

	t.Run("mover la fecha del gasto después de su entrega no la deja inválida", func(t *testing.T) {
		id, err := k.pendingPurchase(t, k.day(-3), k.day(-3))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{Header: &app.EditHeaderInput{ExpenseDate: &today}}, k.user, true); err != nil {
			t.Fatalf("cambiar la fecha del gasto (FR-002: las entregas no se mueven): %v", err)
		}
		k.editsLikeItWasCreated(t, id, k.day(-3), "fecha del gasto movida")
	})
}

// El hermano: editar rechaza una entrega futura y crear y «Llegó todo» la aceptaban. Una compra así
// tampoco se podía editar después.
func TestAFutureDeliveryIsRejectedOnEveryPath(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	tomorrow := k.day(1)

	if _, err := k.pendingPurchase(t, k.day(0), tomorrow); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("crear con entrega de mañana = %v, quiere validación", err)
	}
	id, err := k.pendingPurchase(t, k.day(0), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := k.svc.ReceiveExpense(k.ctx, id, tomorrow, k.user); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("«Llegó todo» con mañana = %v, quiere validación", err)
	}
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{CanReceive: true,
		Add: []app.EditLineInput{k.line(k.fajillas, "Fajillas", "2", "10", tomorrow)}}, k.user, true); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("editar con entrega de mañana = %v, quiere validación", err)
	}
}

// El mismo defecto por el MCP: purchase_create con received_on de ayer (el turno abierto de hoy fija
// la fecha del gasto) y después expense_edit con esa misma llegada.
func TestAPurchaseThatCouldBeCreatedByMCPCanBeEdited(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	f := mcpSeed(t, w)
	grams := unitIDOf(t, w, "g")
	tok := w.token(t, w.company, w.admin, "write:purchases", "write:inventory", "write:expenses")
	ing := ingredientIn(t, w.owner, w.company, "Azúcar entrega anterior MCP", "g")
	var yesterday, tomorrow string
	if err := w.owner.Pool.QueryRow(context.Background(), `select (business_day_of($1, now()) - 1)::text, (business_day_of($1, now()) + 1)::text`,
		w.company).Scan(&yesterday, &tomorrow); err != nil {
		t.Fatal(err)
	}
	line := func() map[string]any {
		return map[string]any{"name": "AZUCAR 1KG", "ingredient_id": ing, "unit_id": grams, "quantity": "1000", "amount": "30.00"}
	}

	exp, _ := purchaseLine(t, w, tok, f, "entrega-ayer-0001", line(), map[string]any{"received_on": yesterday})
	add := line()
	add["arrived_on"] = yesterday
	if res := callTool(t, w.router, tok, "expense_edit", map[string]any{"expense_id": exp, "add": []any{add}}); res.isError {
		t.Fatalf("una compra que se pudo crear por el MCP (llegó el %s) no se puede editar: %s", yesterday, res.text)
	}

	args := map[string]any{"category_id": f.category, "amount": "30.00", "lines": []any{line()}, "received_on": tomorrow}
	res := callTool(t, w.router, tok, "purchase_create", applyArgs(args, "entrega-manana-0001"))
	if !res.isError || !strings.Contains(res.text, "no ha pasado") {
		t.Fatalf("purchase_create con received_on de mañana: %s", res.text)
	}
}
