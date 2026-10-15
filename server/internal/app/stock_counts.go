package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Conteo de existencias (spec 038, historia 3). Cada cifra se guarda al capturarla junto con lo
// que el sistema tenía en ese momento, y al cerrar se ajusta por la diferencia (domain.PlanCountClose):
// las ventas no se detienen mientras se cuenta. Lo no contado queda como estaba.

// CountPresentation es un empaque del insumo, para ayudar a contar «3 cajas + 2 L».
type CountPresentation struct {
	Name      string          `json:"name"`
	QtyInBase decimal.Decimal `json:"qtyInBase"`
}

// CountItem es un insumo por contar. Sin lo que el sistema tiene: el conteo es a ciegas.
type CountItem struct {
	IngredientID  int64               `json:"ingredientId"`
	Name          string              `json:"name"`
	Place         *string             `json:"place"`
	BaseUnitCode  string              `json:"baseUnitCode"`
	Presentations []CountPresentation `json:"presentations"`
	CountedQty    *decimal.Decimal    `json:"countedQty"`
}

// CountView es el conteo abierto con lo que hay que contar.
type CountView struct {
	ID       int64       `json:"id"`
	OpenedAt time.Time   `json:"openedAt"`
	Items    []CountItem `json:"items"`
}

// CountAdjustmentView es un renglón contado al cerrar: lo que el sistema tenía, lo contado y la
// diferencia, en la unidad base.
type CountAdjustmentView struct {
	IngredientID int64           `json:"ingredientId"`
	Name         string          `json:"name"`
	BaseUnitCode string          `json:"baseUnitCode"`
	System       decimal.Decimal `json:"system"`
	Counted      decimal.Decimal `json:"counted"`
	Delta        decimal.Decimal `json:"delta"`
}

// NotCountedView es un insumo que no se contó y por eso queda como estaba.
type NotCountedView struct {
	IngredientID int64  `json:"ingredientId"`
	Name         string `json:"name"`
}

// CountCloseView es lo que hace (o haría) cerrar el conteo. Siempre arreglos, aunque vacíos.
type CountCloseView struct {
	Adjusted   []CountAdjustmentView `json:"adjusted"`
	NotCounted []NotCountedView      `json:"notCounted"`
}

// OpenCount abre el conteo de la sucursal de la sesión. Con uno abierto, 409.
func (s *BackofficeService) OpenCount(ctx context.Context, userID int64) (CountView, error) {
	var opened db.OpenStockCountRow
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		var err error
		opened, err = q.OpenStockCount(ctx, userID)
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: ya hay un conteo abierto en esta sucursal", domain.ErrConflict)
		}
		return err
	})
	if err != nil {
		return CountView{}, err
	}
	v, err := s.countView(ctx, opened.ID, opened.OpenedAt)
	return v, err
}

// CurrentCount devuelve el conteo abierto de la sucursal, o nil si no hay.
func (s *BackofficeService) CurrentCount(ctx context.Context) (*CountView, error) {
	cur, err := s.store.QC(ctx).CurrentStockCount(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v, err := s.countView(ctx, cur.ID, cur.OpenedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *BackofficeService) countView(ctx context.Context, id int64, openedAt time.Time) (CountView, error) {
	q := s.store.QC(ctx)
	rows, err := q.CountItems(ctx, id)
	if err != nil {
		return CountView{}, err
	}
	pres, err := q.ListPresentations(ctx, nil)
	if err != nil {
		return CountView{}, err
	}
	byIng := map[int64][]CountPresentation{}
	for _, p := range pres {
		if p.Active && !p.IsApproximate {
			byIng[p.IngredientID] = append(byIng[p.IngredientID], CountPresentation{Name: p.Name, QtyInBase: p.QtyInBase})
		}
	}
	v := CountView{ID: id, OpenedAt: openedAt, Items: make([]CountItem, len(rows))}
	for i, r := range rows {
		ps := byIng[r.ID]
		if ps == nil {
			ps = []CountPresentation{}
		}
		v.Items[i] = CountItem{IngredientID: r.ID, Name: r.Name, Place: r.CountPlace, BaseUnitCode: r.BaseUnitCode,
			Presentations: ps, CountedQty: r.CountedQty}
	}
	return v, nil
}

// openCount bloquea el conteo y exige que siga abierto.
func openCount(ctx context.Context, q *db.Queries, id int64) (db.LockStockCountRow, error) {
	c, err := q.LockStockCount(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, domain.ErrNotFound
	}
	if err != nil {
		return c, err
	}
	if c.Status != "open" {
		return c, fmt.Errorf("%w: el conteo ya se cerró", domain.ErrConflict)
	}
	return c, nil
}

// CaptureCount guarda una cifra contada (en unidad base) con lo que el sistema tiene ahora en la
// sucursal del conteo. Recapturar reemplaza las dos.
func (s *BackofficeService) CaptureCount(ctx context.Context, countID, ingredientID int64, counted decimal.Decimal, userID int64) error {
	counted = domain.Round4(counted)
	if !domain.ValidStockQtyOrZero(counted) {
		return domain.ErrValidation
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		c, err := openCount(ctx, q, countID)
		if err != nil {
			return err
		}
		system, err := q.OnHandInBranch(ctx, db.OnHandInBranchParams{IngredientID: &ingredientID, BranchID: c.BranchID})
		if err != nil {
			return err
		}
		err = q.UpsertCountLine(ctx, db.UpsertCountLineParams{
			CountID: countID, IngredientID: ingredientID, CountedQty: counted, SystemQty: system, CapturedBy: userID,
		})
		if isForeignKeyViolation(err) {
			return domain.ErrNotFound // un insumo de otra empresa o que no existe
		}
		return err
	})
}

// UncaptureCount quita una cifra: el insumo vuelve a «no contado».
func (s *BackofficeService) UncaptureCount(ctx context.Context, countID, ingredientID int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if _, err := openCount(ctx, q, countID); err != nil {
			return err
		}
		_, err := q.DeleteCountLine(ctx, db.DeleteCountLineParams{CountID: countID, IngredientID: ingredientID})
		return err
	})
}

func (s *BackofficeService) planClose(ctx context.Context, q *db.Queries, countID int64) (CountCloseView, []domain.CountAdjustment, error) {
	lines, err := q.CountLines(ctx, countID)
	if err != nil {
		return CountCloseView{}, nil, err
	}
	in := make([]domain.CountLine, len(lines))
	byID := make(map[int64]db.CountLinesRow, len(lines))
	for i, l := range lines {
		in[i] = domain.CountLine{IngredientID: l.IngredientID, Counted: l.CountedQty, System: l.SystemQty}
		byID[l.IngredientID] = l
	}
	plan := domain.PlanCountClose(in)
	out := CountCloseView{Adjusted: make([]CountAdjustmentView, 0, len(plan)), NotCounted: []NotCountedView{}}
	for _, a := range plan {
		l := byID[a.IngredientID]
		out.Adjusted = append(out.Adjusted, CountAdjustmentView{IngredientID: a.IngredientID, Name: l.Name,
			BaseUnitCode: l.BaseUnitCode, System: a.System, Counted: a.Counted, Delta: a.Delta})
	}
	nc, err := q.NotCountedIngredients(ctx, countID)
	if err != nil {
		return CountCloseView{}, nil, err
	}
	for _, n := range nc {
		out.NotCounted = append(out.NotCounted, NotCountedView{IngredientID: n.ID, Name: n.Name})
	}
	return out, plan, nil
}

// PreviewCount es lo que haría cerrar, sin escribir nada.
func (s *BackofficeService) PreviewCount(ctx context.Context, countID int64) (CountCloseView, error) {
	var out CountCloseView
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		if _, err := openCount(ctx, q, countID); err != nil {
			return err
		}
		if err := q.MarkCountPreviewed(ctx, countID); err != nil {
			return err
		}
		var err error
		out, _, err = s.planClose(ctx, q, countID)
		return err
	})
	return out, err
}

// CloseCount inserta un ajuste por cada cifra con diferencia, en orden de insumo y ligado al
// conteo, y lo deja cerrado. Lo no contado queda como estaba.
func (s *BackofficeService) CloseCount(ctx context.Context, countID, userID int64) (CountCloseView, error) {
	var out CountCloseView
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		c, err := openCount(ctx, q, countID)
		if err != nil {
			return err
		}
		var plan []domain.CountAdjustment
		if out, plan, err = s.planClose(ctx, q, countID); err != nil {
			return err
		}
		ids := make([]int64, len(plan))
		for i, a := range plan {
			ids[i] = a.IngredientID
		}
		if len(ids) > 0 {
			if _, err := q.LockStockLevelsInBranchNowait(ctx, db.LockStockLevelsInBranchNowaitParams{Ids: ids, BranchID: c.BranchID}); err != nil {
				return lockErr(err)
			}
		}
		reason := "conteo"
		for _, a := range plan {
			if err := q.InsertCountMovement(ctx, db.InsertCountMovementParams{
				IngredientID: &a.IngredientID, Quantity: a.Delta, StockCountID: &countID, UserID: &userID,
				Reason: &reason, BranchID: c.BranchID,
			}); err != nil {
				return err
			}
		}
		if _, err := q.FinishStockCount(ctx, db.FinishStockCountParams{ID: countID, Status: "closed", ClosedBy: &userID}); err != nil {
			return err
		}
		// La vista previa dice cuánto sobraba o faltaba (decisión del dueño); con eso se puede volver a
		// capturar lo que el sistema tenía. No se esconde: queda dicho cuántas cifras cambiaron después.
		changed, err := q.CountChangedAfterPreview(ctx, countID)
		if err != nil {
			return err
		}
		branch := c.BranchID
		return auditEntity(ctx, q, domain.AuditStockCounted, "stock_count", countID, &branch, &userID, nil,
			map[string]any{"adjusted": len(out.Adjusted), "notCounted": len(out.NotCounted), "changedAfterPreview": changed})
	})
	return out, err
}

// DiscardCount descarta el conteo sin tocar el almacén.
func (s *BackofficeService) DiscardCount(ctx context.Context, countID, userID int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if _, err := openCount(ctx, q, countID); err != nil {
			return err
		}
		_, err := q.FinishStockCount(ctx, db.FinishStockCountParams{ID: countID, Status: "discarded", ClosedBy: &userID})
		return err
	})
}

// LastClosedCount es cuándo se cerró el último conteo de la sucursal, o nil si nunca.
func (s *BackofficeService) LastClosedCount(ctx context.Context) (*time.Time, error) {
	at, err := s.store.QC(ctx).LastClosedStockCount(ctx)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !at.Valid) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &at.Time, nil
}

// SetCountPlace fija el lugar del insumo que ordena el conteo («Refri»). No es un almacén: es en
// qué orden se camina al contar. nil o vacío lo quita.
func (s *BackofficeService) SetCountPlace(ctx context.Context, ingredientID int64, place *string) error {
	if place != nil {
		p := strings.TrimSpace(*place)
		if p == "" {
			place = nil
		} else if utf8.RuneCountInString(p) > 30 {
			return fmt.Errorf("%w: el lugar va de 1 a 30 letras", domain.ErrValidation)
		} else {
			place = &p
		}
	}
	n, err := s.store.QC(ctx).SetCountPlace(ctx, db.SetCountPlaceParams{ID: ingredientID, Place: place})
	if err == nil && n == 0 {
		return domain.ErrNotFound
	}
	return err
}
