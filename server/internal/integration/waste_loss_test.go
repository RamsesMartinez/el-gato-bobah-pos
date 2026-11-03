//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// EL FALTANTE SIN EXPLICAR (spec 039, historia 2): después de un conteo, lo perdido desde el anterior
// se parte en lo registrado como merma y lo que el conteo encontró de menos, sin contar la merma dos
// veces, con el costo del cierre.

func countAndClose(t *testing.T, svc *app.BackofficeService, ctx context.Context, ing int64, counted string, actor int64) int64 {
	t.Helper()
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
	return c.ID
}

func lossOf(t *testing.T, svc *app.BackofficeService, ctx context.Context, countID, ing int64) app.LossItem {
	t.Helper()
	v, err := svc.CountLoss(ctx, countID)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range v.Items {
		if it.IngredientID == ing {
			return it
		}
	}
	t.Fatalf("el insumo %d no está en el faltante del conteo %d", ing, countID)
	return app.LossItem{}
}

func TestTheLossOfACountSplitsRegisteredAndUnexplained(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	actor := makeUser(t, st, "gerente_faltante", "gerente")
	svc, ctx := asWaste(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	ing := pieceIngredient(t, st, "Insumo del faltante")
	adjust(t, st, "ingrediente", ing, "10")

	first := countAndClose(t, svc, ctx, ing, "10", actor)
	t.Run("el primer conteo no tiene con qué comparar", func(t *testing.T) {
		v, err := svc.CountLoss(ctx, first)
		if err != nil || !v.FirstCount {
			t.Fatalf("= %+v, %v; quiere «primer conteo»", v, err)
		}
	})

	if _, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("3"), Actor: actor}); err != nil {
		t.Fatal(err)
	}
	// Una deshecha no cuenta: resta y devuelve dentro del periodo.
	undone, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("1"), Actor: actor})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UndoWaste(ctx, undone.ID, actor, true); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `update ingredients set current_cost = 10 where id = $1`, ing); err != nil {
		t.Fatal(err)
	}
	second := countAndClose(t, svc, ctx, ing, "5", actor)

	t.Run("3 registrados y el conteo encontró 2 de menos: se perdieron 5", func(t *testing.T) {
		it := lossOf(t, svc, ctx, second, ing)
		if !it.Registered.Equal(qty("3")) || !it.Unexplained.Equal(qty("2")) || !it.Lost.Equal(qty("5")) {
			t.Fatalf("registrado %s, sin explicar %s, perdido %s; quiere 3, 2 y 5 (si el perdido es 8, la merma se contó dos veces)",
				it.Registered, it.Unexplained, it.Lost)
		}
		if it.UnitCost == nil || !it.UnitCost.Equal(qty("10")) {
			t.Fatalf("costo %v; quiere el del cierre (10)", it.UnitCost)
		}
	})

	t.Run("el reporte cerrado no cambia cuando cambia el costo", func(t *testing.T) {
		if _, err := st.Pool.Exec(ctx, `update ingredients set current_cost = 99 where id = $1`, ing); err != nil {
			t.Fatal(err)
		}
		if it := lossOf(t, svc, ctx, second, ing); it.UnitCost == nil || !it.UnitCost.Equal(qty("10")) {
			t.Fatalf("costo %v; el cierre guardó 10", it.UnitCost)
		}
	})

	t.Run("sobraba se dice como sobraba", func(t *testing.T) {
		adjust(t, st, "ingrediente", ing, "0.5") // nada que ver con el conteo: el sistema queda en 5.5
		third := countAndClose(t, svc, ctx, ing, "7", actor)
		it := lossOf(t, svc, ctx, third, ing)
		if !it.Surplus.Equal(qty("1.5")) || !it.Unexplained.IsZero() {
			t.Fatalf("sobraba %s, sin explicar %s; quiere 1.5 y 0", it.Surplus, it.Unexplained)
		}
	})

	inTheThreeCases(t, defaultCompanyID, makeCompany(t, st, "otra-faltante"), func(t *testing.T, as *store.Store, c context.Context) {
		v, err := app.NewBackofficeService(as, clock).CountLoss(c, second)
		if err == nil && len(v.Items) > 0 {
			t.Fatal("se vio el faltante del conteo de otra empresa")
		}
	})
}

// Una merma que empezó antes de la captura y confirma a la mitad: la captura espera a que confirme y
// lee la existencia y el último movimiento del mismo momento. Sin eso, el conteo la ajustaría otra
// vez y lo guardado quedaría con la merma restada dos veces (revisión de arquitectura, E14).
func TestAWasteCommittingDuringTheCaptureIsCountedOnce(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	actor := makeUser(t, st, "gerente_merma_en_vuelo", "gerente")
	svc, ctx := asWaste(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	ing := pieceIngredient(t, st, "Insumo de la merma en vuelo")
	adjust(t, st, "ingrediente", ing, "10")
	countAndClose(t, svc, ctx, ing, "10", actor)

	bg := context.Background()
	w, err := st.Pool.Begin(bg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Rollback(bg) }()
	var rec int64
	if err := w.QueryRow(bg, `
		insert into waste_records (company_id, branch_id, reason_id, item_type, ingredient_id, quantity, entered_qty, status, recorded_by, is_staff_meal)
		values ($1, (select id from branches where company_id = $1 and is_headquarters), $2, 'ingrediente', $3, 2, 2, 'applied', $4, false)
		returning id`, defaultCompanyID, caduco, ing, actor).Scan(&rec); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Exec(bg, `
		insert into stock_movements (company_id, item_type, ingredient_id, movement_type, quantity, waste_record_id, branch_id)
		values ($1, 'ingrediente', $2, 'merma', -2, $3, (select id from branches where company_id = $1 and is_headquarters))`,
		defaultCompanyID, ing, rec); err != nil {
		t.Fatal(err)
	}

	c, err := svc.OpenCount(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	captured := make(chan error, 1)
	go func() { captured <- svc.CaptureCount(ctx, c.ID, ing, qty("7"), actor) }()
	waitForALockWait(t, st)
	if err := w.Commit(bg); err != nil {
		t.Fatal(err)
	}
	if err := <-captured; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CloseCount(ctx, c.ID, actor); err != nil {
		t.Fatal(err)
	}
	if got := onHandIngredient(t, st, ing); !got.Equal(qty("7")) {
		t.Fatalf("quedaron %s; se contaron 7 (si quedan 5, la merma se restó dos veces)", got)
	}
	it := lossOf(t, svc, ctx, c.ID, ing)
	if !it.Registered.Equal(qty("2")) || !it.Unexplained.Equal(qty("1")) {
		t.Fatalf("registrado %s, sin explicar %s; quiere 2 y 1", it.Registered, it.Unexplained)
	}
}

func waitForALockWait(t *testing.T, st *store.Store) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var n int
		if err := st.Pool.QueryRow(context.Background(),
			`select count(*) from pg_stat_activity where datname = current_database() and wait_event_type = 'Lock'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("la captura no esperó a la merma en curso: leyó una existencia que estaba cambiando")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Las pendientes que el conteo vio quedan cubiertas al cerrar; las registradas después de la
// captura, no. Un conteo descartado no cubre nada (FR-013).
func TestClosingACountCoversOnlyThePendingWasteItSaw(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	actor := makeUser(t, st, "gerente_cubre", "gerente")
	svc, ctx := asWaste(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	tope := "1"
	setWasteThreshold(t, ctx, svc, actor, &tope)
	ing := pieceIngredient(t, st, "Insumo de las pendientes")
	adjust(t, st, "ingrediente", ing, "10")
	if _, err := st.Pool.Exec(ctx, `update ingredients set current_cost = 50 where id = $1`, ing); err != nil {
		t.Fatal(err)
	}
	pend := func() int64 {
		t.Helper()
		r, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: caduco, IngredientID: &ing, Qty: qty("1"), Actor: actor})
		if err != nil || r.Status != string(domain.WastePending) {
			t.Fatalf("= %+v, %v", r, err)
		}
		return r.ID
	}

	discarded, err := svc.OpenCount(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	early := pend()
	if err := svc.CaptureCount(ctx, discarded.ID, ing, qty("9"), actor); err != nil {
		t.Fatal(err)
	}
	if err := svc.DiscardCount(ctx, discarded.ID, actor); err != nil {
		t.Fatal(err)
	}
	status := func(id int64) string {
		t.Helper()
		var s string
		if err := st.Pool.QueryRow(ctx, `select status from waste_records where id = $1`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if status(early) != "pending" {
		t.Fatal("un conteo descartado cubrió una merma")
	}

	c, err := svc.OpenCount(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CaptureCount(ctx, c.ID, ing, qty("9"), actor); err != nil {
		t.Fatal(err)
	}
	late := pend()
	if _, err := svc.CloseCount(ctx, c.ID, actor); err != nil {
		t.Fatal(err)
	}
	if status(early) != "covered" {
		t.Fatalf("la pendiente registrada antes de la captura quedó %s; quiere covered", status(early))
	}
	if status(late) != "pending" {
		t.Fatalf("la registrada después de la captura quedó %s: el conteo no la vio, sigue por aprobar", status(late))
	}
	if err := svc.ApproveWaste(ctx, early, actor); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("aprobar una cubierta = %v; la restaría dos veces", err)
	}
}
