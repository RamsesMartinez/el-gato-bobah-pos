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

// RECIBIR UNA COMPRA CON SU FECHA DE CADUCIDAD (spec 098, historia 1).

func (k *editKit) lotOf(t *testing.T, ing int64, d string) decimal.Decimal {
	t.Helper()
	var v decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `select coalesce(sum(qty), 0) from stock_lots
		where ingredient_id = $1 and expires_on = $2::date`, ing, d).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func (k *editKit) pendingTapas(t *testing.T, qty string) (expense, item int64) {
	t.Helper()
	expense = k.paidWithoutGoods(t, "50", k.day(-2))
	k.confirmEdit(t, expense, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", qty, "50", "")}}, nil)
	return expense, k.items(t, expense)["Tapas"]
}

// Recibir con fecha crea el lote; sin fecha, no.
func TestReceivingWithADateCreatesTheLot(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	exp, item := k.pendingTapas(t, "5")
	d := k.day(40)
	k.confirmDelivery(t, exp, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("5"), ExpiresOn: ptr(d)}}}, nil)
	if v := k.lotOf(t, k.tapas, d); !v.Equal(decimal.NewFromInt(5)) {
		t.Fatalf("lote del %s = %s, quería 5", d, v)
	}
	var stored *string
	if err := k.st.Pool.QueryRow(context.Background(), `select expires_on::text from expense_items where id = $1`, item).Scan(&stored); err != nil || stored == nil || *stored != d {
		t.Fatalf("el renglón guardó %v (%v)", stored, err)
	}

	exp2, item2 := k.pendingTapas(t, "3")
	k.confirmDelivery(t, exp2, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item2, Qty: dd("3")}}}, nil)
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from stock_lots where ingredient_id = $1 and qty > 0`, k.tapas).Scan(&n); err != nil || n != 1 {
		t.Fatalf("lotes = %d (%v): recibir sin fecha no crea lote", n, err)
	}
}

// Una segunda entrega con otra fecha pasa lo ya recibido del renglón a la fecha nueva: un renglón
// lleva una sola fecha (spec, Assumptions).
func TestASecondDeliveryWithAnotherDateMovesTheLine(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	exp, item := k.pendingTapas(t, "5")
	d1, d2 := k.day(30), k.day(45)
	k.confirmDelivery(t, exp, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("2"), ExpiresOn: ptr(d1)}}}, nil)
	k.confirmDelivery(t, exp, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("3"), ExpiresOn: ptr(d2)}}}, nil)
	if v := k.lotOf(t, k.tapas, d1); !v.IsZero() {
		t.Fatalf("el lote viejo quedó con %s", v)
	}
	if v := k.lotOf(t, k.tapas, d2); !v.Equal(decimal.NewFromInt(5)) {
		t.Fatalf("el lote nuevo = %s, quería 5", v)
	}
	// Llegó lo que faltaba sin decir fecha: el renglón conserva la que tenía.
	exp3, item3 := k.pendingTapas(t, "4")
	k.confirmDelivery(t, exp3, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item3, Qty: dd("1"), ExpiresOn: ptr(d1)}}}, nil)
	k.confirmDelivery(t, exp3, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item3, Qty: dd("3")}}}, nil)
	if v := k.lotOf(t, k.tapas, d1); !v.Equal(decimal.NewFromInt(4)) {
		t.Fatalf("lote del %s = %s, quería 4", d1, v)
	}
}

// Anular la compra baja el lote en lo que había sumado, también si ya se usó una
// parte: el lote se queda en 0 y el resto sale de lo sin fecha.
func TestRemovingAReceivedLineLowersItsLot(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	exp, item := k.pendingTapas(t, "5")
	d := k.day(20)
	k.confirmDelivery(t, exp, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("5"), ExpiresOn: ptr(d)}}}, nil)
	if _, err := k.st.Pool.Exec(context.Background(), `insert into stock_movements (item_type, ingredient_id, movement_type, quantity)
		values ('ingrediente', $1, 'venta', -2)`, k.tapas); err != nil {
		t.Fatal(err)
	}
	if err := k.svc.CancelExpense(k.ctx, exp, "se capturó dos veces", k.user, true); err != nil {
		t.Fatal(err)
	}
	if v := k.lotOf(t, k.tapas, d); !v.IsZero() {
		t.Fatalf("lote tras anular la compra = %s, quería 0", v)
	}
}

// La fecha se valida en la frontera: fuera de rango o en un producto es 400.
func TestADeliveryRejectsAnAbsurdExpiry(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	exp, item := k.pendingTapas(t, "5")
	for _, bad := range []string{"1999-12-31", k.today.AddDate(11, 0, 0).Format("2006-01-02"), "31/12/2026"} {
		_, err := k.svc.DeliverExpense(k.ctx, exp, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("5"), ExpiresOn: ptr(bad)}}}, k.user, true)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("caducidad %q = %v, quería validación", bad, err)
		}
	}
}
