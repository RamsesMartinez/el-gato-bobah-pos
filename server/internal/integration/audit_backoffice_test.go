//go:build integration

package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// actionsBy devuelve las acciones registradas por `actor`, en orden.
func actionsBy(t *testing.T, st *store.Store, actor int64) []string {
	t.Helper()
	rows, err := st.Pool.Query(context.Background(), `select action from audit_events where actor_user_id = $1 order by id`, actor)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a string
		_ = rows.Scan(&a)
		out = append(out, a)
	}
	return out
}

func TestCashRegisterActionsAreAudited(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cashier := makeUser(t, st, "cajero_bitacora", "cajero")
	reg := registerID(t, st, "Caja principal")

	if _, err := back.OpenSession(ctx, reg, aperturaAMano(dec("500")), cashier); err != nil {
		t.Fatalf("abrir: %v", err)
	}
	var iceConcept int64
	if err := st.Pool.QueryRow(ctx, `select id from cash_concepts where company_id = $1 and name = 'Hielo'`, defaultCompanyID).Scan(&iceConcept); err != nil {
		t.Fatalf("concepto Hielo: %v", err)
	}
	if _, err := back.RecordCashMovement(ctx, reg, app.CashMovementCmd{Kind: domain.CashSalida, Amount: dec("50"), ConceptID: &iceConcept, UserID: cashier}); err != nil {
		t.Fatalf("salida: %v", err)
	}
	var mov int64
	if err := st.Pool.QueryRow(ctx, `select id from register_cash_movements where kind = 'salida' order by id desc limit 1`).Scan(&mov); err != nil {
		t.Fatal(err)
	}
	if _, err := back.CorrectCashOut(ctx, mov, app.CashMovementCmd{Kind: domain.CashSalida, Amount: dec("45"), ConceptID: &iceConcept, UserID: cashier}); err != nil {
		t.Fatalf("corregir la salida: %v", err)
	}
	total := dec("455")
	if _, err := back.CloseSession(ctx, reg, cashier, app.CierreCmd{Total: &total, Motivo: "Conté a mano"}); err != nil {
		t.Fatalf("cerrar: %v", err)
	}

	want := "cash.opened cash.movement cash.movement_corrected cash.closed"
	if got := strings.Join(actionsBy(t, st, cashier), " "); got != want {
		t.Fatalf("bitácora de caja = %q, quiere %q", got, want)
	}
	var branch *int64
	var before, after string
	if err := st.Pool.QueryRow(ctx, `select branch_id, before->>'amount', after->>'amount' from audit_events
		where action = 'cash.movement_corrected'`).Scan(&branch, &before, &after); err != nil {
		t.Fatal(err)
	}
	if branch == nil || before != "50" || after != "45" {
		t.Fatalf("la corrección no guarda sucursal o antes → después: %v %s %s", branch, before, after)
	}
}

func TestExpensesAndTipPayoutsAreAudited(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	orders := app.NewOrdersService(st, clock)
	tips := app.NewTipsService(st, clock)
	admin := makeUser(t, st, "admin_bitacora", "admin")
	abrirCajaPrincipal(t, st, admin)
	reg := principalRegister(t, st)
	cash := paymentMethodID(t, st, "Efectivo")

	var cat int64
	if err := st.Pool.QueryRow(ctx, `insert into expense_categories (name, financial_group) values ('Insumos bitácora', 'operacional') returning id`).Scan(&cat); err != nil {
		t.Fatal(err)
	}
	pending, err := back.CreateExpense(ctx, app.ExpenseInput{CategoryID: cat, Amount: dec("120"), Status: domain.ExpensePendiente, UserID: admin})
	if err != nil {
		t.Fatalf("crear gasto: %v", err)
	}
	if err := back.AddExpensePayment(ctx, pending, app.ExpensePaymentInput{MethodID: cash, Amount: dec("120"), RegisterID: &reg}, admin); err != nil {
		t.Fatalf("pagar gasto: %v", err)
	}
	otherUser, err := back.CreateExpense(ctx, app.ExpenseInput{CategoryID: cat, Amount: dec("30"), Status: domain.ExpensePendiente, UserID: admin})
	if err != nil {
		t.Fatal(err)
	}
	if err := back.CancelExpense(ctx, otherUser, "Capturado dos veces", admin); err != nil {
		t.Fatalf("cancelar gasto: %v", err)
	}

	cobrarConPropina(t, ctx, st, orders, "bitácora", "100", "20", admin, cash)
	if _, err := tips.Payout(ctx, reg, app.TipPayoutCmd{Mode: "parejo", ActorID: admin,
		Recipients: []app.TipRecipient{{UserID: admin}}}); err != nil {
		t.Fatalf("entregar propina: %v", err)
	}

	got := " " + strings.Join(actionsBy(t, st, admin), " ") + " "
	for _, a := range []string{"expense.created", "expense.paid", "expense.cancelled"} {
		if !strings.Contains(got, " "+a+" ") {
			t.Errorf("falta %s en la bitácora: %s", a, got)
		}
	}
	var n int
	if err := st.Pool.QueryRow(ctx, `select count(*) from audit_events where action = 'tip.paid_out' and branch_id is not null`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("la entrega de propina no dejó evento con sucursal: %d %v", n, err)
	}
}

func TestChangingTheCounterPriceKeepsOldAndNew(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := app.WithActorUser(context.Background(), makeUser(t, st, "gerente_precio", "gerente"))
	frappe := makeProduct(t, st, "Frappé Taro G", dec("80"), false)
	admin := app.NewAdminService(st)
	in := app.UpdateProductInput{ID: frappe, Name: "Frappé Taro G", Price: dec("85"), Active: true}
	if err := admin.UpdateProduct(ctx, in); err != nil {
		t.Fatalf("UpdateProduct: %v", err)
	}
	// Guardar sin mover el precio (renombrar) no es un cambio de precio.
	in.Name = "Frappé de Taro G"
	if err := admin.UpdateProduct(ctx, in); err != nil {
		t.Fatal(err)
	}
	var n int
	var before, after string
	if err := st.Pool.QueryRow(context.Background(), `select count(*), min(before->>'price'), min(after->>'price')
		from audit_events where action = 'price.set' and entity_id = $1 and actor_user_id is not null`, frappe).Scan(&n, &before, &after); err != nil {
		t.Fatal(err)
	}
	if n != 1 || before != "80" || after != "85" {
		t.Fatalf("eventos de precio = %d (%s → %s), quiere 1 (80 → 85)", n, before, after)
	}
}

// La lista y sus contadores salen del MISMO filtro: la suma de los contadores es el total sin
// filtro de tipo, y filtrar por persona deja solo lo suyo.
func TestTheAuditLogFiltersAndMatchesItsCounters(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	k.ctx = app.WithActorUser(k.ctx, k.user)
	otherUser := makeUser(t, k.st, "otro_bitacora", "cajero")
	k.order(t, "1", "100", true)
	k.order(t, "1", "0", false)
	otherCtx := app.WithActorUser(k.tenant(t, defaultCompanyID), otherUser)
	if _, err := k.orders.Create(otherCtx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: otherUser,
		Lines: []domain.OrderLineInput{{ProductID: k.product, Qty: dec("1")}}}); err != nil {
		t.Fatal(err)
	}

	svc := app.NewAuditService(k.appSt)
	// occurred_at lo pone el reloj de la base, no el de las pruebas: el rango se arma sobre hoy real.
	today := time.Now()
	theDay := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	whole, err := svc.List(k.ctx, domain.AuditFilter{From: theDay.AddDate(0, 0, -1), To: theDay.AddDate(0, 0, 1), Size: 25})
	if err != nil {
		t.Fatal(err)
	}
	var sum int64
	for _, n := range whole.CountsByGroup {
		sum += n
	}
	if whole.Total < 5 || sum != whole.Total || int64(len(whole.Items)) != whole.Total {
		t.Fatalf("contadores %v (suma %d), total %d, renglones %d: no cuadran", whole.CountsByGroup, sum, whole.Total, len(whole.Items))
	}
	onlyOther, err := svc.List(k.ctx, domain.AuditFilter{From: theDay.AddDate(0, 0, -1), To: theDay.AddDate(0, 0, 1), Size: 25, ActorUserID: &otherUser})
	if err != nil {
		t.Fatal(err)
	}
	if onlyOther.Total != 1 || onlyOther.Items[0].Action != "order.created" {
		t.Fatalf("filtrar por persona = %+v", onlyOther)
	}
	charges, err := svc.List(k.ctx, domain.AuditFilter{From: theDay.AddDate(0, 0, -1), To: theDay.AddDate(0, 0, 1), Size: 25, Group: domain.AuditGroupMoney, Action: "payment.charged"})
	if err != nil {
		t.Fatal(err)
	}
	if charges.Total != 1 || charges.CountsByGroup["money"] != whole.CountsByGroup["money"] {
		t.Fatalf("filtrar por cobros = %d; los contadores no deben vaciarse al filtrar: %v", charges.Total, charges.CountsByGroup)
	}
	if _, err := svc.List(k.ctx, domain.AuditFilter{From: theDay, To: theDay, Size: 30}); err == nil {
		t.Fatal("un tamaño de página inválido no se rechazó")
	}

	inTheThreeCases(t, defaultCompanyID, makeCompany(t, k.st, "bitacora-otra"), func(t *testing.T, s *store.Store, ctx context.Context) {
		p, err := app.NewAuditService(s).List(ctx, domain.AuditFilter{From: theDay.AddDate(0, 0, -1), To: theDay.AddDate(0, 0, 1), Size: 25})
		if err == nil && p.Total > 0 {
			t.Fatalf("se leyeron %d eventos de otra empresa", p.Total)
		}
	})
}

// Quitar un precio de plataforma deja el que había, para poder reponerlo.
func TestDeletingAPlatformPriceKeepsWhatItWas(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	manager := makeUser(t, st, "gerente_quita_precio", "gerente")
	ctx := app.WithActorUser(context.Background(), manager)
	mango := makeProduct(t, st, "Chamoyada de mango", dec("80"), false)
	plat := plataformaUber(t, st, defaultCompanyID)
	prices := app.NewPlatformPricesService(st)
	if err := prices.SetProductPrice(ctx, mango, plat, dec("110"), manager); err != nil {
		t.Fatal(err)
	}
	if _, err := prices.DeleteProductPrice(ctx, mango, plat); err != nil {
		t.Fatal(err)
	}
	var before string
	var actor *int64
	if err := st.Pool.QueryRow(context.Background(), `select before->>'price', actor_user_id from audit_events
		where action = 'price.deleted' and entity_id = $1`, mango).Scan(&before, &actor); err != nil {
		t.Fatalf("quitar el precio no dejó evento: %v", err)
	}
	if before != "110" || actor == nil || *actor != manager {
		t.Fatalf("antes = %s, autor = %v", before, actor)
	}
}

// EL FILTRO «APARATO» OFRECE LOS DE TODO EL PERIODO (FR-016). Sacarlos de la página visible dejaba
// fuera al aparato que solo actuó en la página dos, que es justo el que se busca; y elegir uno no
// debe borrar a los demás de la lista, o no hay forma de cambiar de aparato.
func TestTheDeviceFilterOffersEveryDeviceInTheRange(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "bitacora-aparatos")
	ana := makeUserIn(t, st, co, "ana-aparatos", "gerente")
	keyA, keyB := strings.Repeat("a", 32), strings.Repeat("b", 32)
	for _, key := range []string{keyA, keyA, keyB, ""} {
		if _, err := st.Pool.Exec(context.Background(), `insert into audit_events (company_id, actor_kind, actor_user_id, action, entity_type, entity_id, device_key)
			values ($1, 'user', $2, 'price.set', 'product', 1, nullif($3, ''))`, co, ana, key); err != nil {
			t.Fatal(err)
		}
	}
	k := &draftsKit{appSt: appRoleStore(t)}
	ctx := k.tenant(t, co)
	today := time.Now()
	theDay := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	svc := app.NewAuditService(k.appSt)
	for _, key := range []string{"", keyB} {
		p, err := svc.List(ctx, domain.AuditFilter{From: theDay.AddDate(0, 0, -1), To: theDay.AddDate(0, 0, 1), Size: 25, Page: 0, DeviceKey: key})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(p.DeviceKeys, ",") != keyA+","+keyB {
			t.Errorf("con el filtro %q el selector ofrece %v, quiere los dos", key, p.DeviceKeys)
		}
	}
}

// CADA RENGLÓN DE PEDIDO DICE DE QUÉ PEDIDO ES, con el mismo nombre que Ventas («Mesa 4 · #12»).
// Sin eso, la bitácora no se puede cruzar con Ventas ni con el ticket.
func TestAuditRowsNameTheirOrderLikeSales(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	k.ctx = app.WithActorUser(k.ctx, k.user)
	o := k.order(t, "1", "0", false)
	var number int32
	var folio string
	if err := k.st.Pool.QueryRow(context.Background(), `select daily_number, coalesce(folio_name, '') from orders where id = $1`, o.ID).Scan(&number, &folio); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("Venta #%d", number)
	if folio != "" {
		want = fmt.Sprintf("%s · #%d", folio, number)
	}
	now := time.Now()
	p, err := app.NewAuditService(k.appSt).List(k.ctx, domain.AuditFilter{From: now.AddDate(0, 0, -1), To: now.AddDate(0, 0, 1), Size: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range p.Items {
		if e.OrderID != nil && *e.OrderID == o.ID {
			if e.OrderLabel != want {
				t.Fatalf("el renglón del pedido dice %q, Ventas dice %q", e.OrderLabel, want)
			}
			return
		}
	}
	t.Fatal("el pedido no aparece en la bitácora")
}
