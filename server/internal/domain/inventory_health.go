package domain

import (
	"fmt"
	"time"
)

// Salud del inventario (spec 037): lo que hace que las existencias no cuadren, en cinco secciones.

// HealthSection es una sección de la pantalla. El valor viaja en la query string y en la
// exportación, así que va en inglés y no cambia.
type HealthSection string

const (
	SectionUnsold      HealthSection = "unsold_deduction"
	SectionNegative    HealthSection = "negative_stock"
	SectionNoPurchases HealthSection = "no_purchases"
	SectionMalformed   HealthSection = "malformed_recipe"
	SectionPending     HealthSection = "pending_recipe"
)

// HealthPageSize es lo que la pantalla pide por sección: lo mismo que «Ver 25 más» de Recetas.
const HealthPageSize = 25

// MaxHealthPage acota una página pedida a mano.
const MaxHealthPage = 100

// maxHealthRangeDays: un año y un día. Más que eso es un anti-join sobre todo el histórico de ventas,
// y la pantalla no ofrece un periodo así.
const maxHealthRangeDays = 366

// HealthFilter es lo que la pantalla pide. Sin sección trae la primera página de todas.
type HealthFilter struct {
	Range Range
	// NoPurchasesSince: un insumo cuenta como «sin compras» si no tiene ninguna desde esta fecha.
	// Nil = nunca ha tenido una.
	NoPurchasesSince *time.Time
	Section          HealthSection
	Limit            int32
	Offset           int32
}

// DefaultHealthRange son los últimos 30 días contando hoy. Es el periodo cuando la pantalla no
// manda fechas, nunca cuando las manda mal (eso se rechaza).
func DefaultHealthRange(today time.Time) Range {
	return Range{From: today.AddDate(0, 0, -29), To: today}
}

// Validate rechaza lo que haría que la pantalla contestara otra cosa que la pedida.
func (f HealthFilter) Validate() error {
	if f.Range.From.IsZero() || f.Range.To.IsZero() {
		return fmt.Errorf("%w: el periodo necesita fecha de inicio y de fin", ErrValidation)
	}
	if f.Range.To.Before(f.Range.From) {
		return fmt.Errorf("%w: el periodo termina antes de empezar", ErrValidation)
	}
	// Días contados con los dos extremos: del 13-oct-2025 al 13-oct-2026 son 366.
	if f.Range.To.Sub(f.Range.From) >= maxHealthRangeDays*24*time.Hour {
		return fmt.Errorf("%w: el periodo no puede pasar de %d días", ErrValidation, maxHealthRangeDays)
	}
	if f.NoPurchasesSince != nil && f.NoPurchasesSince.IsZero() {
		return fmt.Errorf("%w: la fecha de «sin compras desde» está vacía", ErrValidation)
	}
	switch f.Section {
	case "", SectionUnsold, SectionNegative, SectionNoPurchases, SectionMalformed, SectionPending:
	default:
		return fmt.Errorf("%w: sección desconocida (%q)", ErrValidation, f.Section)
	}
	if f.Limit < 1 || f.Limit > MaxHealthPage {
		return fmt.Errorf("%w: el tamaño de página va de 1 a %d", ErrValidation, MaxHealthPage)
	}
	if f.Offset < 0 {
		return fmt.Errorf("%w: la página no puede ser negativa", ErrValidation)
	}
	return nil
}
