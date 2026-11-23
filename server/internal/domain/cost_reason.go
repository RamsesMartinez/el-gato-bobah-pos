package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxCostReason es el largo del motivo de una corrección de costo; el check de la 0116 dice lo mismo.
const MaxCostReason = 200

// CostCorrectionReason valida el motivo de una corrección de costo (spec 090). Ausente es válido: la
// pantalla corrige sin motivo. Presente se recorta, y vacío o largo de más se rechaza: un motivo de
// solo espacios no explica nada.
func CostCorrectionReason(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	r := strings.TrimSpace(*raw)
	if r == "" || utf8.RuneCountInString(r) > MaxCostReason {
		return nil, fmt.Errorf("%w: el motivo lleva de 1 a %d letras", ErrValidation, MaxCostReason)
	}
	return &r, nil
}
