package domain

import (
	"errors"
	"strings"
	"testing"
)

// El motivo de una corrección de costo (spec 090): ausente es válido (la pantalla no lo pide);
// presente se recorta y no puede quedar vacío ni pasar de 200 letras (lo mismo que el check de la 0116).
func TestCostCorrectionReason(t *testing.T) {
	s := func(v string) *string { return &v }
	ok, err := CostCorrectionReason(s("  difiere de la compra  "))
	if err != nil || ok == nil || *ok != "difiere de la compra" {
		t.Fatalf("recortado: %v %v", ok, err)
	}
	if r, err := CostCorrectionReason(nil); err != nil || r != nil {
		t.Fatalf("ausente: %v %v", r, err)
	}
	for _, bad := range []string{"", "   ", strings.Repeat("ñ", 201)} {
		if _, err := CostCorrectionReason(s(bad)); !errors.Is(err, ErrValidation) {
			t.Errorf("%.10q: %v", bad, err)
		}
	}
	if r, err := CostCorrectionReason(s(strings.Repeat("ñ", 200))); err != nil || r == nil {
		t.Fatalf("200 letras con acento cuentan como 200, no como 400 bytes: %v", err)
	}
}
