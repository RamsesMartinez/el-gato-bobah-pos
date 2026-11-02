//go:build integration

package integration

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Bordes del motor de edición de gastos (spec 053) que ninguna prueba veía: cada una se escribió
// después de romper a propósito la regla que cuida y ver que nada fallaba.

// Un gasto anulado no se edita ni se recibe: su mercancía ya salió del almacén al anularlo, y
// recibirla otra vez sumaría lo que nadie compró.
func TestACancelledExpenseIsNeitherEditedNorReceived(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id, err := k.svc.CreateExpense(k.ctx, app.ExpenseInput{ExpenseDay: k.day(-1), CategoryID: k.category, Amount: dd("50"),
		Status: domain.ExpensePendiente, Description: "Por anular", UserID: k.user,
		Items: []app.ExpenseItemInput{{ItemType: "ingrediente", IngredientID: &k.tapas, Description: "Tapas",
			Quantity: dd("5"), UnitID: &k.pieza, Amount: dd("50")}}})
	if err != nil {
		t.Fatal(err)
	}
	item := k.items(t, id)["Tapas"]
	if err := k.svc.CancelExpense(k.ctx, id, "prueba", k.user); err != nil {
		t.Fatal(err)
	}
	comment := "después de anular"
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{Header: &app.EditHeaderInput{Description: &comment}}, k.user, true); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("editar un gasto anulado = %v, quiere conflicto", err)
	}
	if _, err := k.svc.DeliverExpense(k.ctx, id, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("5")}}}, k.user, true); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("recibir en un gasto anulado = %v, quiere conflicto", err)
	}
	if n := k.count(t, `select count(*) from stock_movements where expense_id = $1 and quantity > 0`, id); n != 0 {
		t.Fatalf("un gasto anulado sumó %d movimientos", n)
	}
}

// FR-029 con dos tabletas a la vez: la otra edición tiene el gasto tomado y cambia el importe. La
// confirmación espera a que termine (FOR UPDATE) y entonces ya no es lo que se previsualizó. Sin el
// candado, leía el gasto de antes, la huella coincidía y escribía encima de la otra.
func TestAConfirmationWaitsForAnotherEditAndThenIsStale(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	ctx := context.Background()
	id := k.paidWithoutGoods(t, "100", k.day(-2))
	in := app.EditInput{CanReceive: true, Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "10", "100", "")}}
	pre, err := k.svc.EditExpense(k.ctx, id, in, k.user, true)
	if err != nil {
		t.Fatal(err)
	}
	in.Fingerprint = pre.Preview.Fingerprint

	tx, err := k.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select 1 from expenses where id = $1 for update`, id); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := k.svc.EditExpense(k.ctx, id, in, k.user, false)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("confirmó sin esperar a la otra edición: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := tx.Exec(ctx, `update expenses set amount = amount + 10 where id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, domain.ErrExpenseEditStale) {
			t.Fatalf("confirmar después de otra edición = %v, quiere ErrExpenseEditStale", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("la confirmación no terminó")
	}
	if n := k.count(t, `select count(*) from expense_items where expense_id = $1`, id); n != 0 {
		t.Fatal("se escribió una edición sobre otra que la cambió")
	}
}

// Una venta en curso con el mismo insumo: quitar lo recibido responde «vuelve a intentar» en vez de
// quedarse esperando a la venta (spec 038, los insumos se toman sin esperar ANTES de escribir).
func TestRemovingAReceivedLineDoesNotWaitForASale(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	ctx := context.Background()
	id := k.paidWithoutGoods(t, "120", k.day(-5))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "5", "100", k.day(-3)),
		k.line(k.fajillas, "Fajillas", "1", "20", "")}}, nil)
	item := k.items(t, id)["Tapas"]

	tx, err := k.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `select 1 from stock_levels where ingredient_id = $1 for update`, k.tapas); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := k.svc.EditExpense(k.ctx, id, app.EditInput{CanReceive: true, Remove: []int64{item}}, k.user, true)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("quitar con una venta en curso = %v, quiere conflicto de inmediato", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("quitar lo recibido esperó a la venta")
	}
}

// R5: un conteo ABIERTO no es un conteo posterior; ya ajusta contra lo que había al capturar.
func TestAnOpenCountIsNotAPosteriorCount(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	ctx := context.Background()
	id := k.paidWithoutGoods(t, "10", k.day(-3))
	var countID int64
	if err := k.st.Pool.QueryRow(ctx, `insert into stock_counts (company_id, branch_id, opened_by, status, opened_at)
		select $1, id, $2, 'open', now() - interval '1 hour' from branches where company_id = $1 order by id limit 1 returning id`,
		defaultCompanyID, k.user).Scan(&countID); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `insert into stock_count_lines (count_id, company_id, ingredient_id, counted_qty, system_qty, captured_by, captured_at)
		values ($1, $2, $3, 0, 0, $4, now() - interval '1 hour')`, countID, defaultCompanyID, k.tapas, k.user); err != nil {
		t.Fatal(err)
	}
	pre, err := k.svc.EditExpense(k.ctx, id, app.EditInput{CanReceive: true,
		Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "1", "10", k.day(-2))}}, k.user, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range pre.Preview.Decisions {
		if d.Kind == "count" {
			t.Fatalf("un conteo abierto se tomó por posterior: %+v", d)
		}
	}
}

// Spec 049 dentro de la edición: un renglón excluido no cuenta en el importe, no se quita desde
// aquí y no se recibe. Y una entrega no trae dos veces el mismo renglón.
func TestAnExcludedLineStaysOutOfTheEdit(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id, err := k.svc.CreateExpense(k.ctx, app.ExpenseInput{ExpenseDay: k.day(-2), CategoryID: k.category, Amount: dd("100"),
		Status: domain.ExpensePendiente, Description: "Con algo personal", UserID: k.user,
		Items: []app.ExpenseItemInput{
			{ItemType: "ingrediente", IngredientID: &k.tapas, Description: "Tapas", Quantity: dd("6"), UnitID: &k.pieza, Amount: dd("60")},
			{ItemType: "ingrediente", IngredientID: &k.fajillas, Description: "Fajillas de casa", Quantity: dd("4"), UnitID: &k.pieza, Amount: dd("40")},
		}})
	if err != nil {
		t.Fatal(err)
	}
	it := k.items(t, id)
	if _, err := k.svc.SetItemExcluded(k.ctx, id, it["Fajillas de casa"], true, k.user); err != nil {
		t.Fatal(err)
	}
	// Incluidos: 60 + 50 = 110 sobre un importe de 100. Si contara el excluido serían 150.
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Más tapas", "5", "50", "")}}, nil)
	if amount, _, _ := k.expenseRow(t, id); !amount.Equal(dd("110")) {
		t.Fatalf("importe %s, quiere 110: el renglón excluido no es gasto del negocio", amount)
	}
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{Remove: []int64{it["Fajillas de casa"]}}, k.user, true); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("quitar un excluido = %v, quiere validación (se vuelve a incluir primero)", err)
	}
	if _, err := k.svc.DeliverExpense(k.ctx, id, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: it["Fajillas de casa"], Qty: dd("4")}}}, k.user, true); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("recibir un excluido = %v, quiere no encontrado", err)
	}
	twice := []app.DeliveryLineInput{{ItemID: it["Tapas"], Qty: dd("3")}, {ItemID: it["Tapas"], Qty: dd("3")}}
	if _, err := k.svc.DeliverExpense(k.ctx, id, app.DeliveryInput{Lines: twice}, k.user, true); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("el mismo renglón dos veces en una entrega = %v, quiere validación", err)
	}
}

// FR-030: el evento dice QUÉ cambió, no solo que algo cambió: lo agregado con su fecha de llegada,
// lo quitado con su renglón, la decisión tomada ante el conteo y cada entrega con su día.
func TestTheAuditEventSaysWhatTheEditChanged(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	ctx := context.Background()
	id := k.paidWithoutGoods(t, "120", k.day(-10))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "50", "100", k.day(-8)),
		k.line(k.fajillas, "Fajillas", "2", "20", "")}}, nil)
	it := k.items(t, id)
	k.closedCount(t, k.tapas, time.Now().Add(-48*time.Hour))
	key := "count:" + strconv.FormatInt(k.tapas, 10)
	k.confirmEdit(t, id, app.EditInput{Remove: []int64{it["Tapas"]}}, map[string]string{key: "keep"})
	k.confirmDelivery(t, id, app.DeliveryInput{On: k.day(-1), Lines: []app.DeliveryLineInput{{ItemID: it["Fajillas"], Qty: dd("2")}}}, nil)

	has := func(what, sql string, args ...any) {
		t.Helper()
		var ok bool
		if err := k.st.Pool.QueryRow(ctx, `select exists (select 1 from audit_events where entity_id = $1 and `+sql+`)`, append([]any{id}, args...)...).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("la bitácora no guardó %s", what)
		}
	}
	has("el renglón agregado con su fecha de llegada", `action = 'expense.edited' and after->'added' @> $2::jsonb`,
		`[{"description":"Tapas","arrivedOn":"`+k.day(-8)+`"}]`)
	has("el renglón quitado", `action = 'expense.edited' and after->'removed' @> $2::jsonb`,
		`[{"itemId":`+strconv.FormatInt(it["Tapas"], 10)+`}]`)
	has("la decisión ante el conteo", `action = 'expense.edited' and after->'decisions' @> $2::jsonb`,
		`[{"key":"`+key+`","chosen":"keep"}]`)
	has("la entrega con su día", `action = 'expense.received' and after->'lines' @> $2::jsonb`,
		`[{"itemId":`+strconv.FormatInt(it["Fajillas"], 10)+`,"on":"`+k.day(-1)+`"}]`)
}

// FR-002 y constitución V en un gasto SIN entregas: ahí no hay una llegada que tape la regla, así
// que la fecha futura y la fecha vacía tienen que rechazarse por sí mismas.
func TestTheExpenseDateRulesHoldWithoutDeliveries(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "30", k.day(-3))
	future, empty := k.day(1), ""
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{Header: &app.EditHeaderInput{ExpenseDate: &future}}, k.user, true); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("fecha futura sin entregas = %v, quiere validación", err)
	}
	if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{Header: &app.EditHeaderInput{ExpenseDate: &empty}}, k.user, true); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("fecha vacía sin entregas = %v, quiere validación (no cae a hoy)", err)
	}
}

// FR-002: la fecha del gasto no puede quedar después de una llegada... de un renglón VIVO. Lo que
// llegó de un renglón ya quitado no ata la fecha.
func TestARemovedDeliveryDoesNotPinTheExpenseDate(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	ctx := context.Background()
	id := k.paidWithoutGoods(t, "120", k.day(-5))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "5", "100", k.day(-4)),
		k.line(k.fajillas, "Fajillas", "1", "20", "")}}, nil)
	k.confirmEdit(t, id, app.EditInput{Remove: []int64{k.items(t, id)["Tapas"]}}, nil)
	newDate := k.day(-2)
	k.confirmEdit(t, id, app.EditInput{Header: &app.EditHeaderInput{ExpenseDate: &newDate}}, nil)
	var date string
	if err := k.st.Pool.QueryRow(ctx, `select expense_date::text from expenses where id = $1`, id).Scan(&date); err != nil {
		t.Fatal(err)
	}
	if date != newDate {
		t.Fatalf("fecha del gasto %s, quiere %s", date, newDate)
	}
}

// FR-021 con importe sin desglosar: lo pagado por lo que llegó de más sube el importe del gasto,
// no se lo come lo que el importe tenía sin desglosar.
func TestWhatArrivesOverRaisesTheAmountAboveTheUnbrokenPart(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "70", k.day(-2))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "5", "50", "")}}, nil)
	item := k.items(t, id)["Tapas"]
	k.confirmDelivery(t, id, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("6"), ExtraAmount: ptrDec(dd("10"))}}},
		map[string]string{"money": "leave"})
	amount, status, _ := k.expenseRow(t, id)
	if !amount.Equal(dd("80")) || status != "pendiente" {
		t.Fatalf("gasto %s %s, quiere 80 pendiente: $70 de antes más $10 de lo que llegó de más", amount, status)
	}
	if p := k.payments(t, id); len(p) != 1 || p[0] != "70.00" {
		t.Fatalf("pagos = %v: el pago original cambió", p)
	}
}

// FR-017/018: lo que elige quien recibe gana a la preselección, y la vista previa dice cuánto falta.
func TestTheReceiverChoiceOverridesThePreselection(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	ctx := context.Background()
	id := k.paidWithoutGoods(t, "50", k.day(-3))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "5", "50", "")}}, nil)
	item := k.items(t, id)["Tapas"]
	pre, err := k.svc.DeliverExpense(k.ctx, id, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("3")}}}, k.user, true)
	if err != nil {
		t.Fatal(err)
	}
	missing := false
	for _, l := range pre.Preview.Lines {
		missing = missing || (l.Kind == "received" && l.MissingAfter != nil && l.MissingAfter.Equal(dd("2")))
	}
	if !missing {
		t.Fatalf("la vista previa no dijo que faltan 2: %+v", pre.Preview.Lines)
	}
	// Por pieza se preselecciona «Sigue pendiente»; quien recibe elige ajustar.
	k.confirmDelivery(t, id, app.DeliveryInput{Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("3"), Shortfall: "adjust"}}}, nil)
	var qty, lineAmount string
	if err := k.st.Pool.QueryRow(ctx, `select quantity::text, amount::text from expense_items where id = $1`, item).Scan(&qty, &lineAmount); err != nil {
		t.Fatal(err)
	}
	if qty != "3.0000" || lineAmount != "30.00" {
		t.Fatalf("renglón %s por %s, quiere 3 por 30: la elección de quien recibe no se respetó", qty, lineAmount)
	}
	if amount, _, received := k.expenseRow(t, id); !amount.Equal(dd("30")) || received == nil {
		t.Fatalf("gasto %s recibido %v, quiere 30 y recibido", amount, received)
	}
}

// FR-019: «recibido» lleva el día de la última entrega de los renglones VIVOS; la de un renglón
// quitado no cuenta.
func TestReceivedAtIgnoresARemovedLinesDelivery(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "100", k.day(-5))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "5", "50", ""),
		k.line(k.fajillas, "Fajillas", "1", "50", k.day(0))}}, nil)
	it := k.items(t, id)
	k.confirmEdit(t, id, app.EditInput{Remove: []int64{it["Fajillas"]}}, nil)
	k.confirmDelivery(t, id, app.DeliveryInput{On: k.day(-1), Lines: []app.DeliveryLineInput{{ItemID: it["Tapas"], Qty: dd("5")}}}, nil)
	if _, _, received := k.expenseRow(t, id); received == nil || *received != k.day(-1) {
		t.Fatalf("recibido %v, quiere %s (la entrega de hoy era de un renglón quitado)", received, k.day(-1))
	}
}

// FR-024: quitar lo recibido puede dejar existencias negativas; se permite y la vista previa lo
// marca. Pasa cuando ya se vendió lo que se está quitando.
func TestRemovingCanLeaveNegativeStockAndThePreviewSaysSo(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "120", k.day(-5))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "5", "100", k.day(-3)),
		k.line(k.fajillas, "Fajillas", "1", "20", "")}}, nil)
	if _, err := k.st.Pool.Exec(context.Background(), `insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity)
		values ($1, 'ingrediente', $2, 'ajuste', -8)`, defaultCompanyID, k.tapas); err != nil {
		t.Fatal(err)
	}
	res := k.confirmEdit(t, id, app.EditInput{Remove: []int64{k.items(t, id)["Tapas"]}}, nil)
	var tapas *app.StockEffect
	for i := range res.Preview.Inventory {
		if e := res.Preview.Inventory[i]; e.IngredientID != nil && *e.IngredientID == k.tapas {
			tapas = &res.Preview.Inventory[i]
		}
	}
	if tapas == nil || !tapas.After.Equal(dd("-8")) || !tapas.NegativeAfter {
		t.Fatalf("la vista previa no marcó las existencias negativas: %+v", tapas)
	}
	if got := onHandIngredient(t, k.st, k.tapas); !got.Equal(dd("-8")) {
		t.Fatalf("existencias %s, quiere -8", got)
	}
}

// FR-025: el grupo Costos muestra el costo por unidad antes y después de lo que entra.
func TestReceivingShowsTheCostBeforeAndAfter(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "50", k.day(-2))
	res := k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "10", "50", k.day(0))}}, nil)
	if len(res.Preview.Costs) != 1 || res.Preview.Costs[0].IngredientID != k.tapas ||
		!res.Preview.Costs[0].Before.IsZero() || !res.Preview.Costs[0].After.Equal(dd("5")) {
		t.Fatalf("costos = %+v, quiere tapas de 0 a 5 por pieza", res.Preview.Costs)
	}
}

// FR-022 por el camino de la ENTREGA (no solo al agregar con «Ya llegó»): una entrega fechada antes
// de un conteo cerrado pregunta, y «No mover» deja las existencias como las dejó el conteo.
func TestADeliveryBeforeAClosedCountAsksAndKeeps(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "50", k.day(-5))
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{k.line(k.tapas, "Tapas", "5", "50", "")}}, nil)
	item := k.items(t, id)["Tapas"]
	countID := k.closedCount(t, k.tapas, time.Now().Add(-48*time.Hour))
	in := app.DeliveryInput{On: k.day(-4), Lines: []app.DeliveryLineInput{{ItemID: item, Qty: dd("5")}}}
	pre, err := k.svc.DeliverExpense(k.ctx, id, in, k.user, true)
	if err != nil {
		t.Fatal(err)
	}
	var key string
	for _, d := range pre.Preview.Decisions {
		if d.Kind == "count" && d.CountID != nil && *d.CountID == countID {
			key = d.Key
		}
	}
	if key == "" {
		t.Fatalf("una entrega anterior al conteo %d no preguntó: %+v", countID, pre.Preview.Decisions)
	}
	before := onHandIngredient(t, k.st, k.tapas)
	k.confirmDelivery(t, id, in, map[string]string{key: "keep"})
	if after := onHandIngredient(t, k.st, k.tapas); !after.Equal(before) {
		t.Fatalf("con «No mover» las existencias cambiaron de %s a %s", before, after)
	}
}

// Borde de la spec: un renglón sin artículo («otros») no se regala, no trae empaque y no «llega»:
// no hay nada que guardar, y aceptarlo dejaría un renglón recibido que el almacén nunca vio.
func TestALineWithoutAnArticleIsNeitherAGiftNorArrives(t *testing.T) {
	t.Parallel()
	k := newEditKit(t)
	id := k.paidWithoutGoods(t, "30", k.day(-2))
	for name, l := range map[string]app.EditLineInput{
		"regalo":   {Description: "Flete", Quantity: dd("1"), IsGift: true},
		"ya llegó": {Description: "Flete", Quantity: dd("1"), Amount: dd("30"), ArrivedOn: k.day(0)},
		"empaque":  {Description: "Flete", Quantity: dd("1"), Amount: dd("30"), PresentationID: &k.bulto},
	} {
		if _, err := k.svc.EditExpense(k.ctx, id, app.EditInput{CanReceive: true, Add: []app.EditLineInput{l}}, k.user, true); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("renglón sin artículo con %s = %v, quiere validación", name, err)
		}
	}
	// Uno normal sí entra: el rechazo es por lo que trae, no por no tener artículo.
	k.confirmEdit(t, id, app.EditInput{Add: []app.EditLineInput{{Description: "Flete", Quantity: dd("1"), Amount: dd("30")}}}, nil)
}
