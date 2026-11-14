package domain

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

// Las correcciones de un corte cerrado (#290, fase 3): qué se acepta y qué no, antes de tocar la base.
func TestValidateAdjustment(t *testing.T) {
	mil := d("1000")
	cases := []struct {
		name    string
		in      AdjustmentInput
		current *decimal.Decimal
		want    error
	}{
		{"corregir el conteo", AdjustmentInput{Kind: "count_correction", After: dp("1120"), Reason: "miscount", Note: "Se contó mal"}, &mil, nil},
		{"corregir el fondo de un corte que no lo guardó", AdjustmentInput{Kind: "float_correction", After: dp("500"), Reason: "float_error", Note: "x"}, nil, nil},
		{"una nota", AdjustmentInput{Kind: "note", Reason: "other", Note: "El gasto salió de esta caja"}, nil, nil},
		{"tipo desconocido", AdjustmentInput{Kind: "rewrite", After: dp("1"), Reason: "other", Note: "x"}, &mil, ErrValidation},
		{"un reverso no se pide como corrección", AdjustmentInput{Kind: "reversal", Reason: "other", Note: "x"}, nil, ErrValidation},
		{"motivo desconocido", AdjustmentInput{Kind: "note", Reason: "porque sí", Note: "x"}, nil, ErrValidation},
		// Vacío no es un dato: sin nota no hay rastro de por qué se corrigió.
		{"nota vacía", AdjustmentInput{Kind: "note", Reason: "other", Note: "   "}, nil, ErrValidation},
		{"nota de más de 200", AdjustmentInput{Kind: "note", Reason: "other", Note: string(make([]byte, 201))}, nil, ErrValidation},
		// Un monto ausente no es $0: corregir a cero se escribe 0.
		{"corrección sin monto", AdjustmentInput{Kind: "count_correction", Reason: "miscount", Note: "x"}, &mil, ErrValidation},
		{"monto negativo", AdjustmentInput{Kind: "count_correction", After: dp("-1"), Reason: "miscount", Note: "x"}, &mil, ErrValidation},
		{"una nota no lleva monto", AdjustmentInput{Kind: "note", After: dp("1"), Reason: "other", Note: "x"}, nil, ErrValidation},
		// Corregir al mismo valor no deja rastro de nada.
		{"mismo monto que el vigente", AdjustmentInput{Kind: "count_correction", After: dp("1000.00"), Reason: "miscount", Note: "x"}, &mil, ErrValidation},
		// Un corte anterior a la 0067 no tiene arqueo firmado que corregir: se deja una nota.
		// Un carácter de control en la nota era un 500 de Postgres (22021), no un 400 (revisión de seguridad).
		{"nota con un carácter de control", AdjustmentInput{Kind: "note", Reason: "other", Note: "a\x00b"}, nil, ErrValidation},
		{"conteo sin arqueo firmado", AdjustmentInput{Kind: "count_correction", After: dp("1"), Reason: "miscount", Note: "x"}, nil, ErrNoSignedCount},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ValidateAdjustment(c.in, c.current)
			if c.want == nil && err != nil {
				t.Fatalf("se rechazó: %v", err)
			}
			if c.want != nil && !errors.Is(err, c.want) {
				t.Fatalf("err = %v; quería %v", err, c.want)
			}
		})
	}
}

func TestValidateAdjustmentRoundsAndTrims(t *testing.T) {
	got, err := ValidateAdjustment(AdjustmentInput{Kind: "count_correction", After: dp("1120.005"), Reason: "miscount", Note: "  Se contó mal  "}, dp("1000"))
	if err != nil {
		t.Fatal(err)
	}
	if !got.After.Equal(d("1120.01")) || got.Note != "Se contó mal" {
		t.Fatalf("after %s, nota %q", got.After, got.Note)
	}
}

// Solo un corte cerrado se corrige: uno abierto todavía se cuenta al cerrar.
func TestCanAdjustSession(t *testing.T) {
	if err := CanAdjustSession("cerrada"); err != nil {
		t.Fatal(err)
	}
	if err := CanAdjustSession("abierta"); !errors.Is(err, ErrSessionNotClosed) {
		t.Fatalf("un corte abierto se dejó corregir: %v", err)
	}
}

// Un reverso no se revierte, nada se revierte dos veces, y una corrección de monto solo se revierte
// si es la vigente de su tipo: revertir una de abajo con otra encima dejaba rigiendo un valor que ya
// nadie había firmado (revisión de la fase 3).
func TestCanReverseAdjustment(t *testing.T) {
	if err := CanReverseAdjustment("count_correction", false, true); err != nil {
		t.Fatal(err)
	}
	if err := CanReverseAdjustment("count_correction", true, true); !errors.Is(err, ErrAdjustmentReversed) {
		t.Fatalf("revertir dos veces: %v", err)
	}
	if err := CanReverseAdjustment("reversal", false, true); !errors.Is(err, ErrValidation) {
		t.Fatalf("revertir un reverso: %v", err)
	}
	if err := CanReverseAdjustment("count_correction", false, false); !errors.Is(err, ErrAdjustmentNotLatest) {
		t.Fatalf("revertir una corrección que no es la vigente: %v", err)
	}
	if err := CanReverseAdjustment("note", false, false); err != nil {
		t.Fatalf("una nota se revierte siempre: %v", err)
	}
}

// La corrección vigente de un tipo es la última que no se revirtió; sin ninguna, lo firmado.
func TestEffectiveAdjustment(t *testing.T) {
	id := func(n int64) *int64 { return &n }
	adjs := []Adjustment{
		{ID: 1, Kind: "count_correction", After: dp("1100")},
		{ID: 2, Kind: "count_correction", After: dp("1120")},
		{ID: 3, Kind: "reversal", ReversesID: id(2)},
		{ID: 4, Kind: "float_correction", After: dp("500")},
	}
	if got := EffectiveAdjustment(adjs, "count_correction"); got == nil || got.ID != 1 {
		t.Fatalf("vigente del conteo = %+v; quería la 1 (la 2 está revertida)", got)
	}
	if got := EffectiveAdjustment(adjs, "float_correction"); got == nil || got.ID != 4 {
		t.Fatalf("vigente del fondo = %+v", got)
	}
	if got := EffectiveAdjustment(adjs[2:3], "count_correction"); got != nil {
		t.Fatalf("sin correcciones, vigente = %+v", got)
	}
}
