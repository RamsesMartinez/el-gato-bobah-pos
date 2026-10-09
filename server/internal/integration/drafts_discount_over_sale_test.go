//go:build integration

package integration

import (
	"errors"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Cuenta de $80 con $60 de descuento; se quita un renglón de $45 y la venta queda en $35. Antes la
// cuenta se mostraba con el descuento recortado (descuento 35, total 0: «gratis») y al mandarla el
// envío respondía 422 «máximo 35.00»: la pantalla y la cocina decían cosas distintas, y lo que la
// pantalla decía era regalar la cuenta. Ahora el cambio que deja el descuento por encima de la venta
// se rechaza, con el máximo, y la cuenta queda como estaba.
func TestLineChangeCannotLeaveTheDiscountOverTheSale(t *testing.T) {
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	taro := makeProduct(t, k.st, "Taro del descuento", pesos("45"), false)
	cafe := makeProduct(t, k.st, "Café del descuento", pesos("35"), false)
	setup := func(t *testing.T) *app.DraftView {
		t.Helper()
		v := k.newDraft(t, addOf(taro, "1"), addOf(cafe, "1"))
		res, err := k.drafts.PatchHeader(k.ctx, v.ID, app.DraftHeaderPatch{ExpectedVersion: v.HeaderVersion,
			Discount: app.Field[app.DraftDiscount]{Set: true, Value: &app.DraftDiscount{Amount: new(pesos("60"))}}}, k.user)
		if err != nil {
			t.Fatal(err)
		}
		if !res.View.Total.Equal(pesos("20")) {
			t.Fatalf("total = %s, quería 20", res.View.Total)
		}
		return res.View
	}
	lineOf := func(v *app.DraftView, product int64) app.DraftLineView {
		for _, l := range v.Lines {
			if l.ProductID == product {
				return l
			}
		}
		t.Fatalf("no está el producto %d", product)
		return app.DraftLineView{}
	}
	sameAsBefore := func(t *testing.T, id uuid.UUID) {
		t.Helper()
		got, err := k.drafts.Get(k.ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Subtotal.Equal(pesos("80")) || !got.DiscountTotal.Equal(pesos("60")) || !got.Total.Equal(pesos("20")) {
			t.Fatalf("subtotal=%s descuento=%s total=%s: el cambio rechazado tocó la cuenta", got.Subtotal, got.DiscountTotal, got.Total)
		}
	}

	t.Run("quitar el renglón", func(t *testing.T) {
		v := setup(t)
		l := lineOf(v, taro)
		_, err := k.drafts.RemoveLine(k.ctx, v.ID, l.ID, l.Version)
		if !errors.Is(err, domain.ErrDescuentoMayorQueLaVenta) {
			t.Fatalf("quitar dejando el descuento ($60) sobre la venta ($35) = %v, quería DISCOUNT_OVER_SUBTOTAL", err)
		}
		sameAsBefore(t, v.ID)
	})

	t.Run("bajar la cantidad", func(t *testing.T) {
		v := setup(t)
		if _, err := k.drafts.AddLine(k.ctx, v.ID, app.DraftLineCmd{OpID: uuid.New(), IntoLineID: new(lineOf(v, cafe).ID), Qty: pesos("1")}); err != nil {
			t.Fatal(err)
		}
		v, _ = k.drafts.Get(k.ctx, v.ID) // 45 + 70 = 115
		l := lineOf(v, taro)
		if _, err := k.drafts.RemoveLine(k.ctx, v.ID, l.ID, l.Version); err != nil {
			t.Fatalf("quitar dejando la venta en $70 con $60 de descuento sí vale: %v", err)
		}
		v, _ = k.drafts.Get(k.ctx, v.ID)
		c := lineOf(v, cafe)
		_, err := k.drafts.ChangeLine(k.ctx, app.ChangeDraftLineCmd{DraftID: v.ID, LineID: c.ID, ExpectedVersion: c.Version, Qty: new(pesos("1"))})
		if !errors.Is(err, domain.ErrDescuentoMayorQueLaVenta) {
			t.Fatalf("bajar la cantidad dejando el descuento sobre la venta = %v, quería DISCOUNT_OVER_SUBTOTAL", err)
		}
	})

	t.Run("lo que la cuenta muestra es lo que se envía", func(t *testing.T) {
		v := setup(t)
		got, err := k.drafts.Send(k.ctx, v.ID, k.user)
		if err != nil {
			t.Fatalf("la cuenta mostraba total %s y no se pudo enviar: %v", v.Total, err)
		}
		if !got.Order.Total.Equal(v.Total) {
			t.Fatalf("la cuenta mostraba %s y el pedido nació con %s", v.Total, got.Order.Total)
		}
	})
}
