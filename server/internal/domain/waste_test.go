package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func dp(s string) *decimal.Decimal {
	v := d(s)
	return &v
}

// Las únicas transiciones de una merma son las del data-model; cualquier otra es un 409, no un
// estado nuevo. Aprobar dos veces o deshacer algo rechazado restaría o devolvería de más.
func TestWasteTransitions(t *testing.T) {
	cases := []struct {
		from WasteStatus
		act  WasteAction
		to   WasteStatus
		ok   bool
	}{
		{WasteApplied, WasteUndo, WasteUndone, true},
		{WastePending, WasteApprove, WasteApplied, true},
		{WastePending, WasteReject, WasteRejected, true},
		{WastePending, WasteUndo, WasteUndone, true},
		{WastePending, WasteCover, WasteCovered, true},
		{WasteApplied, WasteApprove, "", false},
		{WasteApplied, WasteReject, "", false},
		{WasteApplied, WasteCover, "", false},
		{WasteRejected, WasteApprove, "", false},
		{WasteRejected, WasteUndo, "", false},
		{WasteCovered, WasteApprove, "", false},
		{WasteUndone, WasteUndo, "", false},
		{WasteUndone, WasteApprove, "", false},
	}
	for _, c := range cases {
		got, err := NextWasteStatus(c.from, c.act)
		if c.ok && (err != nil || got != c.to) {
			t.Errorf("%s + %s = %s, %v; quiere %s", c.from, c.act, got, err, c.to)
		}
		if !c.ok && !errors.Is(err, ErrConflict) {
			t.Errorf("%s + %s = %s, %v; quiere ErrConflict", c.from, c.act, got, err)
		}
	}
}

func TestWasteNeedsApproval(t *testing.T) {
	cases := []struct {
		name      string
		threshold *decimal.Decimal
		cost      *decimal.Decimal
		want      bool
	}{
		{"sin tope nada pide aprobación", nil, dp("5000"), false},
		{"sin tope ni costo tampoco", nil, nil, false},
		{"debajo del tope", dp("100"), dp("99.99"), false},
		{"justo en el tope no la pide", dp("100"), dp("100"), false},
		{"arriba del tope", dp("100"), dp("100.01"), true},
		{"sin costo con tope: no se sabe cuánto vale", dp("100"), nil, true},
	}
	for _, c := range cases {
		if got := WasteNeedsApproval(c.threshold, c.cost); got != c.want {
			t.Errorf("%s: %v, quiere %v", c.name, got, c.want)
		}
	}
}

func TestWasteCanUndo(t *testing.T) {
	loc, _ := time.LoadLocation("America/Mexico_City")
	recorded := time.Date(2026, 10, 15, 10, 0, 0, 0, loc)
	cases := []struct {
		name         string
		now          time.Time
		countedAfter bool
		ok           bool
	}{
		{"el mismo día", recorded.Add(5 * time.Hour), false, true},
		{"al día siguiente ya no", time.Date(2026, 10, 16, 0, 1, 0, 0, loc), false, false},
		{"contado después: el conteo ya corrigió", recorded.Add(time.Hour), true, false},
		// Las 23:30 en México son las 05:30 del día siguiente en UTC: decide el día del local.
		{"de noche en el local sigue siendo el mismo día", time.Date(2026, 10, 15, 23, 30, 0, 0, loc).UTC(), false, true},
	}
	for _, c := range cases {
		err := WasteCanUndo(recorded, c.now, loc, c.countedAfter)
		if c.ok && err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if !c.ok && !errors.Is(err, ErrConflict) {
			t.Errorf("%s: %v, quiere ErrConflict", c.name, err)
		}
	}
}

// La merma registrada YA restó de lo que el sistema tenía al contar: lo que el conteo encontró de
// menos es lo sin explicar tal cual. Restarle otra vez la merma la contaba dos veces.
func TestSplitLossCountsRegisteredWasteOnce(t *testing.T) {
	cases := []struct {
		name                                   string
		registered, system, counted            string
		wantLost, wantUnexplained, wantSurplus string
	}{
		{"3 registrados y el conteo encontró 2 de menos", "3", "10", "8", "5", "2", "0"},
		{"todo explicado", "3", "10", "10", "3", "0", "0"},
		{"sobraba", "1", "10", "12", "1", "0", "2"},
		{"sin merma", "0", "5", "4.5", "0.5", "0.5", "0"},
	}
	for _, c := range cases {
		got := SplitLoss(d(c.registered), d(c.system), d(c.counted))
		if !got.Lost.Equal(d(c.wantLost)) || !got.Unexplained.Equal(d(c.wantUnexplained)) || !got.Surplus.Equal(d(c.wantSurplus)) {
			t.Errorf("%s: perdido %s, sin explicar %s, sobraba %s; quiere %s/%s/%s (si el perdido sale de más, la merma se contó dos veces)",
				c.name, got.Lost, got.Unexplained, got.Surplus, c.wantLost, c.wantUnexplained, c.wantSurplus)
		}
	}
}

func TestWasteReasonNames(t *testing.T) {
	if NormalizeReasonName("  Se  CAYÓ ") != NormalizeReasonName("se cayo") {
		t.Error("«  Se  CAYÓ » y «se cayo» son el mismo motivo")
	}
	for _, c := range []struct {
		name, desc string
		ok         bool
	}{
		{"Caducó", "Pasó su fecha", true},
		{"", "algo", false},
		{"   ", "algo", false},
		{"Caducó", "", false},
		{"Caducó", "   ", false},
		{string(make([]byte, 41)), "algo", false},
	} {
		err := ValidWasteReason(c.name, c.desc)
		if c.ok != (err == nil) {
			t.Errorf("ValidWasteReason(%q, %q) = %v", c.name, c.desc, err)
		}
		if err != nil && !errors.Is(err, ErrValidation) {
			t.Errorf("ValidWasteReason(%q) = %v, quiere ErrValidation", c.name, err)
		}
	}
}
