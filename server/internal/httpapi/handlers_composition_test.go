package httpapi

import (
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Un filtro de composición mal escrito se rechaza: volverlo «todos» mostraría una lista que parece
// la pedida y no lo es (constitución V).
func TestCompositionFilterRejectsUnknownValues(t *testing.T) {
	for _, v := range []string{"", "none", "estimated"} {
		if got, err := compositionFilter(v); err != nil || got != v {
			t.Fatalf("compositionFilter(%q) = %q, %v", v, got, err)
		}
	}
	for _, v := range []string{"confirmed", "None", "sin"} {
		if _, err := compositionFilter(v); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("compositionFilter(%q) debe rechazarse, fue %v", v, err)
		}
	}
}
