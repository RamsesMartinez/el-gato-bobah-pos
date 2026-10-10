//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// «CANCELAR LO QUE FALTA» (dueño, 2026-10-09, opción A; migración 0085).
//
// Un entregado de $100 pagado con $40 cuyo cliente se fue. Lo pagado se queda como venta y en el
// cajón; los $60 se dan por perdidos con su motivo. Lo perdido se clasifica UNA vez: no es cobro (ni
// Total de Ventas ni esperado del corte), no es devolución, deja de ser pendiente y deja de bloquear
// el cierre. Corre como gatobobah_app: toca columnas nuevas de orders.
func TestWriteOffIsCountedOnce(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	ctx := k.ctx
	back := app.NewBackofficeService(k.appSt, clock)
	principal := registerID(t, k.st, "Caja principal")
	abrirCajaPrincipal(t, k.st, k.user)

	half := k.order(t, "1", "40", true)
	cmd := app.WriteOffCmd{OrderID: half.ID, Motivo: "se fue sin pagar", ActorID: k.user}
	if err := k.orders.WriteOff(ctx, cmd); err != nil {
		t.Fatalf("WriteOff: %v", err)
	}
	if err := k.orders.WriteOff(ctx, cmd); !errors.Is(err, domain.ErrCancelDeliveredPaid) {
		t.Fatalf("dar por perdido dos veces = %v, quiere ErrCancelDeliveredPaid: ya no debe nada", err)
	}

	sum, err := app.NewSalesService(k.appSt, clock).Summary(ctx, filtroDePrueba())
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Total.Equal(pesos("40")) {
		t.Fatalf("Total de Ventas = %s, quiere 40: lo perdido se contó como cobrado", sum.Total)
	}
	if sum.WrittenOff.Count != 1 || !sum.WrittenOff.Amount.Equal(pesos("60")) {
		t.Fatalf("perdido = %+v, quiere 1 por 60", sum.WrittenOff)
	}
	if !sum.Pending.Amount.IsZero() {
		t.Fatalf("pendiente = %s: lo perdido se contó también como por cobrar", sum.Pending.Amount)
	}
	if !sum.Refunded.Amount.IsZero() {
		t.Fatalf("devoluciones = %s: lo perdido se contó también como devolución", sum.Refunded.Amount)
	}

	view, err := back.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Uncollected.IsZero() {
		t.Fatalf("sin cobrar en el corte = %s: lo perdido se contó también como sin cobrar", view.Uncollected)
	}
	if !view.WrittenOff.Equal(pesos("60")) {
		t.Fatalf("perdido en el corte = %s, quiere 60", view.WrittenOff)
	}
	if got := totalOf(t, view.Totals, "Efectivo"); !got.Equal(pesos("40")) {
		t.Fatalf("el corte espera %s en efectivo, quiere 40: lo pagado se queda en el cajón", got)
	}
	if len(view.Owing) != 0 {
		t.Fatalf("owing = %+v: un pedido con su resto perdido ya no debe", view.Owing)
	}
	if _, err := back.CloseSession(ctx, principal, k.user, cierreDelCajonAMano(t, k.st,
		map[int]decimal.Decimal{int(k.efectivo): pesos("40")})); err != nil {
		t.Fatalf("el cierre se negó con el resto ya perdido: %v", err)
	}
}

// Sin pagos se cancela entero; con lo pagado no debe nada. Y desde otra empresa, una conexión
// reciclada o sin empresa el pedido no existe.
func TestWriteOffGuards(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	none := k.order(t, "1", "0", true)
	if err := k.orders.WriteOff(k.ctx, app.WriteOffCmd{OrderID: none.ID, Motivo: "se fue", ActorID: k.user}); !errors.Is(err, domain.ErrWriteOffWithoutPayments) {
		t.Fatalf("sin pagos = %v, quiere ErrWriteOffWithoutPayments", err)
	}
	if err := k.orders.WriteOff(k.ctx, app.WriteOffCmd{OrderID: none.ID, Motivo: "  ", ActorID: k.user}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("sin motivo = %v, quiere ErrValidation", err)
	}
	half := k.order(t, "1", "40", true)
	other := makeCompany(t, k.st, "empresa-otra-perdido")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		err := app.NewOrdersService(st, clock).WriteOff(ctx, app.WriteOffCmd{OrderID: half.ID, Motivo: "se fue", ActorID: k.user})
		if err == nil {
			t.Fatal("se dio por perdido el pedido de otra empresa")
		}
	})
}
