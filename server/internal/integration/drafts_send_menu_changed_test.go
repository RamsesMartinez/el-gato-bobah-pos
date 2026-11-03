//go:build integration

package integration

import (
	"errors"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// El 2026-10-10 se cambió el grupo de perlas de Frappé Gatuno con la cuenta ya capturada: «Mandar a
// cocina» y «Enviar y cobrar» rechazaban la cuenta entera con un «No se pudo» que no decía qué
// renglón volver a elegir (spec 034, US3). El rechazo de enviar tiene que nombrar el RENGLÓN de la
// cuenta, para que la tableta lo señale sin perder lo demás.
func TestSendRejectedByAStaleMenuNamesTheDraftLine(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	taro := makeProduct(t, k.st, "Taro de la cuenta vieja", pesos("50"), false)
	frappe := makeProduct(t, k.st, "Frappé Gatuno de prueba", pesos("70"), false)
	perlasViejas := extraOf(t, k.st, frappe, "Perlas viejas", pesos("10"), 1)

	expectLine := func(t *testing.T, id uuid.UUID, wantLine uuid.UUID, wantProduct int64, sentinel error) {
		t.Helper()
		_, err := k.drafts.Send(k.ctx, id, k.user)
		if !errors.Is(err, sentinel) {
			t.Fatalf("Send = %v, quería %v (el 422 MENU_CHANGED)", err, sentinel)
		}
		var lr domain.LineRejected
		if !errors.As(err, &lr) {
			t.Fatalf("el rechazo de enviar no dice qué renglón (%T): la tableta no puede señalarlo", err)
		}
		if lr.LineID != wantLine.String() || lr.ProductID != wantProduct {
			t.Fatalf("renglón %q producto %d, quería %s producto %d", lr.LineID, lr.ProductID, wantLine, wantProduct)
		}
	}

	t.Run("el grupo de extras se cambió por otro", func(t *testing.T) {
		v := k.newDraft(t, addOf(taro, "1"), withExtra(frappe, perlasViejas, 1))
		if _, err := k.st.Pool.Exec(k.ctx, `delete from product_modifier_groups where product_id = $1`, frappe); err != nil {
			t.Fatal(err)
		}
		extraOf(t, k.st, frappe, "Perlas nuevas", pesos("10"), 1)
		expectLine(t, v.ID, lineOf(t, v, frappe), frappe, domain.ErrOptionNotFound)
	})

	t.Run("el producto dejó de venderse", func(t *testing.T) {
		v := k.newDraft(t, addOf(taro, "1"))
		if _, err := k.st.Pool.Exec(k.ctx, `update products set is_active = false where id = $1`, taro); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = k.st.Pool.Exec(k.ctx, `update products set is_active = true where id = $1`, taro) })
		expectLine(t, v.ID, lineOf(t, v, taro), taro, domain.ErrProductNotSell)
	})
}

// Visto en la revisión al retomar el 034: cambiar los extras de un renglón con el menú ya cambiado
// respondía MENU_CHANGED sin decir qué renglón, aunque el servicio sabe cuál se está editando.
func TestChangeLineRejectedByAStaleMenuNamesTheLine(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	frappe := makeProduct(t, k.st, "Frappé de extras viejos", pesos("70"), false)
	perlasViejas := extraOf(t, k.st, frappe, "Perlas de antes", pesos("10"), 1)
	v := k.newDraft(t, withExtra(frappe, perlasViejas, 1))
	l := v.Lines[0]
	if _, err := k.st.Pool.Exec(k.ctx, `delete from product_modifier_groups where product_id = $1`, frappe); err != nil {
		t.Fatal(err)
	}

	mods := []domain.DraftModifier{{OptionID: perlasViejas, Qty: 1}}
	_, err := k.drafts.ChangeLine(k.ctx, app.ChangeDraftLineCmd{DraftID: v.ID, LineID: l.ID, ExpectedVersion: l.Version, Modifiers: &mods})
	if !errors.Is(err, domain.ErrOptionNotFound) {
		t.Fatalf("ChangeLine = %v, quería el 422 MENU_CHANGED (opción que ya no es del producto)", err)
	}
	var lr domain.LineRejected
	if !errors.As(err, &lr) || lr.LineID != l.ID.String() {
		t.Fatalf("el rechazo de cambiar los extras nombra el renglón %q, quería %s: la tableta no sabe cuál volver a elegir", lr.LineID, l.ID)
	}
}

func lineOf(t *testing.T, v *app.DraftView, productID int64) uuid.UUID {
	t.Helper()
	for _, l := range v.Lines {
		if l.ProductID == productID {
			return l.ID
		}
	}
	t.Fatalf("la cuenta no tiene el producto %d", productID)
	return uuid.UUID{}
}
