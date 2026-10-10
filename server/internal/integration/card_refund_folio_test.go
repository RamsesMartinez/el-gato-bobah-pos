//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// UNA DEVOLUCIÓN CON TARJETA NO TERMINA SIN EL FOLIO QUE IMPRIME LA TERMINAL (punto 10; EB-34,
// EB-36). Se guarda con quién lo capturó, y la pantalla sabe en qué terminal hacerla. La de
// efectivo no lo pide.
func TestCardRefundNeedsTerminalFolio(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_folio", "admin")
	abrirCajaPrincipal(t, st, cajero)
	ord := pedidoCobradoParcial(t, ctx, st, orders, "folio", "200", "200", cajero, paymentMethodID(t, st, "Tarjeta débito"), false)

	info, err := orders.RefundInfo(ctx, ord)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.CardTerminals) != 1 || info.CardTerminals[0] != "Terminal" {
		t.Fatalf("la pantalla no sabe en qué terminal devolver: %+v", info.CardTerminals)
	}
	for _, folio := range []string{"", "   "} {
		err := orders.Devolver(ctx, app.DevolucionCmd{OrderID: ord, Monto: dec("50"), Motivo: "prueba", ActorID: cajero, CardFolio: folio})
		if !errors.Is(err, domain.ErrRefundFolioRequired) {
			t.Fatalf("folio %q: err = %v", folio, err)
		}
	}
	if err := orders.Devolver(ctx, app.DevolucionCmd{OrderID: ord, Monto: dec("50"), Motivo: "prueba", ActorID: cajero, CardFolio: " 00123 "}); err != nil {
		t.Fatal(err)
	}
	var folio string
	var quien int64
	if err := st.Pool.QueryRow(ctx, `select card_refund_folio, card_refund_captured_by from order_refunds where order_id = $1`, ord).Scan(&folio, &quien); err != nil {
		t.Fatal(err)
	}
	if folio != "00123" || quien != cajero {
		t.Fatalf("folio %q por %d; quería 00123 por %d", folio, quien, cajero)
	}

	efectivo := pedidoCobradoParcial(t, ctx, st, orders, "folio efectivo", "100", "100", cajero, paymentMethodID(t, st, "Efectivo"), false)
	if err := orders.Devolver(ctx, app.DevolucionCmd{OrderID: efectivo, Monto: dec("20"), Motivo: "prueba", ActorID: cajero}); err != nil {
		t.Fatalf("en efectivo no se pide folio: %v", err)
	}
}
