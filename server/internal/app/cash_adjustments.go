package app

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// CashAdjustmentService corrige un corte cerrado sin reescribir lo firmado (#290, spec 082 fase 3):
// cada corrección se agrega con antes, después, quién, cuándo y por qué, y se deshace con un reverso.
type CashAdjustmentService struct{ cashDeps }

// SessionAdjustmentView es una corrección como la ve el detalle del corte.
type SessionAdjustmentView struct {
	ID            int64            `json:"id"`
	Kind          string           `json:"kind"`
	Before        *decimal.Decimal `json:"before"`
	After         *decimal.Decimal `json:"after"`
	Reason        string           `json:"reason"`
	ReasonNote    string           `json:"reasonNote"`
	CreatedByName string           `json:"createdByName"`
	CreatedAt     time.Time        `json:"createdAt"`
	ReversesID    *int64           `json:"reversesId"`
	// ReversedBy: la corrección que la revirtió, si la hay.
	ReversedBy *int64 `json:"reversedBy"`
}

// EffectiveView es lo que rige después de las correcciones vigentes: el contado del cajón y el fondo
// que se dejó. Nil en cada uno = rige lo firmado.
type EffectiveView struct {
	DrawerCounted *decimal.Decimal `json:"drawerCounted"`
	FloatLeft     *decimal.Decimal `json:"floatLeft"`
}

// AdjustSession registra una corrección de conteo, de fondo o una nota sobre un corte cerrado.
func (s *CashAdjustmentService) AdjustSession(ctx context.Context, sessionID, userID int64, in domain.AdjustmentInput) (*SessionAdjustmentView, error) {
	var out *SessionAdjustmentView
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		sess, err := q.LockSessionForAdjustment(ctx, sessionID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := domain.CanAdjustSession(string(sess.Status)); err != nil {
			return err
		}
		adjs, err := q.ListSessionAdjustments(ctx, sessionID)
		if err != nil {
			return err
		}
		var current *decimal.Decimal
		switch in.Kind {
		case domain.AdjustmentCountCorrection:
			current, err = signedDrawerCount(ctx, q, sessionID)
			if err != nil {
				return err
			}
		case domain.AdjustmentFloatCorrection:
			current = sess.FloatLeft
		}
		if v := domain.EffectiveAdjustment(adjustmentsOf(adjs), in.Kind); v != nil && v.After != nil {
			current = v.After
		}
		valid, err := domain.ValidateAdjustment(in, current)
		if err != nil {
			return err
		}
		var before *decimal.Decimal
		if valid.Kind != domain.AdjustmentNote {
			before = current
		}
		row, err := q.InsertSessionAdjustment(ctx, db.InsertSessionAdjustmentParams{
			SessionID: sessionID, Kind: valid.Kind, BeforeAmount: before, AfterAmount: valid.After,
			Reason: valid.Reason, Note: valid.Note, CreatedBy: userID,
		})
		if err != nil {
			return err
		}
		out = &SessionAdjustmentView{ID: row.ID, Kind: valid.Kind, Before: before, After: valid.After,
			Reason: valid.Reason, ReasonNote: valid.Note, CreatedAt: row.CreatedAt}
		return auditCash(ctx, q, "cash.session_adjusted", sessionID, &userID, map[string]any{"kind": valid.Kind, "before": before},
			map[string]any{"kind": valid.Kind, "after": valid.After, "reason": valid.Reason, "note": valid.Note})
	})
	return out, err
}

// ReverseAdjustment deshace una corrección con otra que la nombra. Una corrección se revierte una sola
// vez, y solo desde su corte.
func (s *CashAdjustmentService) ReverseAdjustment(ctx context.Context, sessionID, adjustmentID, userID int64, note string) (*SessionAdjustmentView, error) {
	var out *SessionAdjustmentView
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		if _, err := q.LockSessionForAdjustment(ctx, sessionID); errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		} else if err != nil {
			return err
		}
		adjs, err := q.ListSessionAdjustments(ctx, sessionID)
		if err != nil {
			return err
		}
		var target *db.ListSessionAdjustmentsRow
		reverted := false
		for i := range adjs {
			if adjs[i].ID == adjustmentID {
				target = &adjs[i]
			}
			if adjs[i].ReversesID != nil && *adjs[i].ReversesID == adjustmentID {
				reverted = true
			}
		}
		if target == nil {
			return domain.ErrNotFound
		}
		if err := domain.CanReverseAdjustment(target.Kind, reverted); err != nil {
			return err
		}
		valid, err := domain.ValidateAdjustment(domain.AdjustmentInput{Kind: domain.AdjustmentNote, Reason: "other", Note: note}, nil)
		if err != nil {
			return err
		}
		row, err := q.InsertSessionAdjustment(ctx, db.InsertSessionAdjustmentParams{
			SessionID: sessionID, Kind: domain.AdjustmentReversal, Reason: "other", Note: valid.Note,
			ReversesID: &adjustmentID, CreatedBy: userID,
		})
		if isUniqueViolation(err) {
			// Otra tableta revirtió la misma corrección entre la lectura y el insert.
			return domain.ErrAdjustmentReversed
		}
		if err != nil {
			return err
		}
		out = &SessionAdjustmentView{ID: row.ID, Kind: domain.AdjustmentReversal, Reason: "other",
			ReasonNote: valid.Note, CreatedAt: row.CreatedAt, ReversesID: &adjustmentID}
		return auditCash(ctx, q, "cash.session_adjustment_reversed", sessionID, &userID,
			map[string]any{"adjustmentId": adjustmentID}, map[string]any{"reversalId": row.ID, "note": valid.Note})
	})
	return out, err
}

// signedDrawerCount es lo contado al cerrar, o nil si el corte no tiene arqueo con esperado (antes de
// la 0067 no hay arqueo firmado que corregir).
func signedDrawerCount(ctx context.Context, q *db.Queries, sessionID int64) (*decimal.Decimal, error) {
	c, err := q.GetCashCount(ctx, db.GetCashCountParams{SessionID: sessionID, Moment: "cierre"})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if c.Expected == nil {
		return nil, nil
	}
	return &c.Total, nil
}

func adjustmentsOf(rows []db.ListSessionAdjustmentsRow) []domain.Adjustment {
	out := make([]domain.Adjustment, len(rows))
	for i, r := range rows {
		out[i] = domain.Adjustment{ID: r.ID, Kind: r.Kind, After: r.AfterAmount, ReversesID: r.ReversesID}
	}
	return out
}

// adjustmentViews arma la lista del detalle, con quién revirtió cada una.
func adjustmentViews(rows []db.ListSessionAdjustmentsRow) []SessionAdjustmentView {
	revertidaPor := map[int64]int64{}
	for _, r := range rows {
		if r.ReversesID != nil {
			revertidaPor[*r.ReversesID] = r.ID
		}
	}
	out := make([]SessionAdjustmentView, len(rows))
	for i, r := range rows {
		out[i] = SessionAdjustmentView{ID: r.ID, Kind: r.Kind, Before: r.BeforeAmount, After: r.AfterAmount,
			Reason: r.Reason, ReasonNote: r.Note, CreatedByName: r.CreatedByName, CreatedAt: r.CreatedAt, ReversesID: r.ReversesID}
		if by, ok := revertidaPor[r.ID]; ok {
			out[i].ReversedBy = &by
		}
	}
	return out
}

// effectiveOf es lo que rige tras las correcciones vigentes; nil si no hay ninguna.
func effectiveOf(adjs []domain.Adjustment) *EffectiveView {
	var e EffectiveView
	if v := domain.EffectiveAdjustment(adjs, domain.AdjustmentCountCorrection); v != nil {
		e.DrawerCounted = v.After
	}
	if v := domain.EffectiveAdjustment(adjs, domain.AdjustmentFloatCorrection); v != nil {
		e.FloatLeft = v.After
	}
	if e.DrawerCounted == nil && e.FloatLeft == nil {
		return nil
	}
	return &e
}

// withCorrectedCount devuelve el arqueo con el contado corregido, para calcular la diferencia
// corregida sin tocar el arqueo firmado que la vista sigue mostrando.
func withCorrectedCount(drawer *ArqueoDelCajonView, eff *EffectiveView) *ArqueoDelCajonView {
	if drawer == nil || eff == nil || eff.DrawerCounted == nil {
		return drawer
	}
	c := *drawer
	c.Counted = eff.DrawerCounted
	return &c
}
