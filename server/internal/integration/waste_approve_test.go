//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

func setWasteThreshold(t *testing.T, ctx context.Context, svc *app.BackofficeService, actor int64, threshold *string) {
	t.Helper()
	if err := svc.SetWasteThreshold(ctx, threshold, actor); err != nil {
		t.Fatalf("tope: %v", err)
	}
}

// Arriba del tope una merma espera aprobación y no resta; aprobar resta, rechazar no, y una resuelta
// no se vuelve a resolver (spec 039, FR-010 a FR-012).
func TestWasteOverTheThresholdWaitsForApproval(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cajero := makeUser(t, st, "cajero_tope", "cajero")
	gerente := makeUser(t, st, "gerente_tope", "gerente")
	svc, ctx := asWaste(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	tope := "100"
	setWasteThreshold(t, ctx, svc, gerente, &tope)

	ing := pieceIngredient(t, st, "Perlas del tope")
	adjust(t, st, "ingrediente", ing, "10")
	if _, err := st.Pool.Exec(ctx, `update ingredients set current_cost = 95 where id = $1`, ing); err != nil {
		t.Fatal(err)
	}

	t.Run("bajo el tope resta de una vez", func(t *testing.T) {
		res, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("1"), Actor: cajero})
		if err != nil || res.Status != string(domain.WasteApplied) {
			t.Fatalf("= %+v, %v; $95 no pasa el tope de $100", res, err)
		}
	})

	var cara int64
	t.Run("sobre el tope queda por aprobar y no resta", func(t *testing.T) {
		res, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("3"), Actor: cajero})
		if err != nil || res.Status != string(domain.WastePending) {
			t.Fatalf("= %+v, %v; $285 pasa el tope", res, err)
		}
		cara = res.ID
		if got := onHandIngredient(t, st, ing); !got.Equal(qty("9")) {
			t.Fatalf("quedaron %s; la por aprobar no resta (quiere 9)", got)
		}
		pend, err := svc.PendingWaste(ctx)
		if err != nil || len(pend) != 1 || pend[0].ID != cara {
			t.Fatalf("por aprobar = %+v, %v", pend, err)
		}
	})

	t.Run("aprobar resta; aprobar otra vez es conflicto", func(t *testing.T) {
		if err := svc.ApproveWaste(ctx, cara, gerente); err != nil {
			t.Fatal(err)
		}
		if got := onHandIngredient(t, st, ing); !got.Equal(qty("6")) {
			t.Fatalf("quedaron %s; aprobar resta las 3 (quiere 6)", got)
		}
		if err := svc.ApproveWaste(ctx, cara, gerente); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("aprobar dos veces = %v; restaría dos veces", err)
		}
	})

	t.Run("rechazar no resta", func(t *testing.T) {
		res, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("2"), Actor: cajero})
		if err != nil || res.Status != string(domain.WastePending) {
			t.Fatalf("= %+v, %v", res, err)
		}
		if err := svc.RejectWaste(ctx, res.ID, gerente); err != nil {
			t.Fatal(err)
		}
		if got := onHandIngredient(t, st, ing); !got.Equal(qty("6")) {
			t.Fatalf("quedaron %s; rechazada no resta (quiere 6)", got)
		}
	})

	t.Run("un insumo sin costo con tope espera aprobación y al aprobarse toma el costo de ese día", func(t *testing.T) {
		sin := pieceIngredient(t, st, "Insumo sin costo del tope")
		adjust(t, st, "ingrediente", sin, "5")
		res, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &sin, Qty: qty("1"), Actor: cajero})
		if err != nil || res.Status != string(domain.WastePending) {
			t.Fatalf("= %+v, %v; sin costo no se sabe cuánto vale", res, err)
		}
		if _, err := st.Pool.Exec(ctx, `update ingredients set current_cost = 7 where id = $1`, sin); err != nil {
			t.Fatal(err)
		}
		if err := svc.ApproveWaste(ctx, res.ID, gerente); err != nil {
			t.Fatal(err)
		}
		var total *string
		if err := st.Pool.QueryRow(ctx, `select total_cost::text from waste_records where id = $1`, res.ID).Scan(&total); err != nil {
			t.Fatal(err)
		}
		if total == nil || *total != "7.00" {
			t.Fatalf("costo al aprobar = %v; quiere 7.00 (si no, queda sin costo para siempre)", total)
		}
	})

	t.Run("sin tope ninguna espera", func(t *testing.T) {
		setWasteThreshold(t, ctx, svc, gerente, nil)
		res, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("5"), Actor: cajero})
		if err != nil || res.Status != string(domain.WasteApplied) {
			t.Fatalf("= %+v, %v; sin tope resta de una vez", res, err)
		}
	})
}

// Deshacer devuelve exactamente lo restado, con un movimiento propio, y nunca borra (FR-009).
func TestUndoingWasteGivesBackWhatItTook(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	cajero := makeUser(t, st, "cajero_deshacer", "cajero")
	otro := makeUser(t, st, "cajero_ajeno_deshacer", "cajero")
	svc, ctx := asWaste(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	ing := pieceIngredient(t, st, "Insumo de deshacer")
	adjust(t, st, "ingrediente", ing, "10")

	res, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("3"), Actor: cajero})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UndoWaste(ctx, res.ID, otro, false); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("deshacer la de otro sin permiso de configurar = %v; quiere ErrForbidden", err)
	}
	if err := svc.UndoWaste(ctx, res.ID, cajero, false); err != nil {
		t.Fatal(err)
	}
	if got := onHandIngredient(t, st, ing); !got.Equal(qty("10")) {
		t.Fatalf("quedaron %s; deshacer devuelve las 3", got)
	}
	if err := svc.UndoWaste(ctx, res.ID, cajero, false); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("deshacer dos veces = %v; devolvería de más", err)
	}
	var status string
	var movimientos int
	if err := st.Pool.QueryRow(ctx, `select status, (select count(*) from stock_movements where waste_record_id = $1)
		from waste_records where id = $1`, res.ID).Scan(&status, &movimientos); err != nil {
		t.Fatal(err)
	}
	if status != "undone" || movimientos != 2 {
		t.Fatalf("estado %s con %d movimientos; quiere undone con el que restó y el que devolvió", status, movimientos)
	}

	t.Run("con el insumo ya contado después no se deshace", func(t *testing.T) {
		res, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("1"), Actor: cajero})
		if err != nil {
			t.Fatal(err)
		}
		closeCountOf(t, ing, "9", cajero)
		if err := svc.UndoWaste(ctx, res.ID, cajero, false); !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("= %v; el conteo ya corrigió lo guardado", err)
		}
	})
}

// closeCountOf abre, captura y cierra un conteo de un solo insumo con la cifra dada, como el
// servicio de verdad.
func closeCountOf(t *testing.T, ing int64, counted string, actor int64) {
	t.Helper()
	svc, ctx := asWaste(t, defaultCompanyID)
	c, err := svc.OpenCount(ctx, actor)
	if err != nil {
		t.Fatalf("abrir conteo: %v", err)
	}
	if err := svc.CaptureCount(ctx, c.ID, ing, qty(counted), actor); err != nil {
		t.Fatalf("capturar: %v", err)
	}
	if _, err := svc.CloseCount(ctx, c.ID, actor); err != nil {
		t.Fatalf("cerrar: %v", err)
	}
}
