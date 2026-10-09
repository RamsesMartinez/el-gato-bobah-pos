//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Un extra que no pertenece a ningún grupo del producto se aceptaba y se COBRABA, por los dos
// caminos: el pedido directo (`POST /orders`) y la cuenta en captura. Bastaba el id de una opción
// de otro producto, o mandarla a un producto sin grupos.
func TestAnExtraOfAnotherProductIsNotSold(t *testing.T) {
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	cafe := makeProduct(t, k.st, "Café sin extras", pesos("30"), false)
	crepa := makeProduct(t, k.st, "Crepa con extras", pesos("80"), false)
	ajena := optionID(t, k.st, defaultCompanyID) // de un grupo que ningún producto tiene
	propia := extraOf(t, k.st, crepa, "Nutella", pesos("15"), 1)
	mods := func(opt int64) []domain.DraftModifier { return []domain.DraftModifier{{OptionID: opt, Qty: 1}} }

	t.Run("pedido directo", func(t *testing.T) {
		for name, line := range map[string]domain.OrderLineInput{
			"producto sin grupos":          {ProductID: cafe, Qty: pesos("1"), Modifiers: []domain.OrderModInput{{OptionID: propia, Qty: 1}}},
			"opción de un grupo que no es": {ProductID: crepa, Qty: pesos("1"), Modifiers: []domain.OrderModInput{{OptionID: ajena, Qty: 1}}},
		} {
			_, err := k.orders.Create(k.ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: k.user,
				Lines: []domain.OrderLineInput{line}})
			if !errors.Is(err, domain.ErrOptionNotFound) {
				t.Fatalf("%s: = %v, quería 422: ese extra no es de ese producto", name, err)
			}
		}
		if _, err := k.orders.Create(k.ctx, app.CreateOrderCmd{ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: k.user,
			Lines: []domain.OrderLineInput{{ProductID: crepa, Qty: pesos("1"), Modifiers: []domain.OrderModInput{{OptionID: propia, Qty: 1}}}}}); err != nil {
			t.Fatalf("el extra propio del producto tiene que venderse: %v", err)
		}
	})

	t.Run("cuenta en captura", func(t *testing.T) {
		_, _, err := k.drafts.Create(k.ctx, app.CreateDraftCmd{ID: uuid.New(), Actor: k.user,
			Lines: []app.DraftLineCmd{{OpID: uuid.New(), ProductID: cafe, Qty: pesos("1"), Modifiers: mods(propia)}}})
		if !errors.Is(err, domain.ErrOptionNotFound) {
			t.Fatalf("crear la cuenta con un extra ajeno = %v, quería 422", err)
		}
		v := k.newDraft(t, app.DraftLineCmd{OpID: uuid.New(), ProductID: crepa, Qty: pesos("1"), Modifiers: mods(propia)})
		if _, err := k.drafts.AddLine(k.ctx, v.ID, app.DraftLineCmd{OpID: uuid.New(), ProductID: crepa, Qty: pesos("1"), Modifiers: mods(ajena)}); !errors.Is(err, domain.ErrOptionNotFound) {
			t.Fatalf("agregar un extra ajeno = %v, quería 422", err)
		}
		// Ligado cuando se capturó y desligado después: la cuenta no lo cobra y enviar lo rechaza.
		if _, err := k.st.Pool.Exec(context.Background(), `delete from product_modifier_groups where product_id = $1`, crepa); err != nil {
			t.Fatal(err)
		}
		got, err := k.drafts.Get(k.ctx, v.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Lines[0].Available || !got.Total.IsZero() {
			t.Fatalf("available=%v total=%s: la cuenta sigue cobrando un extra que el producto ya no admite", got.Lines[0].Available, got.Total)
		}
		if _, err := k.drafts.Send(k.ctx, v.ID, k.user); !errors.Is(err, domain.ErrOptionNotFound) {
			t.Fatalf("enviar con un extra que el producto ya no admite = %v, quería 422", err)
		}
	})
}
