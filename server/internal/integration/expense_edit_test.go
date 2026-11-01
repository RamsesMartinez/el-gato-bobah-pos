//go:build integration

package integration

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// EDITAR UN GASTO YA GUARDADO (spec 053). El caso que lo motivó, del 1-nov-2026: una compra de
// $67.50 de tapas y fajillas se guardó «solo el gasto», pagada; las existencias quedaron en −5 y no
// hubo forma de agregarle la mercancía.

type editKit struct {
	*purchaseKit
	pieza    int16
	kg       int16
	transfer int16
	tapas    int64
	fajillas int64
	today    time.Time
}

func newEditKit(t *testing.T) *editKit {
	t.Helper()
	k := &editKit{purchaseKit: newPurchaseKit(t)}
	ctx := context.Background()
	if err := k.st.Pool.QueryRow(ctx, `select id from units where code = 'pieza'`).Scan(&k.pieza); err != nil {
		t.Fatal(err)
	}
	if err := k.st.Pool.QueryRow(ctx, `select id from units where code = 'kg'`).Scan(&k.kg); err != nil {
		t.Fatal(err)
	}
	if err := k.st.Pool.QueryRow(ctx, `select id from payment_methods where company_id = $1 and not affects_cash_drawer order by id limit 1`,
		defaultCompanyID).Scan(&k.transfer); err != nil {
		t.Fatal(err)
	}
	k.tapas = ingredientIn(t, k.st, defaultCompanyID, "Tapas 16 oz edición", "pieza")
	k.fajillas = ingredientIn(t, k.st, defaultCompanyID, "Fajillas edición", "pieza")
	mx, _ := time.LoadLocation(domain.DefaultTimezone)
	k.today = domain.BusinessDate(time.Now(), mx)
	return k
}

func (k *editKit) day(offset int) string { return k.today.AddDate(0, 0, offset).Format("2006-01-02") }

// paidWithoutGoods es el gasto del 1-nov: pagado por transferencia, sin renglones.
func (k *editKit) paidWithoutGoods(t *testing.T, amount string, day string) int64 {
	t.Helper()
	id, err := k.svc.CreateExpense(k.ctx, app.ExpenseInput{
		ExpenseDay: day, CategoryID: k.category, Amount: dd(amount), Status: domain.ExpensePagada,
		Description: "Tapas y fajillas", UserID: k.user,
		Payments: []app.ExpensePaymentInput{{MethodID: k.transfer, Amount: dd(amount)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (k *editKit) line(ing int64, desc, qty, amount, arrived string) app.EditLineInput {
	return app.EditLineInput{ItemType: "ingrediente", IngredientID: &ing, Description: desc, Quantity: dd(qty),
		UnitID: &k.pieza, Amount: dd(amount), ArrivedOn: arrived}
}

// confirmEdit previsualiza, decide lo que pida con `decide` y confirma con la huella vista.
func (k *editKit) confirmEdit(t *testing.T, id int64, in app.EditInput, decide map[string]string) app.EditResult {
	t.Helper()
	in.Decisions = decide
	in.CanReceive = true // el usuario del kit es admin: edita y recibe
	pre, err := k.svc.EditExpense(k.ctx, id, in, k.user, true)
	if err != nil {
		t.Fatalf("vista previa: %v", err)
	}
	in.Fingerprint = pre.Preview.Fingerprint
	res, err := k.svc.EditExpense(k.ctx, id, in, k.user, false)
	if err != nil {
		t.Fatalf("confirmar: %v", err)
	}
	if res.Preview.Fingerprint != pre.Preview.Fingerprint {
		t.Fatal("lo confirmado no es lo previsualizado")
	}
	return res
}

func (k *editKit) confirmDelivery(t *testing.T, id int64, in app.DeliveryInput, decide map[string]string) app.EditResult {
	t.Helper()
	in.Decisions = decide
	pre, err := k.svc.DeliverExpense(k.ctx, id, in, k.user, true)
	if err != nil {
		t.Fatalf("vista previa de la entrega: %v", err)
	}
	in.Fingerprint = pre.Preview.Fingerprint
	res, err := k.svc.DeliverExpense(k.ctx, id, in, k.user, false)
	if err != nil {
		t.Fatalf("confirmar la entrega: %v", err)
	}
	return res
}

func (k *editKit) items(t *testing.T, expense int64) map[string]int64 {
	t.Helper()
	rows, err := k.st.Pool.Query(context.Background(), `select description, id from expense_items where expense_id = $1 and voided_at is null`, expense)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var d string
		var id int64
		if err := rows.Scan(&d, &id); err != nil {
			t.Fatal(err)
		}
		out[d] = id
	}
	return out
}

func (k *editKit) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (k *editKit) expenseRow(t *testing.T, id int64) (amount decimal.Decimal, status string, received *string) {
	t.Helper()
	if err := k.st.Pool.QueryRow(context.Background(), `select amount, status::text, received_at::text from expenses where id = $1`, id).
		Scan(&amount, &status, &received); err != nil {
		t.Fatal(err)
	}
	return
}

func (k *editKit) payments(t *testing.T, id int64) []string {
	t.Helper()
	rows, err := k.st.Pool.Query(context.Background(), `select amount::text from expense_payments where expense_id = $1 order by id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var a string
		_ = rows.Scan(&a)
		out = append(out, a)
	}
	return out
}

// closedCount cierra, como owner, un conteo del insumo capturado en `at`.
func (k *editKit) closedCount(t *testing.T, ing int64, at time.Time) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := k.st.Pool.QueryRow(ctx, `insert into stock_counts (company_id, branch_id, opened_by, status, closed_by, closed_at, opened_at)
		select $1, id, $2, 'closed', $2, $3, $3 from branches where company_id = $1 order by id limit 1 returning id`,
		defaultCompanyID, k.user, at).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `insert into stock_count_lines (count_id, company_id, ingredient_id, counted_qty, system_qty, captured_by, captured_at)
		values ($1, $2, $3, 0, 0, $4, $5)`, id, defaultCompanyID, ing, k.user, at); err != nil {
		t.Fatal(err)
	}
	return id
}

// Criterio 1 y SC-001: el gasto del 1-nov se corrige sin tocar el importe ni el pago.
func TestEditTheNov1ExpenseAddsGoodsWithoutTouchingMoney(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "67.50", k.day(-5))
	beforeTapas := onHandIngredient(t, k.st, k.tapas)

	in := app.EditInput{Add: []app.EditLineInput{
		k.line(k.tapas, "Tapas Traveler 16 oz", "50", "50.00", ""),
		k.line(k.fajillas, "Fajillas de cartón", "10", "17.50", ""),
	}}
	pre, err := k.svc.EditExpense(k.ctx, id, in, k.user, true)
	if err != nil {
		t.Fatal(err)
	}
	if !pre.Preview.Money.AmountAfter.Equal(dd("67.50")) || pre.Preview.Money.DecisionKey != nil {
		t.Fatalf("la vista previa movió el dinero: %+v", pre.Preview.Money)
	}
	if len(pre.Preview.Inventory) != 0 {
		t.Fatalf("renglones sin recibir movieron el almacén en la vista previa: %+v", pre.Preview.Inventory)
	}
	if pre.Preview.Lines == nil || pre.Preview.Costs == nil || pre.Preview.Decisions == nil || pre.Preview.Header == nil {
		t.Fatal("un arreglo de la vista previa salió nil: el JSON diría null")
	}
	k.confirmEdit(t, id, in, nil)
	if n := k.count(t, `select count(*) from stock_movements where expense_id = $1`, id); n != 0 {
		t.Fatalf("agregar sin recibir dejó %d movimientos", n)
	}

	it := k.items(t, id)
	k.confirmDelivery(t, id, app.DeliveryInput{Lines: []app.DeliveryLineInput{
		{ItemID: it["Tapas Traveler 16 oz"], Qty: dd("50")}, {ItemID: it["Fajillas de cartón"], Qty: dd("10")},
	}}, nil)

	if got := onHandIngredient(t, k.st, k.tapas); !got.Equal(beforeTapas.Add(dd("50"))) {
		t.Fatalf("tapas = %s, quiere %s", got, beforeTapas.Add(dd("50")))
	}
	if got := onHandIngredient(t, k.st, k.fajillas); !got.Equal(dd("10")) {
		t.Fatalf("fajillas = %s, quiere 10", got)
	}
	amount, status, received := k.expenseRow(t, id)
	if !amount.Equal(dd("67.50")) || status != "pagada" {
		t.Fatalf("el gasto quedó en %s %s: el dinero no debía moverse", amount, status)
	}
	if received == nil || *received != k.day(0) {
		t.Fatalf("recibido = %v, quiere hoy", received)
	}
	if p := k.payments(t, id); len(p) != 1 || p[0] != "67.50" {
		t.Fatalf("pagos = %v: el pago original cambió", p)
	}
	if k.audited(t, "expense.edited", id) != 1 || k.audited(t, "expense.received", id) != 1 {
		t.Fatal("faltó la bitácora de la edición o de la entrega")
	}
}

// R1/SC-002: una vista previa no deja nada: ni movimientos, ni bitácora, ni importe, ni costo.
func TestEditPreviewWritesNothing(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "100", k.day(-2))
	costBefore := k.ingredientCost(t)
	azucar := app.EditLineInput{ItemType: "ingrediente", IngredientID: &k.azucar, Description: "Azúcar", Quantity: dd("1"),
		PresentationID: &k.bulto, Amount: dd("150"), ArrivedOn: k.day(0)}
	pre, err := k.svc.EditExpense(k.ctx, id, app.EditInput{CanReceive: true, Add: []app.EditLineInput{azucar}}, k.user, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(pre.Preview.Inventory) != 1 || !pre.Preview.Inventory[0].After.Sub(pre.Preview.Inventory[0].Before).Equal(dd("25000")) {
		t.Fatalf("la vista previa no mostró el bulto entrando: %+v", pre.Preview.Inventory)
	}
	if !pre.Preview.Money.AmountAfter.Equal(dd("150")) {
		t.Fatalf("importe previsto %s, quiere 150", pre.Preview.Money.AmountAfter)
	}
	if n := k.count(t, `select count(*) from stock_movements where expense_id = $1`, id); n != 0 {
		t.Fatalf("la vista previa dejó %d movimientos", n)
	}
	if n := k.audited(t, "expense.edited", id); n != 0 {
		t.Fatalf("la vista previa dejó %d eventos en la bitácora", n)
	}
	if amount, _, _ := k.expenseRow(t, id); !amount.Equal(dd("100")) {
		t.Fatalf("la vista previa cambió el importe a %s", amount)
	}
	if !k.ingredientCost(t).Equal(costBefore) {
		t.Fatal("la vista previa cambió el costo del insumo")
	}
	if n := k.count(t, `select count(*) from expense_items where expense_id = $1`, id); n != 0 {
		t.Fatal("la vista previa dejó el renglón")
	}
}

// R2/FR-029: si entre la vista previa y la confirmación cambió algo, no se escribe nada.
func TestEditWithAStaleFingerprintIsRejected(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "100", k.day(-2))
	in := app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "10", "100", "")}}
	pre, err := k.svc.EditExpense(k.ctx, id, in, k.user, true)
	if err != nil {
		t.Fatal(err)
	}
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.fajillas, "Fajillas", "1", "30", "")}}, map[string]string{"money": "leave"})
	in.Fingerprint = pre.Preview.Fingerprint
	if _, err := k.svc.EditExpense(k.ctx, id, in, k.user, false); !errors.Is(err, domain.ErrExpenseEditStale) {
		t.Fatalf("confirmar una vista previa vieja = %v, quiere ErrExpenseEditStale", err)
	}
	if n := k.count(t, `select count(*) from expense_items where expense_id = $1 and description = 'Tapas'`, id); n != 0 {
		t.Fatal("se escribió una edición con una vista previa vieja")
	}
}

// Criterio 5 y FR-011/012: el importe que sube sobre lo pagado pide decidir; el pago no cambia.
func TestEditRaisingTheAmountAsksForPaymentAndKeepsThePayment(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "67.50", k.day(-1))
	in := app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "50", "80.00", "")}}
	pre, err := k.svc.EditExpense(k.ctx, id, in, k.user, true)
	if err != nil {
		t.Fatal(err)
	}
	if pre.Preview.Money.DecisionKey == nil || !pre.Preview.Money.Due.Equal(dd("12.50")) {
		t.Fatalf("el aviso de dinero no salió: %+v", pre.Preview.Money)
	}
	in.Fingerprint = pre.Preview.Fingerprint
	if _, err := k.svc.EditExpense(k.ctx, id, in, k.user, false); !errors.Is(err, domain.ErrDecisionRequired) {
		t.Fatalf("confirmar sin decidir = %v, quiere ErrDecisionRequired", err)
	}
	k.confirmEdit(t, id, in, map[string]string{"money": "leave"})
	amount, status, _ := k.expenseRow(t, id)
	if !amount.Equal(dd("80")) || status != "pendiente" {
		t.Fatalf("gasto %s %s, quiere 80 pendiente", amount, status)
	}
	if p := k.payments(t, id); len(p) != 1 || p[0] != "67.50" {
		t.Fatalf("pagos = %v: el pago original cambió", p)
	}
	// FR-013: borrar lo agregado lo regresa a pagado, sin tocar el pago.
	it := k.items(t, id)
	k.confirmEdit(t, id, app.EditInput{Remove: []int64{it["Tapas"]}, Add: []app.EditLineInput{k.line(k.tapas, "Tapas 2", "50", "67.50", "")}}, nil)
	if amount, status, _ := k.expenseRow(t, id); !amount.Equal(dd("67.50")) || status != "pagada" {
		t.Fatalf("tras sustituir, gasto %s %s, quiere 67.50 pagada", amount, status)
	}
}

// FR-004: agregar con «Ya llegó» recibe en la misma confirmación, con su fecha.
func TestEditAddingAnArrivedLineReceivesItWithItsDate(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "30", k.day(-3))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "6", "30", k.day(-1))}}, nil)
	var on string
	if err := k.st.Pool.QueryRow(context.Background(), `select effective_on::text from stock_movements where expense_id = $1`, id).Scan(&on); err != nil {
		t.Fatal(err)
	}
	if on != k.day(-1) {
		t.Fatalf("la entrega quedó del %s, quiere %s", on, k.day(-1))
	}
	if _, _, received := k.expenseRow(t, id); received == nil || *received != k.day(-1) {
		t.Fatalf("recibido %v, quiere %s", received, k.day(-1))
	}
	// Una entrega del futuro o anterior al gasto no existe.
	for _, bad := range []string{k.day(1), k.day(-4)} {
		_, err := k.svc.EditExpense(k.ctx, id, app.EditInput{CanReceive: true, Add: []app.EditLineInput{k.line(k.tapas, "Otra", "1", "1", bad)}}, k.user, true)
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("una entrega del %s = %v, quiere validación", bad, err)
		}
	}
}

// Criterio 4: entregas parciales, cada una con su fecha.
func TestDeliveriesInPartsKeepTheirDates(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "50", k.day(-5))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "5", "50", "")}}, nil)
	item := k.items(t, id)["Tapas"]

	pre, err := k.svc.DeliverExpense(k.ctx, id, app.DeliveryInput{On: k.day(-1), Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("3")}}}, k.user, true)
	if err != nil {
		t.Fatal(err)
	}
	var short *app.EditDecision
	for i := range pre.Preview.Decisions {
		if pre.Preview.Decisions[i].Kind == "shortfall" {
			short = &pre.Preview.Decisions[i]
		}
	}
	if short == nil || short.Chosen == nil || *short.Chosen != "pending" {
		t.Fatalf("por pieza, lo que falta debía preseleccionarse como pendiente: %+v", pre.Preview.Decisions)
	}
	k.confirmDelivery(t, id, app.DeliveryInput{On: k.day(-1), Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("3")}}}, nil)
	if _, _, received := k.expenseRow(t, id); received != nil {
		t.Fatal("con 2 por llegar, el gasto quedó recibido")
	}
	k.confirmDelivery(t, id, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("2")}}}, nil)
	if _, _, received := k.expenseRow(t, id); received == nil || *received != k.day(0) {
		t.Fatalf("recibido %v, quiere hoy", received)
	}
	detail, err := k.svc.ExpenseDetail(k.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var hist []app.DeliveryView
	for _, it := range detail.Items {
		if it.ID == item {
			hist = it.Deliveries
		}
	}
	if len(hist) != 2 || hist[0].On != k.day(-1) || !hist[0].QtyBase.Equal(dd("3")) || hist[1].On != k.day(0) {
		t.Fatalf("historial = %+v, quiere ayer 3 y hoy 2", hist)
	}
	if k.audited(t, "expense.received", id) != 2 {
		t.Fatal("cada entrega deja su evento en la bitácora")
	}
	// Una entrega del futuro se rechaza.
	if _, err := k.svc.DeliverExpense(k.ctx, id, app.DeliveryInput{On: k.day(1), Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("1"), ExtraAmount: ptrDec(dd("1"))}}}, k.user, true); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("entrega del futuro = %v", err)
	}
	// Llegó de más sin decir cuánto se pagó por lo de más: 400.
	if _, err := k.svc.DeliverExpense(k.ctx, id, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("1")}}}, k.user, true); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("de más sin importe = %v", err)
	}
}

// FR-017/018: lo que se pesa y llega de menos se ajusta a lo recibido; el importe baja.
func TestShortWeightAdjustsTheAmount(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "300", k.day(-2))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{{ItemType: "ingrediente", IngredientID: &k.azucar,
		Description: "Azúcar suelta", Quantity: dd("10"), UnitID: &k.kg, Amount: dd("300")}}}, nil)
	item := k.items(t, id)["Azúcar suelta"]
	res := k.confirmDelivery(t, id, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("8.5")}}}, nil)
	if !res.Preview.Money.AmountAfter.Equal(dd("255")) || !res.Preview.Money.Overpaid.Equal(dd("45")) {
		t.Fatalf("dinero = %+v, quiere importe 255 y pagado de más 45", res.Preview.Money)
	}
	amount, status, received := k.expenseRow(t, id)
	if !amount.Equal(dd("255")) || status != "pagada" || received == nil {
		t.Fatalf("gasto %s %s recibido %v, quiere 255 pagada y recibida", amount, status, received)
	}
}

// Criterio 2: borrar lo que no llegó no mueve el almacén.
func TestRemovingAPendingLineCreatesNoMovements(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "100", k.day(-1))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "5", "60", ""), k.line(k.fajillas, "Fajillas", "1", "40", "")}}, nil)
	k.confirmEdit(t, id, app.EditInput{Remove: []int64{k.items(t, id)["Tapas"]}}, nil)
	if n := k.count(t, `select count(*) from stock_movements where expense_id = $1`, id); n != 0 {
		t.Fatalf("borrar sin recibir dejó %d movimientos", n)
	}
	if amount, _, _ := k.expenseRow(t, id); !amount.Equal(dd("40")) {
		t.Fatalf("importe %s, quiere 40", amount)
	}
	if n := k.count(t, `select count(*) from expense_items where expense_id = $1 and voided_at is not null`, id); n != 1 {
		t.Fatal("el renglón quitado debe quedar marcado, no borrado")
	}
	// Borrar todo de un gasto sin «sin desglosar» lo dejaría en $0.
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{Remove: []int64{k.items(t, id)["Fajillas"]}}, k.user, true); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("dejar el gasto en $0 = %v, quiere validación", err)
	}
}

// Criterio 3, SC-006, revisión M2: borrar lo recibido sale con la fecha de cada entrega, y una
// corrección posterior se netea en vez de volverse una compra positiva.
func TestRemovingAReceivedLineReversesEachDeliveryOnItsDay(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	// $20 sin desglosar: quitar el único renglón no deja el gasto en $0.
	id := k.paidWithoutGoods(t, "120", k.day(-10))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "50", "100", k.day(-8))}}, nil)
	item := k.items(t, id)["Tapas"]
	// «Corregir lo que llegó» baja a 30 hoy (lo deja el camino de la 038).
	if _, err := k.st.Pool.Exec(context.Background(), `with m as (
		insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity, expense_id, expense_item_id)
		values ($1, 'ingrediente', $2, 'compra', -20, $3, $4) returning 1)
		update expense_items set applied_qty_base = applied_qty_base - 20 where id = $4`, defaultCompanyID, k.tapas, id, item); err != nil {
		t.Fatal(err)
	}
	before := onHandIngredient(t, k.st, k.tapas)
	k.confirmEdit(t, id, app.EditInput{Remove: []int64{item}}, nil)

	rows, err := k.st.Pool.Query(context.Background(), `select quantity::text, effective_on::text from stock_movements
		where expense_item_id = $1 and reason like 'renglón quitado%' order by id`, item)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var q, on string
		_ = rows.Scan(&q, &on)
		got = append(got, q+"@"+on)
	}
	rows.Close()
	if len(got) != 1 || got[0] != "-30.0000@"+k.day(-8) {
		t.Fatalf("contra-movimientos = %v, quiere uno de -30 fechado %s (nunca una compra positiva)", got, k.day(-8))
	}
	if n := k.count(t, `select count(*) from stock_movements where expense_item_id = $1 and movement_type = 'compra' and quantity > 0 and reason like 'renglón quitado%'`, item); n != 0 {
		t.Fatal("un borrado emitió una compra positiva")
	}
	var sum decimal.Decimal
	if err := k.st.Pool.QueryRow(context.Background(), `select coalesce(sum(quantity), 0) from stock_movements where expense_item_id = $1`, item).Scan(&sum); err != nil {
		t.Fatal(err)
	}
	if !sum.IsZero() {
		t.Fatalf("los movimientos del renglón suman %s, no 0", sum)
	}
	if after := onHandIngredient(t, k.st, k.tapas); !after.Equal(before.Sub(dd("30"))) {
		t.Fatalf("existencias %s, quiere %s", after, before.Sub(dd("30")))
	}
	if k.audited(t, "expense.edited", id) != 2 {
		t.Fatal("el borrado no dejó su evento")
	}
}

// FR-022/023, revisiones A2 y M1: con un conteo posterior, se decide; «No mover» deja lo guardado
// como lo dejó el conteo, con un movimiento ligado al conteo y no al renglón.
func TestRemovingAReceivedLineAfterACountAsksAndKeep(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "100", k.day(-10))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "50", "100", k.day(-8))}}, nil)
	item := k.items(t, id)["Tapas"]
	countID := k.closedCount(t, k.tapas, time.Now().Add(-48*time.Hour))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.fajillas, "Fajillas", "1", "1", "")}}, map[string]string{"money": "leave"})

	in := app.EditInput{CanReceive: true, Remove: []int64{item}}
	pre, err := k.svc.EditExpense(k.ctx, id, in, k.user, true)
	if err != nil {
		t.Fatal(err)
	}
	var dec *app.EditDecision
	for i := range pre.Preview.Decisions {
		if pre.Preview.Decisions[i].Kind == "count" {
			dec = &pre.Preview.Decisions[i]
		}
	}
	if dec == nil || dec.CountID == nil || *dec.CountID != countID || dec.Chosen != nil {
		t.Fatalf("la vista previa debió pedir decidir por el conteo %d: %+v", countID, pre.Preview.Decisions)
	}
	in.Fingerprint = pre.Preview.Fingerprint
	if _, err := k.svc.EditExpense(k.ctx, id, in, k.user, false); !errors.Is(err, domain.ErrDecisionRequired) {
		t.Fatalf("confirmar sin decidir = %v", err)
	}
	before := onHandIngredient(t, k.st, k.tapas)
	k.confirmEdit(t, id, app.EditInput{Remove: []int64{item}}, map[string]string{dec.Key: "keep"})
	if after := onHandIngredient(t, k.st, k.tapas); !after.Equal(before) {
		t.Fatalf("con «No mover» las existencias cambiaron de %s a %s", before, after)
	}
	if n := k.count(t, `select count(*) from stock_movements sm join stock_counts c on c.id = sm.stock_count_id
		where sm.expense_id = $1 and sm.stock_count_id = $2 and sm.expense_item_id is null and sm.movement_type = 'ajuste'
		  and sm.quantity = 50 and sm.branch_id = c.branch_id and sm.effective_on = $3::date`, id, countID, k.day(-8)); n != 1 {
		t.Fatal("falta la compensación ligada al conteo, sin renglón, con la sucursal del conteo y la fecha de la entrega")
	}
}

// R5: recibir HOY nunca pregunta por un conteo ya cerrado; un conteo abierto tampoco cuenta.
func TestReceivingTodayNeverAsksAboutACount(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "10", k.day(-3))
	k.closedCount(t, k.tapas, time.Now().Add(-time.Hour))
	pre, err := k.svc.EditExpense(k.ctx, id, app.EditInput{CanReceive: true, Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "1", "10", k.day(0))}}, k.user, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range pre.Preview.Decisions {
		if d.Kind == "count" {
			t.Fatalf("recibir hoy preguntó por el conteo: %+v", d)
		}
	}
	// Con fecha anterior al conteo, sí pregunta, y «ajustar» suma.
	pre, err = k.svc.EditExpense(k.ctx, id, app.EditInput{CanReceive: true, Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "1", "10", k.day(-2))}}, k.user, true)
	if err != nil {
		t.Fatal(err)
	}
	asked := false
	for _, d := range pre.Preview.Decisions {
		asked = asked || d.Kind == "count"
	}
	if !asked {
		t.Fatal("una entrega anterior a un conteo cerrado no preguntó")
	}
}

// FR-001/002 y criterio 7: el encabezado se corrige con bitácora; la fecha mueve solo la del gasto.
func TestEditingTheHeader(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "30", k.day(-3))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "3", "30", k.day(-2))}}, nil)
	var otherCat, supplier int64
	ctx := context.Background()
	if err := k.st.Pool.QueryRow(ctx, `insert into expense_categories (company_id, name, financial_group) values ($1, 'Desechables 53', 'operacional') returning id`, defaultCompanyID).Scan(&otherCat); err != nil {
		t.Fatal(err)
	}
	if err := k.st.Pool.QueryRow(ctx, `insert into suppliers (company_id, name) values ($1, 'Plásticos 53') returning id`, defaultCompanyID).Scan(&supplier); err != nil {
		t.Fatal(err)
	}
	// Más temprano: no puede quedar después de la entrega del día -2.
	newDate, comment := k.day(-4), "Pedido semanal"
	res := k.confirmEdit(t, id, app.EditInput{Header: &app.EditHeaderInput{SupplierSet: true, SupplierID: &supplier,
		CategoryID: &otherCat, Description: &comment, ExpenseDate: &newDate}}, nil)
	if len(res.Preview.Header) != 4 {
		t.Fatalf("cambios de encabezado = %+v, quiere 4", res.Preview.Header)
	}
	var cat int64
	var sup *int64
	var date, desc, status string
	if err := k.st.Pool.QueryRow(ctx, `select category_id, supplier_id, expense_date::text, description, status::text from expenses where id = $1`, id).
		Scan(&cat, &sup, &date, &desc, &status); err != nil {
		t.Fatal(err)
	}
	if cat != otherCat || sup == nil || *sup != supplier || date != newDate || desc != comment || status != "pagada" {
		t.Fatalf("encabezado = %d %v %s %q %s", cat, sup, date, desc, status)
	}
	if n := k.count(t, `select count(*) from stock_movements where expense_id = $1 and effective_on = $2::date`, id, k.day(-2)); n != 1 {
		t.Fatal("cambiar la fecha del gasto movió la fecha de la entrega")
	}
	if k.audited(t, "expense.edited", id) != 2 {
		t.Fatal("el encabezado no dejó su evento")
	}
	// FR-030: el evento dice qué cambió, con el antes y el después.
	var hasHeader bool
	if err := k.st.Pool.QueryRow(ctx, `select bool_and((after->'header'->'category'->>'before')::bigint = $2
		and (after->'header'->'category'->>'after')::bigint = $3 and after->'header' ? 'expenseDate')
		from audit_events where action = 'expense.edited' and entity_id = $1 and after ? 'header'`, id, k.category, otherCat).Scan(&hasHeader); err != nil {
		t.Fatal(err)
	}
	if !hasHeader {
		t.Fatal("la bitácora no guardó la categoría de antes y la de después")
	}
	future := k.day(1)
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{Header: &app.EditHeaderInput{ExpenseDate: &future}}, k.user, true); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("fecha futura = %v", err)
	}
	// Una categoría de otra empresa no existe para ésta (la llave foránea salta RLS).
	coB := makeCompany(t, k.st, "editar-gasto-b")
	var catB int64
	if err := k.st.Pool.QueryRow(ctx, `insert into expense_categories (company_id, name, financial_group) values ($1, 'Ajena', 'operacional') returning id`, coB).Scan(&catB); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{Header: &app.EditHeaderInput{CategoryID: &catB}}, k.user, true); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("categoría de otra empresa = %v, quiere validación", err)
	}
}

// Revisión M5: un gasto pagado de antes de los pagos (sin ninguno) no se vuelve deuda al editarlo.
func TestALegacyPaidExpenseWithoutPaymentsStaysPaid(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "50", k.day(-2))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "5", "50", "")}}, nil)
	if _, err := k.st.Pool.Exec(context.Background(), `delete from expense_payments where expense_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.fajillas, "Fajillas", "1", "20", "")}}, nil)
	if _, status, _ := k.expenseRow(t, id); status != "pagada" {
		t.Fatalf("un pagado sin pagos quedó %s", status)
	}
}

// Aislamiento (constitución IV): otra empresa, conexión reciclada y sin empresa no editan ni
// reciben un gasto ajeno.
func TestEditingAnotherCompanysExpenseIsIsolated(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "10", k.day(-1))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "1", "10", "")}}, nil)
	item := k.items(t, id)["Tapas"]
	other := makeCompany(t, k.st, "editar-gasto-ajena")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, as *store.Store, ctx context.Context) {
		svc := app.NewBackofficeService(as, nil)
		// No existe para quien no es de la empresa: el mismo 404 que un id que no existe (sin oráculo).
		if _, err := svc.EditExpense(ctx, id, app.EditInput{CanReceive: true, Remove: []int64{item}}, k.user, true); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("editar un gasto ajeno = %v, quiere no encontrado", err)
		}
		if _, err := svc.DeliverExpense(ctx, id, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("1")}}}, k.user, false); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("recibir en un gasto ajeno = %v, quiere no encontrado", err)
		}
	})
	if n := k.count(t, `select count(*) from stock_movements where expense_id = $1`, id); n != 0 {
		t.Fatal("otra empresa movió el almacén de este gasto")
	}
}

// Revisión de código (go-backend-reviewer, 1): quitar un renglón que ya llegó mueve el almacén, así
// que exige también el permiso de recibir, no solo el de editar.
func TestRemovingAReceivedLineNeedsTheReceivePermission(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "120", k.day(-5))
	k.confirmEdit(t, id, app.EditInput{CanReceive: true, Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "5", "100", k.day(-3)), k.line(k.fajillas, "Fajillas", "1", "20", "")}}, nil)
	it := k.items(t, id)
	_, err := k.svc.EditExpense(k.ctx, id, app.EditInput{Remove: []int64{it["Tapas"]}}, k.user, true)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("quitar lo recibido sin permiso de recibir = %v, quiere prohibido", err)
	}
	// Lo que no llegó sí se quita con solo editar: no mueve nada.
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{Remove: []int64{it["Fajillas"]}}, k.user, true); err != nil {
		t.Fatalf("quitar lo no recibido con solo editar: %v", err)
	}
	// Agregar algo que «ya llegó» tampoco pasa sin el permiso, aunque el handler no lo filtre.
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Otra", "1", "1", k.day(0))}}, k.user, true); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("agregar «ya llegó» sin permiso de recibir = %v", err)
	}
}

// Revisión de código (go-backend-reviewer, 2): anular un gasto con un «No mover» deshace también la
// compensación, o las existencias quedarían por debajo de lo que dejó el conteo.
func TestCancellingAfterKeepLeavesTheCountedStock(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	k.closedCount(t, k.tapas, time.Now().Add(-48*time.Hour))
	before := onHandIngredient(t, k.st, k.tapas)
	id, err := k.svc.CreateExpense(k.ctx, app.ExpenseInput{ExpenseDay: k.day(-5), CategoryID: k.category, Amount: dd("100"),
		Status: domain.ExpensePendiente, Description: "Por pagar", UserID: k.user})
	if err != nil {
		t.Fatal(err)
	}
	k.confirmEdit(t, id, app.EditInput{CanReceive: true, Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "50", "100", k.day(-4))}},
		map[string]string{"count:" + strconv.FormatInt(k.tapas, 10): "keep"})
	if got := onHandIngredient(t, k.st, k.tapas); !got.Equal(before) {
		t.Fatalf("con «No mover» lo guardado cambió de %s a %s", before, got)
	}
	if err := k.svc.CancelExpense(k.ctx, id, "prueba", k.user); err != nil {
		t.Fatal(err)
	}
	if got := onHandIngredient(t, k.st, k.tapas); !got.Equal(before) {
		t.Fatalf("anular dejó lo guardado en %s; el conteo lo había dejado en %s", got, before)
	}
}

// Revisión de código (go-backend-reviewer, 3, 4 y 8): parámetros presentes y malformados se rechazan.
func TestEditRejectsMalformedValues(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "30", k.day(-5))
	k.confirmEdit(t, id, app.EditInput{CanReceive: true, Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "3", "30", k.day(-3))}}, nil)
	empty := ""
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{Header: &app.EditHeaderInput{ExpenseDate: &empty}}, k.user, true); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("fecha del gasto vacía = %v, quiere validación (no cae a hoy)", err)
	}
	after := k.day(-1)
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{Header: &app.EditHeaderInput{ExpenseDate: &after}}, k.user, true); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("fecha del gasto posterior a su entrega = %v, quiere validación", err)
	}
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.fajillas, "Fajillas", "5", "1", "")}}, map[string]string{"money": "leave"})
	item := k.items(t, id)["Fajillas"]
	if _, err := k.svc.DeliverExpense(k.ctx, id, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("2"), ExtraAmount: ptrDec(dd("5"))}}}, k.user, true); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("importe de lo de más sin que llegara de más = %v, quiere validación", err)
	}
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{Remove: []int64{item}, Decisions: map[string]string{"count:1": "quizas"}}, k.user, true); err != nil {
		// Una llave de decisión que la vista previa no pidió se ignora; un valor desconocido para una
		// que sí pide, se rechaza (TestEditDecisionsRejectUnknown).
		t.Fatalf("decisión que no aplica: %v", err)
	}
}

// Revisión de código (go-backend-reviewer, 5): un renglón cuyos movimientos suman más que lo aplicado
// no se «revierte» sumando mercancía: es una inconsistencia y se rechaza.
func TestRemovingALineWithInconsistentMovementsNeverAddsStock(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "120", k.day(-5))
	k.confirmEdit(t, id, app.EditInput{CanReceive: true, Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "5", "100", k.day(-3))}}, map[string]string{})
	item := k.items(t, id)["Tapas"]
	if _, err := k.st.Pool.Exec(context.Background(), `update expense_items set applied_qty_base = 2 where id = $1`, item); err != nil {
		t.Fatal(err)
	}
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{CanReceive: true, Remove: []int64{item}}, k.user, true); err == nil {
		t.Fatal("quitó un renglón cuyos movimientos no cuadran con lo aplicado")
	}
	if n := k.count(t, `select count(*) from stock_movements where expense_item_id = $1 and quantity > 0 and reason like 'renglón quitado%'`, item); n != 0 {
		t.Fatal("un borrado sumó mercancía")
	}
}

// Revisión de código (go-backend-reviewer, 6): una edición solo de encabezado no toma los insumos, así
// que no choca con una venta en curso.
func TestEditingOnlyTheHeaderDoesNotLockIngredients(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "30", k.day(-5))
	k.confirmEdit(t, id, app.EditInput{CanReceive: true, Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "3", "30", k.day(-3))}}, nil)
	ctx := context.Background()
	tx, err := k.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `select 1 from ingredients where id = $1 for update`, k.tapas); err != nil {
		t.Fatal(err)
	}
	comment := "Solo el comentario"
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{Header: &app.EditHeaderInput{Description: &comment}}, k.user, true); err != nil {
		t.Fatalf("editar solo el comentario chocó con una venta en curso: %v", err)
	}
}

// Revisión de seguridad (1 y 3): una referencia de otra empresa o inexistente es un 400/404, nunca un
// 500, y nunca entra. La llave foránea salta RLS por diseño; el servicio pregunta bajo RLS.
func TestEditRejectsReferencesFromAnotherCompany(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "100", k.day(-2))
	coB := makeCompany(t, k.st, "editar-gasto-refs-b")
	ctx := context.Background()
	ingB := ingredientIn(t, k.st, coB, "Ajeno", "pieza")
	var prodB, supB int64
	if err := k.st.Pool.QueryRow(ctx, `insert into products (company_id, name, price, category_id)
		select $1, 'Ajeno', 10, (select id from categories where company_id = $1 limit 1) returning id`, coB).Scan(&prodB); err != nil {
		t.Logf("sin producto ajeno: %v", err)
	}
	if err := k.st.Pool.QueryRow(ctx, `insert into suppliers (company_id, name) values ($1, 'Ajeno') returning id`, coB).Scan(&supB); err != nil {
		t.Fatal(err)
	}
	bad := func(name string, in app.EditInput) {
		t.Helper()
		_, err := k.svc.EditExpense(k.ctx, id, in, k.user, true)
		if !errors.Is(err, domain.ErrValidation) && !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("%s = %v, quiere 400 o 404", name, err)
		}
	}
	bad("insumo ajeno", app.EditInput{Add: []app.EditLineInput{k.line(ingB, "Ajeno", "1", "10", "")}})
	if prodB != 0 {
		bad("producto ajeno", app.EditInput{Add: []app.EditLineInput{{ItemType: "producto", ProductID: &prodB, Description: "Ajeno",
			Quantity: dd("1"), UnitID: &k.pieza, Amount: dd("10")}}})
	}
	nope := int16(32000)
	bad("unidad que no existe", app.EditInput{Add: []app.EditLineInput{{ItemType: "ingrediente", IngredientID: &k.tapas, Description: "Tapas",
		Quantity: dd("1"), UnitID: &nope, Amount: dd("10")}}})
	bad("proveedor ajeno", app.EditInput{Header: &app.EditHeaderInput{SupplierSet: true, SupplierID: &supB}})
	if n := k.count(t, `select count(*) from expense_items where expense_id = $1`, id); n != 0 {
		t.Fatal("entró un renglón con una referencia ajena")
	}
}

// Prueba con usuario nuevo (spec 053): un gasto de $67.50 pagado que sube a $77.50 debe $10. Pagar
// el total otra vez se rechaza; pagar el saldo lo deja pagado con un solo peso de más: ninguno.
func TestPayingAnEditedExpensePaysOnlyWhatIsDue(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "67.50", k.day(-2))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "5", "77.50", "")}}, map[string]string{"money": "leave"})
	if err := k.svc.AddExpensePayment(k.ctx, id, app.ExpensePaymentInput{MethodID: k.transfer, Amount: dd("77.50")}, k.user); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("pagar el total otra vez = %v, quiere validación", err)
	}
	if err := k.svc.AddExpensePayment(k.ctx, id, app.ExpensePaymentInput{MethodID: k.transfer, Amount: dd("10")}, k.user); err != nil {
		t.Fatal(err)
	}
	if amount, status, _ := k.expenseRow(t, id); !amount.Equal(dd("77.50")) || status != "pagada" {
		t.Fatalf("gasto %s %s, quiere 77.50 pagada", amount, status)
	}
	if p := k.payments(t, id); len(p) != 2 || p[1] != "10.00" {
		t.Fatalf("pagos = %v", p)
	}
}
