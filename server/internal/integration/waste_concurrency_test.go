//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Registrar no espera a una venta del mismo insumo ni la hace esperar: con la existencia tomada por
// una venta en curso, responde «vuelve a intentar» (409) de inmediato, como las compras (038).
func TestRecordingWasteDoesNotWaitForASale(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cajero := makeUser(t, st, "cajero_merma_venta", "cajero")
	svc, ctx := asWaste(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	ing := pieceIngredient(t, st, "Insumo de la venta en curso")
	adjust(t, st, "ingrediente", ing, "10")

	sale, err := st.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sale.Rollback(context.Background()) }()
	if _, err := sale.Exec(context.Background(), `select 1 from stock_levels where ingredient_id = $1 for update`, ing); err != nil {
		t.Fatal(err)
	}
	quick, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err = svc.RecordWaste(quick, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("1"), Actor: cajero})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("= %v; quiere 409 sin esperar a la venta", err)
	}
}

// Con dos sucursales y sin selector, la merma no adivina en cuál pasó.
func TestWasteDoesNotGuessTheBranch(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cajero := makeUser(t, st, "cajero_dos_sucursales", "cajero")
	ing := pieceIngredient(t, st, "Insumo de dos sucursales")
	adjust(t, st, "ingrediente", ing, "10")
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	addBranch(t, st, defaultCompanyID, "SEGUNDA")
	svc, ctx := asWaste(t, defaultCompanyID)
	_, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("1"), Actor: cajero})
	if !errors.Is(err, domain.ErrBranchAmbiguous) {
		t.Fatalf("= %v; quiere BRANCH_AMBIGUOUS, no la matriz", err)
	}
}
