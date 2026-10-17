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

// POR MOTIVO Y LOS MOTIVOS DE LA EMPRESA (spec 039, historia 3). Juntar no reescribe el pasado:
// cada registro conserva su motivo y el reporte suma bajo el que queda.

func todayRange(t *testing.T, st *store.Store) (time.Time, time.Time) {
	t.Helper()
	var now time.Time
	if err := st.Pool.QueryRow(context.Background(), `select now()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return day.AddDate(0, 0, -1), day.AddDate(0, 0, 1)
}

func TestWasteByReasonAndManagingReasons(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	actor := makeUser(t, st, "gerente_motivos", "gerente")
	svc, ctx := asWaste(t, defaultCompanyID)
	caduco := reasonNamed(t, st, defaultCompanyID, "Caducó")
	personal := reasonNamed(t, st, defaultCompanyID, "Consumo del personal")
	ing := pieceIngredient(t, st, "Insumo de los motivos")
	adjust(t, st, "ingrediente", ing, "50")
	if _, err := st.Pool.Exec(ctx, `update ingredients set current_cost = 10 where id = $1`, ing); err != nil {
		t.Fatal(err)
	}
	record := func(reason int64, q string) int64 {
		t.Helper()
		r, err := svc.RecordWaste(ctx, app.RecordWasteCmd{ReasonID: reason, IngredientID: &ing, Qty: qty(q), Actor: actor})
		if err != nil {
			t.Fatal(err)
		}
		return r.ID
	}
	record(caduco, "2")
	record(personal, "1")
	derretido, err := svc.CreateWasteReason(ctx, "Se derritió", "Hielo fuera del refri", actor)
	if err != nil {
		t.Fatal(err)
	}
	conDerretido := record(derretido, "3")
	from, to := todayRange(t, st)

	totals := func() map[int64]app.WasteReasonTotal {
		t.Helper()
		v, err := svc.WasteByReason(ctx, from, to)
		if err != nil {
			t.Fatal(err)
		}
		out := map[int64]app.WasteReasonTotal{}
		for _, r := range v {
			out[r.ReasonID] = r
		}
		return out
	}

	t.Run("cada motivo con su total y el del personal aparte", func(t *testing.T) {
		got := totals()
		if !got[caduco].TotalCost.Equal(qty("20")) || got[caduco].IsStaffMeal {
			t.Fatalf("caducó = %+v; quiere $20 y no del personal", got[caduco])
		}
		if !got[personal].IsStaffMeal || !got[personal].TotalCost.Equal(qty("10")) {
			t.Fatalf("personal = %+v; quiere $10 aparte", got[personal])
		}
	})

	t.Run("juntar suma bajo el que queda sin cambiar el motivo de ningún registro (E16)", func(t *testing.T) {
		if err := svc.MergeWasteReason(ctx, derretido, caduco, actor); err != nil {
			t.Fatal(err)
		}
		got := totals()
		if !got[caduco].TotalCost.Equal(qty("50")) {
			t.Fatalf("caducó tras juntar = %s; quiere $50", got[caduco].TotalCost)
		}
		var reason int64
		if err := st.Pool.QueryRow(ctx, `select reason_id from waste_records where id = $1`, conDerretido).Scan(&reason); err != nil {
			t.Fatal(err)
		}
		if reason != derretido {
			t.Fatalf("el registro quedó con el motivo %d; quiere el original %d (juntar no reescribe el pasado)", reason, derretido)
		}
		rows, err := svc.ListWaste(ctx, from, to, &caduco)
		if err != nil || len(rows) != 2 {
			t.Fatalf("filtrar por caducó = %d registros, %v; quiere los 2 (el suyo y el juntado)", len(rows), err)
		}
	})

	t.Run("no se junta con uno juntado, archivado o consigo mismo", func(t *testing.T) {
		for _, into := range []int64{derretido, caduco} {
			if err := svc.MergeWasteReason(ctx, caduco, into, actor); err == nil {
				t.Errorf("juntar caducó con %d = nil; quiere un rechazo", into)
			}
		}
	})

	t.Run("cambiar «no cuenta como pérdida» no reclasifica el pasado", func(t *testing.T) {
		if err := svc.UpdateWasteReason(ctx, caduco, app.WasteReasonUpdate{Name: "Caducó", Description: "Pasó su fecha", IsStaffMeal: true}, actor); err != nil {
			t.Fatal(err)
		}
		if got := totals(); got[caduco].IsStaffMeal {
			t.Fatal("lo registrado antes cambió de grupo al cambiar la marca del motivo")
		}
	})

	t.Run("archivar deja de ofrecerlo sin cambiar lo registrado", func(t *testing.T) {
		if err := svc.UpdateWasteReason(ctx, personal, app.WasteReasonUpdate{Name: "Consumo del personal", Description: "Lo del equipo", IsStaffMeal: true, Archived: true}, actor); err != nil {
			t.Fatal(err)
		}
		active, err := svc.WasteReasons(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range active {
			if r.ID == personal {
				t.Fatal("un motivo archivado se sigue ofreciendo")
			}
		}
		if got := totals(); !got[personal].TotalCost.Equal(qty("10")) {
			t.Fatal("archivar cambió lo registrado")
		}
	})

	t.Run("renombrar a un nombre que ya existe es conflicto", func(t *testing.T) {
		err := svc.UpdateWasteReason(ctx, reasonNamed(t, st, defaultCompanyID, "Se cayó"), app.WasteReasonUpdate{Name: " caduco ", Description: "x"}, actor)
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("= %v; quiere ErrConflict", err)
		}
	})

	inTheThreeCases(t, defaultCompanyID, makeCompany(t, st, "otra-motivos"), func(t *testing.T, as *store.Store, c context.Context) {
		s := app.NewBackofficeService(as, clock)
		if v, err := s.WasteByReason(c, from, to); err == nil {
			for _, r := range v {
				if r.ReasonID == caduco {
					t.Fatal("se vieron los totales por motivo de otra empresa")
				}
			}
		}
		if err := s.MergeWasteReason(c, derretido, caduco, actor); err == nil {
			t.Fatal("una sesión ajena juntó motivos de la dueña")
		}
	})
}

// Borrar una empresa con mermas cae por cascada: la llave del movimiento a su registro no lo frena
// (no action, no restrict; revisión de arquitectura, E17).
func TestDeletingACompanyWithWasteCascades(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	co := makeCompany(t, st, "borrar-con-merma")
	user := makeUserIn(t, st, co, "cajero_borrar_merma", "cajero")
	ctx := context.Background()
	var ing int64
	if err := st.Pool.QueryRow(ctx, `insert into ingredients (company_id, name, base_unit_id)
		select $1, 'Insumo a borrar', id from units where code = 'pieza' returning id`, co).Scan(&ing); err != nil {
		t.Fatal(err)
	}
	as := appRoleStore(t)
	actx, release, err := as.AcquireTenant(ctx, co)
	if err != nil {
		t.Fatal(err)
	}
	reason := reasonNamed(t, st, co, "Caducó")
	if _, err := app.NewBackofficeService(as, clock).RecordWaste(actx, app.RecordWasteCmd{ReasonID: reason, IngredientID: &ing, Qty: qty("1"), Actor: user}); err != nil {
		release()
		t.Fatal(err)
	}
	release()
	if _, err := st.Pool.Exec(ctx, `delete from companies where id = $1`, co); err != nil {
		t.Fatalf("borrar una empresa con mermas: %v", err)
	}
}
