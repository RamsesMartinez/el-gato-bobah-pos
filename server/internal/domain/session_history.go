package domain

import (
	"fmt"
	"time"
)

// SessionHistoryFilter es una página del histórico de cortes, opcionalmente acotada por el día del
// turno. Sin paginar solo se veían los 50 más recientes y un corte más viejo no se podía abrir desde
// la pantalla, que es justo donde se audita.
type SessionHistoryFilter struct {
	From, To *time.Time
	Limit    int32
	Offset   int32
}

// Validate rechaza un rango al revés y una página fuera de tope en vez de caer a un default.
func (f SessionHistoryFilter) Validate() error {
	if f.From != nil && f.To != nil && f.From.After(*f.To) {
		return fmt.Errorf("%w: la fecha de inicio va después de la final", ErrValidation)
	}
	if f.Limit < 1 || f.Limit > MaxSalesPageSize {
		return fmt.Errorf("%w: el tamaño de página va de 1 a %d", ErrValidation, MaxSalesPageSize)
	}
	if f.Offset < 0 {
		return fmt.Errorf("%w: la página no puede ser negativa", ErrValidation)
	}
	return nil
}
