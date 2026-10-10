//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// NINGUNA SALIDA SIN CONCEPTO (punto 3, EB-18), y el concepto nuevo se agrega en la misma captura
// sin duplicar uno que ya existe con otras mayúsculas (EB-20).
func TestCashOutNeedsConceptAndConceptsDoNotDuplicate(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	conceptos := app.NewCashConceptsService(st)
	cajero := makeUser(t, st, "cajero_conceptos", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	reg := principalRegister(t, st)

	if _, err := back.RecordCashMovement(ctx, reg, app.CashMovementCmd{Kind: "salida", Amount: dec("50"), UserID: cajero}); !errors.Is(err, domain.ErrConceptRequired) {
		t.Fatalf("salida sin concepto: err = %v, quería ErrConceptRequired", err)
	}
	hielo, err := conceptos.Create(ctx, "  hielo ")
	if err != nil {
		t.Fatalf("crear un concepto que ya existe debe devolver el existente: %v", err)
	}
	lista, err := conceptos.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for _, c := range lista {
		if c.Name == "Hielo" || c.Name == "hielo" {
			n++
			if c.ID != hielo.ID {
				t.Fatalf("«hielo» creó otro concepto (%d) en vez de usar «Hielo» (%d)", hielo.ID, c.ID)
			}
		}
	}
	if n != 1 {
		t.Fatalf("hay %d conceptos «hielo»", n)
	}
	gas, err := conceptos.Create(ctx, "Gas")
	if err != nil {
		t.Fatal(err)
	}
	v, err := back.RecordCashMovement(ctx, reg, app.CashMovementCmd{Kind: "salida", Amount: dec("120"), ConceptID: &gas.ID, UserID: cajero})
	if err != nil {
		t.Fatal(err)
	}
	if v.Movements[len(v.Movements)-1].Concept != "Gas" {
		t.Fatalf("la salida no guardó el nombre del concepto: %+v", v.Movements)
	}
}

// CORREGIR NO EDITA: crea el reverso ligado y una salida nueva; el corte cuadra y la original
// sigue a la vista. Una salida se corrige una sola vez (EB-15, EB-16).
func TestCorrectCashOutCreatesReversalAndReplacement(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	conceptos := app.NewCashConceptsService(st)
	cajero := makeUser(t, st, "cajero_corregir", "gerente")
	abrirCajaPrincipal(t, st, cajero)
	reg := principalRegister(t, st)
	basura, _ := conceptos.Create(ctx, "Basura")
	hielo, _ := conceptos.Create(ctx, "Hielo")

	antes, _ := back.CurrentByRegister(ctx, reg)
	v, err := back.RecordCashMovement(ctx, reg, app.CashMovementCmd{Kind: "salida", Amount: dec("100"), ConceptID: &basura.ID, UserID: cajero})
	if err != nil {
		t.Fatal(err)
	}
	orig := v.Movements[len(v.Movements)-1].ID
	if _, err := back.CorrectCashOut(ctx, orig, app.CashMovementCmd{Kind: "salida", Amount: dec("45"), ConceptID: &hielo.ID, UserID: cajero}); err != nil {
		t.Fatalf("corregir: %v", err)
	}
	despues, _ := back.CurrentByRegister(ctx, reg)
	if !antes.NetMovements.Sub(despues.NetMovements).Equal(dec("45")) {
		t.Fatalf("el neto bajó %s; tras corregir 100 por 45 debía bajar 45", antes.NetMovements.Sub(despues.NetMovements))
	}
	var original, reversos int
	for _, m := range despues.Movements {
		if m.ID == orig && m.Amount.Equal(dec("100")) {
			original++
		}
		if m.Kind == domain.CashReverso {
			reversos++
		}
	}
	if original != 1 || reversos != 1 {
		t.Fatalf("original visible %d, reversos %d; quería 1 y 1", original, reversos)
	}
	if _, err := back.CorrectCashOut(ctx, orig, app.CashMovementCmd{Kind: "salida", Amount: dec("40"), ConceptID: &hielo.ID, UserID: cajero}); !errors.Is(err, domain.ErrAlreadyReversed) {
		t.Fatalf("corregir dos veces: err = %v", err)
	}
}

// Una salida de un turno ya cerrado se corrige en el turno abierto y el corte cerrado no cambia
// (EB-17).
func TestCorrectingClosedShiftCashOutLandsInOpenShift(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	conceptos := app.NewCashConceptsService(st)
	cajero := makeUser(t, st, "cajero_corregir_cerrado", "gerente")
	viejo := abrirCajaPrincipal(t, st, cajero)
	reg := principalRegister(t, st)
	basura, _ := conceptos.Create(ctx, "Basura")
	v, err := back.RecordCashMovement(ctx, reg, app.CashMovementCmd{Kind: "salida", Amount: dec("30"), ConceptID: &basura.ID, UserID: cajero})
	if err != nil {
		t.Fatal(err)
	}
	orig := v.Movements[len(v.Movements)-1].ID
	if _, err := st.Pool.Exec(ctx, `update register_sessions set status = 'cerrada', closed_at = now() where id = $1`, viejo); err != nil {
		t.Fatal(err)
	}
	nuevo := abrirCajaPrincipal(t, st, cajero)
	if _, err := back.CorrectCashOut(ctx, orig, app.CashMovementCmd{Kind: "salida", Amount: dec("20"), ConceptID: &basura.ID, UserID: cajero}); err != nil {
		t.Fatal(err)
	}
	var enViejo, enNuevo int
	_ = st.Pool.QueryRow(ctx, `select count(*) from register_cash_movements where session_id = $1`, viejo).Scan(&enViejo)
	_ = st.Pool.QueryRow(ctx, `select count(*) from register_cash_movements where session_id = $1`, nuevo).Scan(&enNuevo)
	if enViejo != 1 || enNuevo != 2 {
		t.Fatalf("turno cerrado %d movimientos (quería 1), abierto %d (quería reverso y nueva)", enViejo, enNuevo)
	}
}

// Fusionar mueve las salidas del duplicado al que queda y archiva el duplicado (EB-19).
func TestMergeConceptsMovesCashOuts(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	conceptos := app.NewCashConceptsService(st)
	cajero := makeUser(t, st, "cajero_fusion", "admin")
	abrirCajaPrincipal(t, st, cajero)
	reg := principalRegister(t, st)
	a, _ := conceptos.Create(ctx, "Garrafón")
	b, _ := conceptos.Create(ctx, "Agua")
	if _, err := back.RecordCashMovement(ctx, reg, app.CashMovementCmd{Kind: "salida", Amount: dec("30"), ConceptID: &a.ID, UserID: cajero}); err != nil {
		t.Fatal(err)
	}
	if err := conceptos.Merge(ctx, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = st.Pool.QueryRow(ctx, `select count(*) from register_cash_movements where concept_id = $1`, b.ID).Scan(&n)
	if n != 1 {
		t.Fatalf("las salidas del duplicado no pasaron al que queda: %d", n)
	}
	lista, _ := conceptos.List(ctx)
	for _, c := range lista {
		if c.ID == a.ID {
			t.Fatal("el duplicado sigue ofreciéndose")
		}
	}
	if err := conceptos.Merge(ctx, b.ID, a.ID); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("fusionar hacia un archivado: err = %v (abriría un ciclo)", err)
	}
}

func TestCashConceptsIsolatedInTheThreeCases(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	if _, err := app.NewCashConceptsService(st).Create(ctx, "Solo de la dueña"); err != nil {
		t.Fatal(err)
	}
	otra := makeCompany(t, st, "otra-conceptos")
	inTheThreeCases(t, defaultCompanyID, otra, func(t *testing.T, as *store.Store, ctx context.Context) {
		lista, err := app.NewCashConceptsService(as).List(ctx)
		if err != nil {
			return
		}
		for _, c := range lista {
			if c.Name == "Solo de la dueña" {
				t.Fatal("se vio un concepto de otra empresa")
			}
		}
	})
}
