//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// MANDAR A COCINA CONVIERTE LA CUENTA EN PEDIDO POR EL CAMINO DE HOY (FR-005, D-2, D-6, research R-4).
func TestSendCreatesTheOrder(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	ctx := context.Background()
	abrirCajaPrincipal(t, k.st, k.user)
	sender := makeUser(t, k.st, "cajero_que_envia", "cajero")
	jefa := makeUser(t, k.st, "gerente_que_descuenta", "gerente")
	taro := makeProduct(t, k.st, "Taro enviado", pesos("55"), true)
	crepa := makeProduct(t, k.st, "Crepa enviada", pesos("80"), true)
	uber := platformID(t, k.st, defaultCompanyID, "Uber Eats")

	v := k.newDraftNamed(t, "Persa", addOf(taro, "2"), addOf(crepa, "1"))
	if _, err := k.drafts.PatchHeader(k.ctx, v.ID, app.DraftHeaderPatch{ExpectedVersion: v.HeaderVersion,
		PlatformID: app.Field[int16]{Set: true, Value: &uber}, PlatformOrderRef: app.Field[string]{Set: true, Value: new("UB-77")},
		Discount: app.Field[app.DraftDiscount]{Set: true, Value: &app.DraftDiscount{Amount: new(pesos("10"))}}}, jefa); err != nil {
		t.Fatal(err)
	}

	res, err := k.drafts.Send(k.ctx, v.ID, sender)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	o := res.Order
	if !res.Created || o.FolioName != "Persa" || o.Number != 1 {
		t.Fatalf("created=%v nombre=%q número=%d: el pedido nace con el nombre amarrado y el folio del turno", res.Created, o.FolioName, o.Number)
	}
	if len(res.PrintLineIDs) != 2 {
		t.Fatalf("printLineIds = %v: la comanda del pedido nuevo lleva todo", res.PrintLineIDs)
	}
	var openedBy, discountBy, refBy int64
	var bizDate string
	var pending int
	if err := k.st.Pool.QueryRow(ctx, `
		select opened_by, discount_set_by, platform_ref_set_by, business_date::text,
		       (select count(*) from order_lines where order_id = o.id and enviado_a_cocina_at is null)
		  from orders o where id = $1`, o.ID).Scan(&openedBy, &discountBy, &refBy, &bizDate, &pending); err != nil {
		t.Fatal(err)
	}
	if openedBy != k.user {
		t.Errorf("opened_by = %d, quería %d (quien abrió la cuenta, no quien tocó Enviar)", openedBy, k.user)
	}
	if discountBy != jefa || refBy != jefa {
		t.Errorf("discount_set_by=%d platform_ref_set_by=%d, quería %d: quien los puso en la cuenta", discountBy, refBy, jefa)
	}
	if bizDate != domain.BusinessDate(fixedNow, domain.LoadBusinessLocation(domain.DefaultTimezone)).Format("2006-01-02") {
		t.Errorf("business_date = %s: la fecha la da el reloj al enviar (D-6)", bizDate)
	}
	if pending != 0 {
		t.Errorf("%d renglones sin marcar en cocina: el primer agregado reimprimiría el pedido", pending)
	}
	if n := countOrderMovements(t, k.st, o.ID); n != 2 {
		t.Errorf("movimientos de almacén = %d, quería 2 (uno por renglón)", n)
	}
	if o.PlatformOrderRef == nil || *o.PlatformOrderRef != "UB-77" || !o.Discount.Equal(pesos("10")) {
		t.Errorf("folio=%v descuento=%s", o.PlatformOrderRef, o.Discount)
	}
	d, err := k.drafts.Get(k.ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != domain.DraftSent || d.OrderID == nil || *d.OrderID != o.ID {
		t.Fatalf("la cuenta quedó %s → %v", d.Status, d.OrderID)
	}
}

// LA RED SE CAE AL CONFIRMAR: UN SOLO PEDIDO, UNA SOLA COMANDA (caso 15).
func TestSendIsIdempotent(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café idempotente al enviar", pesos("30"), false)
	v := k.newDraft(t, addOf(cafe, "1"))
	first, err := k.drafts.Send(k.ctx, v.ID, k.user)
	if err != nil {
		t.Fatal(err)
	}
	again, err := k.drafts.Send(k.ctx, v.ID, k.user)
	if err != nil {
		t.Fatalf("reintento: %v", err)
	}
	if again.Order.ID != first.Order.ID || len(again.PrintLineIDs) != 0 || again.PrintLineIDs == nil {
		t.Fatalf("reintento: pedido %d (antes %d), printLineIds %v: cocina recibiría la comanda dos veces",
			again.Order.ID, first.Order.ID, again.PrintLineIDs)
	}
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from orders`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("pedidos = %d (%v)", n, err)
	}
}

// Dos tabletas mandan la misma cuenta a la vez: un pedido.
func TestTwoTabletsSendTheSameAccount(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café de dos envíos", pesos("30"), false)
	ctxs := []context.Context{k.tenant(t, k.company), k.tenant(t, k.company)}
	for round := range 3 {
		v := k.newDraft(t, addOf(cafe, "1"))
		ids := make([]int64, 2)
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i := range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, err := k.drafts.Send(ctxs[i], v.ID, k.user)
				errs[i] = err
				if err == nil {
					ids[i] = r.Order.ID
				}
			}()
		}
		wg.Wait()
		if errs[0] != nil || errs[1] != nil || ids[0] != ids[1] {
			t.Fatalf("vuelta %d: errores %v, pedidos %v", round, errs, ids)
		}
	}
	var n int
	if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from orders`).Scan(&n); err != nil || n != 3 {
		t.Fatalf("pedidos = %d tras 3 cuentas mandadas desde dos tabletas", n)
	}
}

func TestSendWithoutOpenRegister(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café sin caja", pesos("30"), false)
	v := k.newDraft(t, addOf(cafe, "1"))
	if _, err := k.drafts.Send(k.ctx, v.ID, k.user); !errors.Is(err, domain.ErrNoOpenRegister) {
		t.Fatalf("= %v, quería NO_OPEN_REGISTER", err)
	}
	got, _ := k.drafts.Get(k.ctx, v.ID)
	if got.Status != domain.DraftCapturing || len(got.Lines) != 1 {
		t.Fatalf("la cuenta quedó %s con %d renglones: un envío rechazado no la toca", got.Status, len(got.Lines))
	}
}

func TestSendRejectsWhatNoLongerSells(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	ctx := context.Background()

	t.Run("producto desactivado: 422 con su nombre", func(t *testing.T) {
		taro := makeProduct(t, k.st, "Taro agotado", pesos("55"), false)
		v := k.newDraft(t, addOf(taro, "1"))
		if _, err := k.st.Pool.Exec(ctx, `update products set is_active = false where id = $1`, taro); err != nil {
			t.Fatal(err)
		}
		_, err := k.drafts.Send(k.ctx, v.ID, k.user)
		var pu domain.ProductUnavailable
		if !errors.As(err, &pu) || pu.Name != "Taro agotado" {
			t.Fatalf("= %v", err)
		}
	})

	t.Run("opción de modificador borrada: 422 que nombra el producto, nunca 500", func(t *testing.T) {
		crepa := makeProduct(t, k.st, "Crepa con extra", pesos("80"), false)
		opt := optionID(t, k.st, defaultCompanyID, crepa)
		v := k.newDraft(t, app.DraftLineCmd{OpID: uuid.New(), ProductID: crepa, Qty: pesos("1"),
			Modifiers: []domain.DraftModifier{{OptionID: opt, Qty: 1}}})
		if _, err := k.st.Pool.Exec(ctx, `delete from modifier_options where id = $1`, opt); err != nil {
			t.Fatal(err)
		}
		_, err := k.drafts.Send(k.ctx, v.ID, k.user)
		if !errors.Is(err, domain.ErrOptionNotFound) || !strings.Contains(err.Error(), "Crepa con extra") {
			t.Fatalf("= %v", err)
		}
	})

	t.Run("cuenta vacía", func(t *testing.T) {
		cafe := makeProduct(t, k.st, "Café quitado", pesos("30"), false)
		v := k.newDraft(t, addOf(cafe, "1"))
		if _, err := k.drafts.RemoveLine(k.ctx, v.ID, v.Lines[0].ID, v.Lines[0].Version); err != nil {
			t.Fatal(err)
		}
		if _, err := k.drafts.Send(k.ctx, v.ID, k.user); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("= %v", err)
		}
	})

	t.Run("descartada en otra tableta", func(t *testing.T) {
		cafe := makeProduct(t, k.st, "Café descartado al enviar", pesos("30"), false)
		v := k.newDraft(t, addOf(cafe, "1"))
		if _, err := k.st.Pool.Exec(ctx, `update order_drafts set status = 'descartada', discarded_at = now(),
			discard_reason = 'manual' where id = $1`, v.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := k.drafts.Send(k.ctx, v.ID, k.user); !errors.Is(err, domain.ErrDraftDiscarded) {
			t.Fatalf("= %v", err)
		}
	})
}

// Plataforma capturada a mano sin folio: se manda igual y queda pendiente de folio, como hoy. La
// pantalla pide el folio antes, pero «Mandar sin folio» es su salida explícita
// (FolioPlataformaSheet): rechazarlo aquí la dejaría sin salida con el repartidor enfrente.
func TestSendPlatformWithoutFolioStaysPending(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café de plataforma", pesos("30"), false)
	uber := platformID(t, k.st, defaultCompanyID, "Uber Eats")
	v := k.newDraft(t, addOf(cafe, "1"))
	if _, err := k.drafts.PatchHeader(k.ctx, v.ID, app.DraftHeaderPatch{ExpectedVersion: v.HeaderVersion,
		ServiceType: new("domicilio"), PlatformID: app.Field[int16]{Set: true, Value: &uber}}, k.user); err != nil {
		t.Fatal(err)
	}
	res, err := k.drafts.Send(k.ctx, v.ID, k.user)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if res.Order.PlatformOrderRef != nil || res.Order.DeliveryPlatformID == nil || *res.Order.DeliveryPlatformID != uber {
		t.Fatalf("pedido = %+v", res.Order)
	}
}

// LA CUENTA QUE CRUZA UN CIERRE DE TURNO CONSERVA SU ANIMAL (caso 23).
func TestDraftAcrossShiftsKeepsItsAnimal(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	ctx := context.Background()
	first := abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café de dos turnos", pesos("30"), false)
	v := k.newDraftNamed(t, "Persa", addOf(cafe, "1"))
	// Cierra el turno (la cuenta no lo bloquea) y abre otro.
	if _, err := k.st.Pool.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now(), closed_by = $2 where id = $1`,
		first, k.user); err != nil {
		t.Fatal(err)
	}
	second := abrirCajaPrincipal(t, k.st, k.user)
	// En el turno nuevo ya se cantó un «Persa» (por un camino que no pasa por la bolsa).
	if _, err := k.st.Pool.Exec(ctx, `
		insert into orders (client_uuid, business_date, daily_number, service_type, opened_by, subtotal, total, status,
		                    register_session_id, folio_name)
		values (gen_random_uuid(), $3, 999, 'mostrador', $2, 0, 0, 'entregada', $1, 'Persa')`, second, k.user, fixedNow); err != nil {
		t.Fatal(err)
	}
	res, err := k.drafts.Send(k.ctx, v.ID, k.user)
	if err != nil {
		t.Fatal(err)
	}
	if res.Order.FolioName != "Persa 2" {
		t.Fatalf("salió %q: quería «Persa 2» — nunca otro animal, el cliente ya oyó «Persa»", res.Order.FolioName)
	}
}

// El barrido y el envío a la vez: la cuenta queda enviada, nunca descartada con su pedido creado.
func TestSweepAndSendAtOnce(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café del barrido", pesos("30"), false)
	accounts := app.NewAccountsService(k.appSt, k.orders)
	ctxs := []context.Context{k.tenant(t, k.company), k.tenant(t, k.company)}
	for round := range 5 {
		v := k.newDraft(t, addOf(cafe, "1"))
		if _, err := k.st.Pool.Exec(context.Background(), `update order_drafts set updated_at = now() - interval '13 hours' where id = $1`, v.ID); err != nil {
			t.Fatal(err)
		}
		var sendErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, sendErr = k.drafts.Send(ctxs[0], v.ID, k.user) }()
		go func() { defer wg.Done(); _, _ = accounts.Live(ctxs[1], false) }()
		wg.Wait()
		status := draftStatus(t, k.st, v.ID)
		var orders int
		if err := k.st.Pool.QueryRow(context.Background(), `select count(*) from orders where client_uuid = $1`, v.ID).Scan(&orders); err != nil {
			t.Fatal(err)
		}
		switch {
		case sendErr == nil && (status != domain.DraftSent || orders != 1):
			t.Fatalf("vuelta %d: se envió pero la cuenta quedó %s con %d pedidos", round, status, orders)
		case sendErr != nil && (status != domain.DraftDiscarded || orders != 0):
			t.Fatalf("vuelta %d: el envío falló (%v) y la cuenta quedó %s con %d pedidos", round, sendErr, status, orders)
		}
	}
}

// UNA CUENTA DESCARTADA NO GASTA FOLIO: el consecutivo no tiene huecos ni repetidos (SC-006).
func TestDiscardedDraftsSpendNoFolio(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	session := abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café del folio", pesos("30"), false)
	for i := range 6 {
		v := k.newDraft(t, addOf(cafe, "1"))
		if i%2 == 1 {
			if _, err := k.st.Pool.Exec(context.Background(), `update order_drafts set status = 'descartada',
				discarded_at = now(), discard_reason = 'manual' where id = $1`, v.ID); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if _, err := k.drafts.Send(k.ctx, v.ID, k.user); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := k.st.Pool.Query(context.Background(), `select daily_number from orders where register_session_id = $1 order by daily_number`, session)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := 1
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Fatalf("folio %d donde tocaba %d: hay un hueco o un repetido", n, want)
		}
		want++
	}
	if want != 4 {
		t.Fatalf("%d pedidos, quería 3", want-1)
	}
}

func TestSendInTheThreeCases(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	other := makeCompany(t, k.st, "otra-envio")
	otherUser := makeUserIn(t, k.st, other, "cajero_otra_envio", "cajero")
	cafe := makeProduct(t, k.st, "Café ajeno al envío", pesos("30"), false)
	v := k.newDraft(t, addOf(cafe, "1"))
	inTheThreeCases(t, k.company, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewDraftsService(st, app.NewOrdersService(st, clock))
		if _, err := svc.Send(ctx, v.ID, otherUser); err == nil {
			t.Fatal("otra empresa mandó a cocina la cuenta de la dueña")
		}
	})
	if got := draftStatus(t, k.st, v.ID); got != domain.DraftCapturing {
		t.Fatalf("la cuenta de la dueña quedó %s", got)
	}
}
