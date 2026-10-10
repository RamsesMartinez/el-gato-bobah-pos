//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// cerrarDejandoPropinas cierra el turno abierto de la caja principal dejando la propina en caja.
func cerrarDejandoPropinas(t *testing.T, ctx context.Context, st *store.Store, reg, cajero int64) {
	t.Helper()
	entregarPendientes(t, st)
	back := app.NewBackofficeService(st, clock)
	v, err := back.CurrentByRegister(ctx, reg)
	if err != nil {
		t.Fatal(err)
	}
	var efectivo = dec("0")
	for _, m := range v.Totals {
		if m.Kind == "efectivo" && m.Expected != nil {
			efectivo = *m.Expected
		}
	}
	if _, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &efectivo, Motivo: "prueba", Propinas: "quedan_en_caja"}); err != nil {
		t.Fatalf("cerrar: %v", err)
	}
}

// LO HEREDADO CONSERVA SU MEDIO: una propina de tarjeta que se quedó en caja y se entrega en el
// turno siguiente sigue siendo «propina de tarjeta pagada en efectivo» (punto 2). Heredarla como
// un solo número la convertía en efectivo y el corte la escondía.
func TestInheritedCardTipStaysCardTipNextShift(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	tips := app.NewTipsService(st, clock)
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_her_tarjeta", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	reg := principalRegister(t, st)
	cobrarConPropina(t, ctx, st, orders, "her tarjeta", "100", "40", cajero, paymentMethodID(t, st, "Tarjeta débito"))
	cerrarDejandoPropinas(t, ctx, st, reg, cajero)

	abrirCajaPrincipal(t, st, cajero)
	pend, err := tips.Pending(ctx, reg)
	if err != nil {
		t.Fatal(err)
	}
	if !pend.Total.Equal(dec("40")) {
		t.Fatalf("heredado = %s, quería 40", pend.Total)
	}
	if _, err := tips.Payout(ctx, reg, app.TipPayoutCmd{Mode: "parejo", ActorID: cajero,
		Recipients: []app.TipRecipient{{UserID: cajero}}}); err != nil {
		t.Fatal(err)
	}
	v, err := back.CurrentByRegister(ctx, reg)
	if err != nil {
		t.Fatal(err)
	}
	if !v.CardTipsPaidInCash.Equal(dec("40")) {
		t.Fatalf("propina de tarjeta pagada en efectivo = %s en el turno siguiente; quería 40", v.CardTipsPaidInCash)
	}
}

// UNA PROPINA DEVUELTA DESPUÉS DEL CIERRE DEJA DE HEREDARSE. Sin esto el turno siguiente ofrecía
// entregar dinero que ya se le regresó al cliente.
func TestTipRefundedAfterCloseLowersInherited(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	tips := app.NewTipsService(st, clock)
	cajero := makeUser(t, st, "cajero_her_dev", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	reg := principalRegister(t, st)
	cobrarConPropina(t, ctx, st, orders, "her dev a", "100", "30", cajero, paymentMethodID(t, st, "Efectivo"))
	cobrarConPropina(t, ctx, st, orders, "her dev b", "100", "25", cajero, paymentMethodID(t, st, "Efectivo"))
	cerrarDejandoPropinas(t, ctx, st, reg, cajero)

	abrirCajaPrincipal(t, st, cajero)
	// Se devuelve la propina del primer pedido, ya con el turno nuevo abierto.
	if _, err := st.Pool.Exec(ctx, `insert into order_refunds (order_id, payment_method_id, amount, tip_amount, reason, refunded_by, created_at)
		select op.order_id, op.payment_method_id, 0, 30, 'prueba', $1, now() + interval '1 minute'
		  from order_payments op where op.tip_amount = 30`, cajero); err != nil {
		t.Fatal(err)
	}
	pend, err := tips.Pending(ctx, reg)
	if err != nil {
		t.Fatal(err)
	}
	if !pend.Total.Equal(dec("25")) {
		t.Fatalf("heredado = %s tras devolver 30 de 55; quería 25", pend.Total)
	}
}
