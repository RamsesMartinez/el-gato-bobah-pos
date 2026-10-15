//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// POR RECIBIR (spec 038, historia 1b): lo pagado que no ha llegado se recibe por partes, de más
// con su importe, o se cierra con «ya no va a llegar».

// porLlegar registra 3 bultos de azúcar pagados ($1,500) que no han llegado.
func (k *purchaseKit) porLlegar(t *testing.T) (expense, item int64) {
	t.Helper()
	in := k.bultos("3", "1500")
	in.ReceivedAt = ""
	in.Items[0].QtyReceived = nil
	id, err := k.svc.CreateExpense(k.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	item, _ = k.itemOf(t, id)
	return id, item
}

func (k *purchaseKit) expense(t *testing.T, id int64) (amount decimal.Decimal, received bool) {
	t.Helper()
	var r *string
	if err := k.st.Pool.QueryRow(context.Background(), `select amount, received_at::text from expenses where id = $1`, id).Scan(&amount, &r); err != nil {
		t.Fatal(err)
	}
	return amount, r != nil
}

func pendingOf(t *testing.T, list []app.PendingReceipt, item int64) *app.PendingReceipt {
	t.Helper()
	for i := range list {
		if list[i].ItemID == item {
			return &list[i]
		}
	}
	return nil
}

func TestReceivingInParts(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, item := k.porLlegar(t)
	list, err := k.svc.PendingReceipts(k.ctx)
	if err != nil {
		t.Fatal(err)
	}
	p := pendingOf(t, list, item)
	if p == nil || !p.Missing.Equal(dd("3")) || p.ExpenseID != id {
		t.Fatalf("por recibir = %+v; quiere 3 bultos de la compra %d", p, id)
	}

	// Llegan 2 de 3: suman 50 kg y queda 1 por recibir.
	if err := k.svc.ReceiveItem(k.ctx, id, item, app.ReceiveItemInput{Qty: dd("2"), SeenApplied: dd("0")}, k.user); err != nil {
		t.Fatalf("recibir 2: %v", err)
	}
	if got := k.onHand(t); !got.Equal(dd("50000")) {
		t.Fatalf("tras recibir 2 = %s; quiere 50000", got)
	}
	if _, received := k.expense(t, id); received {
		t.Error("falta 1 bulto y la compra quedó como recibida")
	}
	// La misma recepción con lo aplicado viejo: otra tableta ya la hizo → 409, nunca suma dos veces.
	if err := k.svc.ReceiveItem(k.ctx, id, item, app.ReceiveItemInput{Qty: dd("2"), SeenApplied: dd("0")}, k.user); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("recepción repetida = %v; quiere ErrConflict", err)
	}

	// El último «ya no va a llegar»: no suma, no cambia el total y cierra la compra.
	if err := k.svc.MarkNotComing(k.ctx, id, item, dd("1"), k.user); err != nil {
		t.Fatalf("ya no va a llegar: %v", err)
	}
	amount, received := k.expense(t, id)
	if !amount.Equal(dd("1500")) || !received {
		t.Errorf("tras cerrar: total %s, recibida %v; quiere 1500 y recibida", amount, received)
	}
	if got := k.onHand(t); !got.Equal(dd("50000")) {
		t.Errorf("«ya no va a llegar» movió lo guardado a %s", got)
	}
	list, _ = k.svc.PendingReceipts(k.ctx)
	if pendingOf(t, list, item) != nil {
		t.Error("cerrado y sigue en «Por recibir»")
	}
}

func TestReceivingMoreThanMissingRaisesTheTotal(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, item := k.porLlegar(t)
	// Llegan 4 de 3: sin el importe de la de más, no se adivina.
	if err := k.svc.ReceiveItem(k.ctx, id, item, app.ReceiveItemInput{Qty: dd("4"), SeenApplied: dd("0")}, k.user); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("de más sin importe = %v; quiere ErrValidation", err)
	}
	extra := dd("500")
	if err := k.svc.ReceiveItem(k.ctx, id, item, app.ReceiveItemInput{Qty: dd("4"), SeenApplied: dd("0"), ExtraAmount: &extra}, k.user); err != nil {
		t.Fatalf("de más con importe: %v", err)
	}
	amount, received := k.expense(t, id)
	if !amount.Equal(dd("2000")) || !received {
		t.Errorf("total %s, recibida %v; quiere 2000 y recibida", amount, received)
	}
	if got := k.onHand(t); !got.Equal(dd("100000")) {
		t.Errorf("lo guardado = %s; quiere 100000 (4 bultos)", got)
	}
	var qty, lineAmount decimal.Decimal
	_ = k.st.Pool.QueryRow(context.Background(), `select quantity, amount from expense_items where id = $1`, item).Scan(&qty, &lineAmount)
	if !qty.Equal(dd("4")) || !lineAmount.Equal(dd("2000")) {
		t.Errorf("renglón: %s por $%s; quiere 4 por $2000", qty, lineAmount)
	}
}

func TestSomethingElseArrived(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("1", "500"))
	if err != nil {
		t.Fatal(err)
	}
	cost := k.ingredientCost(t)
	// Un regalo: suma, no cambia el total ni el costo.
	gift := app.ArrivedItemInput{ItemType: "ingrediente", IngredientID: &k.azucar, Description: "Azúcar de regalo",
		Quantity: dd("1"), PresentationID: &k.bulto, Amount: dd("0"), IsGift: true}
	if err := k.svc.AddArrivedItem(k.ctx, id, gift, k.user); err != nil {
		t.Fatalf("regalo: %v", err)
	}
	if amount, _ := k.expense(t, id); !amount.Equal(dd("500")) {
		t.Errorf("el regalo cambió el total a %s", amount)
	}
	if c := k.ingredientCost(t); !c.Equal(cost) {
		t.Errorf("el regalo cambió el costo de %s a %s", cost, c)
	}
	gift.Amount = dd("10")
	if err := k.svc.AddArrivedItem(k.ctx, id, gift, k.user); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("regalo con importe = %v; quiere ErrValidation", err)
	}
	// Lo pagué: suma y sube el total por su importe.
	paid := gift
	paid.IsGift, paid.Amount = false, dd("600")
	if err := k.svc.AddArrivedItem(k.ctx, id, paid, k.user); err != nil {
		t.Fatalf("lo pagué: %v", err)
	}
	if amount, _ := k.expense(t, id); !amount.Equal(dd("1100")) {
		t.Errorf("total = %s; quiere 1100", amount)
	}
	if got := k.onHand(t); !got.Equal(dd("75000")) {
		t.Errorf("lo guardado = %s; quiere 75000", got)
	}
}

// Una compra pagada que sube de total por lo que llegó de más vuelve a deber: el pago ya no cubre.
func TestAPaidPurchaseThatGrowsOwesTheDifference(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, item := k.porLlegar(t)
	if _, err := k.st.Pool.Exec(context.Background(), `update expenses set status = 'pagada', paid_at = now() where id = $1`, id); err != nil {
		t.Fatal(err)
	}
	extra := dd("500")
	if err := k.svc.ReceiveItem(k.ctx, id, item, app.ReceiveItemInput{Qty: dd("4"), SeenApplied: dd("0"), ExtraAmount: &extra}, k.user); err != nil {
		t.Fatal(err)
	}
	var status string
	var paidAt *time.Time
	_ = k.st.Pool.QueryRow(context.Background(), `select status::text, paid_at from expenses where id = $1`, id).Scan(&status, &paidAt)
	if status != "pendiente" {
		t.Errorf("estado = %s; quiere pendiente (se deben $500)", status)
	}
	// Revisión de base de datos del 038: «pendiente» con la fecha de un pago que ya no la cubre es un
	// dato que un reporte de cuentas por pagar tomaría por cierto.
	if paidAt != nil {
		t.Errorf("pendiente con fecha de pago %v", paidAt)
	}
}

// Revisión de seguridad del 038: cada importe de más se validaba solo, y la suma no. Diez «Llegó algo
// más» de $9,999,999 llevaban el total de la compra a desbordar la columna: 500 en vez de 400.
func TestATotalThatGrowsPastTheMoneyCeilingIsRejected(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, err := k.svc.CreateExpense(k.ctx, k.bultos("1", "500"))
	if err != nil {
		t.Fatal(err)
	}
	big := app.ArrivedItemInput{ItemType: "ingrediente", IngredientID: &k.azucar, Description: "Azúcar",
		Quantity: dd("100"), PresentationID: &k.bulto, Amount: domain.MaxMoney}
	if err := k.svc.AddArrivedItem(k.ctx, id, big, k.user); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("total sobre el tope = %v; quiere ErrValidation", err)
	}
	// Y un renglón de una sola pieza carísima: su costo unitario no cabe en la columna (500 antes).
	one := big
	one.Quantity, one.Amount = dd("1"), dd("2000000")
	if err := k.svc.AddArrivedItem(k.ctx, id, one, k.user); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("costo unitario sobre la columna = %v; quiere ErrValidation", err)
	}
	gasto := k.bultos("1", "2000000")
	if _, err := k.svc.CreateExpense(k.ctx, gasto); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("captura con costo unitario sobre la columna = %v; quiere ErrValidation", err)
	}
}

func TestPendingReceiptsWithTwoBranchesAreNotGuessed(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	k.porLlegar(t)
	addBranch(t, k.st, defaultCompanyID, "NORTE")
	if _, err := k.svc.PendingReceipts(k.ctx); !errors.Is(err, domain.ErrBranchAmbiguous) {
		t.Fatalf("con dos sucursales = %v; quiere ErrBranchAmbiguous", err)
	}
}

func TestPendingReceiptsAreIsolated(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	id, item := k.porLlegar(t)
	other := makeCompany(t, k.st, "por-recibir-otra")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		svc := app.NewBackofficeService(as, nil)
		if list, err := svc.PendingReceipts(ctx); err == nil && pendingOf(t, list, item) != nil {
			t.Fatal("se ve lo por recibir de otra empresa")
		}
		if err := svc.ReceiveItem(ctx, id, item, app.ReceiveItemInput{Qty: dd("1"), SeenApplied: dd("0")}, k.user); err == nil {
			t.Fatal("recibió en una compra de otra empresa")
		}
		if err := svc.MarkNotComing(ctx, id, item, dd("1"), k.user); err == nil {
			t.Fatal("cerró un renglón de otra empresa")
		}
	})
	if got := k.onHand(t); !got.IsZero() {
		t.Errorf("otra empresa movió lo guardado a %s", got)
	}
}

// «Llegó una parte» desde la captura (FR-007e): suma lo que llegó y el resto queda por recibir.
func TestAPartlyArrivedLineLeavesTheRestPending(t *testing.T) {
	t.Parallel()
	k := newPurchaseKit(t)
	in := k.bultos("2", "1000")
	one := dd("1")
	in.Items[0].QtyReceived = &one
	id, err := k.svc.CreateExpense(k.ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if got := k.onHand(t); !got.Equal(dd("25000")) {
		t.Errorf("lo guardado = %s; quiere 25000 (llegó 1 de 2 bultos)", got)
	}
	if _, received := k.expense(t, id); received {
		t.Error("falta un bulto y la compra quedó como recibida")
	}
	item, _ := k.itemOf(t, id)
	list, err := k.svc.PendingReceipts(k.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p := pendingOf(t, list, item); p == nil || !p.Missing.Equal(dd("1")) {
		t.Errorf("por recibir = %+v; quiere 1 bulto", p)
	}
}
