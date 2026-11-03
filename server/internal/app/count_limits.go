package app

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// CountLimits devuelve los límites del conteo de la empresa de la sesión. Sin fila (un respaldo
// anterior a la 0098 o una empresa dada de alta a mano) caen los defaults, sin escribir: la lectura
// la hace quien cuenta, que no puede configurar.
func (s *BackofficeService) CountLimits(ctx context.Context) (domain.CountLimits, error) {
	return countLimits(ctx, s.store.QC(ctx))
}

func countLimits(ctx context.Context, q *db.Queries) (domain.CountLimits, error) {
	r, err := q.GetCountLimits(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DefaultCountLimits(), nil
	}
	if err != nil {
		return domain.CountLimits{}, err
	}
	return domain.CountLimits{
		Mass:   domain.CountLimitPair{Max: r.MaxMass, Confirm: r.ConfirmMass},
		Volume: domain.CountLimitPair{Max: r.MaxVolume, Confirm: r.ConfirmVolume},
		Length: domain.CountLimitPair{Max: r.MaxLength, Confirm: r.ConfirmLength},
		Piece:  domain.CountLimitPair{Max: r.MaxPiece, Confirm: r.ConfirmPiece},
	}, nil
}

// SetCountLimits guarda los límites del conteo y deja el evento con antes y después en la misma
// transacción. Bajar el máximo no toca lo ya capturado: aplica a la siguiente captura.
func (s *BackofficeService) SetCountLimits(ctx context.Context, l domain.CountLimits, actor int64) error {
	for _, p := range []*domain.CountLimitPair{&l.Mass, &l.Volume, &l.Length, &l.Piece} {
		p.Max, p.Confirm = domain.Round4(p.Max), domain.Round4(p.Confirm)
	}
	if err := l.Validate(); err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		before, err := countLimits(ctx, q)
		if err != nil {
			return err
		}
		if err := q.UpsertCountLimits(ctx, db.UpsertCountLimitsParams{
			MaxMass: l.Mass.Max, MaxVolume: l.Volume.Max, MaxLength: l.Length.Max, MaxPiece: l.Piece.Max,
			ConfirmMass: l.Mass.Confirm, ConfirmVolume: l.Volume.Confirm, ConfirmLength: l.Length.Confirm, ConfirmPiece: l.Piece.Confirm,
		}); err != nil {
			return err
		}
		return auditEntity(ctx, q, domain.AuditSettingsCountLimits, "count_limits", 0, nil, &actor, before, l)
	})
}
