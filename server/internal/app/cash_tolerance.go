package app

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// CloseExplanation es la explicación de la diferencia que deja quien cierra viendo el esperado
// (#290, fase 4). Se guarda como una nota del corte en la misma transacción del cierre.
type CloseExplanation struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

// CashTolerance es la tolerancia de diferencia de la empresa de la sesión.
func (s *CashReportService) CashTolerance(ctx context.Context) (decimal.Decimal, error) {
	return s.store.QC(ctx).GetCashTolerance(ctx)
}

// SetCashTolerance guarda la tolerancia. Vacía, negativa o absurda es ErrValidation: ausente no es $0.
func (s *CashReportService) SetCashTolerance(ctx context.Context, v *decimal.Decimal) error {
	t, err := domain.ValidTolerance(v)
	if err != nil {
		return err
	}
	n, err := s.store.QC(ctx).SetCashTolerance(ctx, t)
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// needsExplanation calcula la marca «por explicar» de un corte con la tolerancia de hoy. Nil para
// quien no configura la caja: la marca dice que hubo diferencia.
func needsExplanation(canSee bool, diff *domain.CutDiff, tolerance decimal.Decimal, adjs []domain.Adjustment, next *NextOpeningView) *bool {
	if !canSee || diff == nil {
		return nil
	}
	nota := false
	revertida := map[int64]bool{}
	for _, a := range adjs {
		if a.ReversesID != nil {
			revertida[*a.ReversesID] = true
		}
	}
	for _, a := range adjs {
		if a.Kind == domain.AdjustmentNote && !revertida[a.ID] {
			nota = true
		}
	}
	v := domain.CutNeedsExplanation(diff.Total, tolerance, nota, next != nil)
	return &v
}
