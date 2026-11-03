//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Cada camino de la merma deja su evento en la bitácora, en la misma transacción: uno que se
// revierte no deja evento, y uno que se confirma no puede quedarse sin él (spec 039, FR-021).
func TestEveryWasteActionLeavesItsAuditEvent(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cajero := makeUser(t, st, "cajero_bitacora_merma", "cajero")
	gerente := makeUser(t, st, "gerente_bitacora_merma", "gerente")
	svc, ctx := asWaste(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	ing := pieceIngredient(t, st, "Insumo de la bitácora de merma")
	adjust(t, st, "ingrediente", ing, "50")
	if _, err := st.Pool.Exec(ctx, `update ingredients set current_cost = 40 where id = $1`, ing); err != nil {
		t.Fatal(err)
	}
	count := func(action string) int {
		t.Helper()
		var n int
		if err := st.Pool.QueryRow(context.Background(), `select count(*) from audit_events where action = $1`, action).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	tope := "100"
	setWasteThreshold(t, ctx, svc, gerente, &tope)
	if count("settings.waste_threshold") != 1 {
		t.Fatal("cambiar el tope no dejó evento")
	}
	applied, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("1"), Actor: cajero})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("3"), Actor: cajero})
	if err != nil {
		t.Fatal(err)
	}
	rejected, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("4"), Actor: cajero})
	if err != nil {
		t.Fatal(err)
	}
	if count("waste.recorded") != 3 {
		t.Fatalf("waste.recorded = %d; quiere 3", count("waste.recorded"))
	}
	if err := svc.ApproveWaste(ctx, pending.ID, gerente); err != nil {
		t.Fatal(err)
	}
	if err := svc.RejectWaste(ctx, rejected.ID, gerente); err != nil {
		t.Fatal(err)
	}
	if err := svc.UndoWaste(ctx, applied.ID, cajero, false); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"waste.approved", "waste.rejected", "waste.undone"} {
		if count(a) != 1 {
			t.Errorf("%s = %d; quiere 1", a, count(a))
		}
	}
	// Una aprobación que falla (ya resuelta) no deja evento: va en la misma transacción.
	if err := svc.ApproveWaste(ctx, pending.ID, gerente); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("= %v", err)
	}
	if count("waste.approved") != 1 {
		t.Fatal("una aprobación rechazada dejó evento")
	}
}
