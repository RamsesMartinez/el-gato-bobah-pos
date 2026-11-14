package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

// Correcciones de un corte cerrado (#290, fase 3). Lo firmado al cerrar no se reescribe: una
// corrección se agrega con el valor de antes, el de después, quién, cuándo y por qué, y se deshace
// con otra que la revierte. Mismo precedente que el reverso de una salida (0088).
const (
	AdjustmentCountCorrection = "count_correction"
	AdjustmentFloatCorrection = "float_correction"
	AdjustmentNote            = "note"
	AdjustmentReversal        = "reversal"
)

var adjustmentReasons = map[string]bool{
	"miscount": true, "unrecorded_movement": true, "wrong_register": true, "float_error": true, "other": true,
}

var (
	ErrSessionNotClosed   = fmt.Errorf("%w: el corte sigue abierto: se corrige al cerrarlo", ErrConflict)
	ErrAdjustmentReversed = fmt.Errorf("%w: esa corrección ya se revirtió", ErrConflict)
	ErrNoSignedCount      = fmt.Errorf("%w: este corte no tiene arqueo del cajón que corregir; deja una nota", ErrValidation)
	// ErrAdjustmentNotLatest: solo se revierte la corrección vigente de su tipo. Revertir una de abajo
	// con otra encima dejaba rigiendo un valor que nadie firmó.
	ErrAdjustmentNotLatest = fmt.Errorf("%w: solo se revierte la última corrección de ese dato; revierte primero la más reciente", ErrConflict)
)

// AdjustmentInput es lo que pide quien corrige. After es nil en una nota.
type AdjustmentInput struct {
	Kind   string
	After  *decimal.Decimal
	Reason string
	Note   string
}

// ValidateAdjustment valida y normaliza una corrección. `current` es el valor vigente de lo que se
// corrige (lo firmado o la última corrección), nil si el corte no lo guardó.
func ValidateAdjustment(in AdjustmentInput, current *decimal.Decimal) (AdjustmentInput, error) {
	in.Note = strings.TrimSpace(in.Note)
	// PrintableName rechaza los caracteres de control: un NUL llegaba a Postgres y salía como 500.
	if !adjustmentReasons[in.Reason] || in.Note == "" || utf8.RuneCountInString(in.Note) > 200 || !PrintableName(in.Note) {
		return in, ErrValidation
	}
	switch in.Kind {
	case AdjustmentNote:
		if in.After != nil {
			return in, ErrValidation
		}
		return in, nil
	case AdjustmentCountCorrection, AdjustmentFloatCorrection:
	default:
		return in, ErrValidation
	}
	if in.Kind == AdjustmentCountCorrection && current == nil {
		return in, ErrNoSignedCount
	}
	if in.After == nil {
		return in, ErrValidation
	}
	after := Round2(*in.After)
	if !ValidMoney(after, true) || after.IsNegative() {
		return in, ErrValidation
	}
	if current != nil && after.Equal(Round2(*current)) {
		return in, fmt.Errorf("%w: el monto es el mismo que ya tiene el corte", ErrValidation)
	}
	in.After = &after
	return in, nil
}

// CanAdjustSession: solo un corte cerrado se corrige; uno abierto todavía se cuenta al cerrar.
func CanAdjustSession(status string) error {
	if status != "cerrada" {
		return ErrSessionNotClosed
	}
	return nil
}

// CanReverseAdjustment: un reverso no se revierte, una corrección se revierte una sola vez, y una
// corrección de monto solo si es la vigente de su tipo (`effective`). Las notas no rigen nada: se
// revierten siempre.
func CanReverseAdjustment(kind string, alreadyReversed, effective bool) error {
	if kind == AdjustmentReversal {
		return fmt.Errorf("%w: un reverso no se revierte; registra otra corrección", ErrValidation)
	}
	if alreadyReversed {
		return ErrAdjustmentReversed
	}
	if kind != AdjustmentNote && !effective {
		return ErrAdjustmentNotLatest
	}
	return nil
}

// Adjustment es una corrección ya guardada, en el orden en que se registró.
type Adjustment struct {
	ID         int64
	Kind       string
	After      *decimal.Decimal
	ReversesID *int64
}

// EffectiveAdjustment devuelve la corrección vigente de un tipo: la última que no se revirtió, o
// nil si no hay ninguna (rige lo firmado).
func EffectiveAdjustment(adjs []Adjustment, kind string) *Adjustment {
	revertida := map[int64]bool{}
	for _, a := range adjs {
		if a.ReversesID != nil {
			revertida[*a.ReversesID] = true
		}
	}
	for i := len(adjs) - 1; i >= 0; i-- {
		if adjs[i].Kind == kind && !revertida[adjs[i].ID] {
			return &adjs[i]
		}
	}
	return nil
}
