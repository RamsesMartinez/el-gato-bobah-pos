//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// extraOf siembra una opción con su tope por renglón, en un grupo propio ligado al producto: el
// extra que el producto SÍ admite.
func extraOf(t *testing.T, st *store.Store, product int64, name string, delta decimal.Decimal, maxPerLine int16) int64 {
	t.Helper()
	ctx := context.Background()
	var group, id int64
	if err := st.Pool.QueryRow(ctx, `insert into modifier_groups (company_id, name) values ($1, $2) returning id`,
		defaultCompanyID, "Grupo de "+name).Scan(&group); err != nil {
		t.Fatalf("grupo de %s: %v", name, err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into product_modifier_groups (product_id, group_id, min_select, max_select)
		values ($1, $2, 0, 5)`, product, group); err != nil {
		t.Fatalf("ligar el grupo de %s: %v", name, err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into modifier_options (company_id, group_id, name, price_delta, max_per_line)
		values ($1, $2, $3, $4, $5) returning id`, defaultCompanyID, group, name, delta, maxPerLine).Scan(&id); err != nil {
		t.Fatalf("opción %s: %v", name, err)
	}
	return id
}

func withExtra(product, option int64, qty int) app.DraftLineCmd {
	return app.DraftLineCmd{OpID: uuid.New(), ProductID: product, Qty: pesos("1"),
		Modifiers: []domain.DraftModifier{{OptionID: option, Qty: qty}}}
}

// Una salsa ×5 con tope de 2 se aceptaba al agregar: la cuenta mostraba el producto como «ya no se
// vende» (available=false, $0) y el rechazo llegaba hasta enviar. El tope es del producto y se
// conoce desde el primer toque: se rechaza ahí, con el mismo 422 que daría enviar.
func TestOptionOverItsMaxIsRejectedWhenAdded(t *testing.T) {
	k := newDraftsKit(t)
	taro := makeProduct(t, k.st, "Taro con salsa", pesos("50"), false)
	salsa := extraOf(t, k.st, taro, "Salsa con tope", pesos("5"), 2)

	t.Run("al crear la cuenta", func(t *testing.T) {
		_, _, err := k.drafts.Create(k.ctx, app.CreateDraftCmd{ID: uuid.New(), Lines: []app.DraftLineCmd{withExtra(taro, salsa, 5)}, Actor: k.user})
		if !errors.Is(err, domain.ErrOptionOverMax) {
			t.Fatalf("salsa ×5 con tope 2 al crear = %v, quería OPTION_OVER_MAX", err)
		}
	})
	t.Run("al agregar", func(t *testing.T) {
		v := k.newDraft(t, withExtra(taro, salsa, 2))
		_, err := k.drafts.AddLine(k.ctx, v.ID, withExtra(taro, salsa, 5))
		if !errors.Is(err, domain.ErrOptionOverMax) {
			t.Fatalf("salsa ×5 con tope 2 al agregar = %v, quería OPTION_OVER_MAX", err)
		}
	})
	t.Run("al cambiar los modificadores", func(t *testing.T) {
		v := k.newDraft(t, withExtra(taro, salsa, 1))
		l := v.Lines[0]
		mods := []domain.DraftModifier{{OptionID: salsa, Qty: 5}}
		_, err := k.drafts.ChangeLine(k.ctx, app.ChangeDraftLineCmd{DraftID: v.ID, LineID: l.ID, ExpectedVersion: l.Version, Modifiers: &mods})
		if !errors.Is(err, domain.ErrOptionOverMax) {
			t.Fatalf("salsa ×5 con tope 2 al cambiar = %v, quería OPTION_OVER_MAX", err)
		}
	})
	t.Run("al subir pestañas: se rechaza esa pestaña, no la subida", func(t *testing.T) {
		good := app.ImportAccount{ID: uuid.New(), Lines: []app.DraftLineCmd{withExtra(taro, salsa, 2)}}
		bad := app.ImportAccount{ID: uuid.New(), Lines: []app.DraftLineCmd{withExtra(taro, salsa, 5)}}
		res, err := k.drafts.Import(k.ctx, []app.ImportAccount{good, bad}, k.user)
		if err != nil {
			t.Fatal(err)
		}
		if res[0].Outcome != "created" || res[1].Outcome != "rejected" {
			t.Fatalf("resultados %s/%s, quería created/rejected", res[0].Outcome, res[1].Outcome)
		}
	})
}
