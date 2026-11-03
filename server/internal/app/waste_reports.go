package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// LossItem es el faltante de un artículo en un conteo cerrado (spec 039, historia 2).
type LossItem struct {
	IngredientID int64            `json:"ingredientId"`
	Name         string           `json:"name"`
	UnitCode     string           `json:"unitCode"`
	HasPrevious  bool             `json:"hasPrevious"`
	Registered   decimal.Decimal  `json:"registered"`
	Unexplained  decimal.Decimal  `json:"unexplained"`
	Surplus      decimal.Decimal  `json:"surplus"`
	Lost         decimal.Decimal  `json:"lost"`
	UnitCost     *decimal.Decimal `json:"unitCost"`
	LostCost     *decimal.Decimal `json:"lostCost"`
	UnexplCost   *decimal.Decimal `json:"unexplainedCost"`
}

// LossTotals suma en pesos lo que tiene costo; lo que no, se cuenta aparte.
type LossTotals struct {
	Lost        decimal.Decimal `json:"lost"`
	Registered  decimal.Decimal `json:"registered"`
	Unexplained decimal.Decimal `json:"unexplained"`
	WithoutCost int             `json:"withoutCost"`
}

// CountLossView es el faltante de un conteo cerrado. `FirstCount`: ningún artículo tenía un conteo
// anterior comparable, así que no hay periodo y no se inventan cifras.
type CountLossView struct {
	FirstCount bool       `json:"firstCount"`
	Items      []LossItem `json:"items"`
	Totals     LossTotals `json:"totals"`
}

// CountLoss parte lo perdido de cada artículo contado entre lo registrado como merma y lo que el
// conteo encontró de menos. Los pesos usan el costo guardado al cerrar, así que el reporte de un
// conteo cerrado no cambia cuando cambia un costo.
func (s *BackofficeService) CountLoss(ctx context.Context, countID int64) (CountLossView, error) {
	rows, err := s.store.QC(ctx).CountLossLines(ctx, countID)
	if err != nil {
		return CountLossView{}, err
	}
	out := CountLossView{FirstCount: true, Items: make([]LossItem, 0, len(rows))}
	for _, r := range rows {
		it := LossItem{IngredientID: r.IngredientID, Name: r.Name, UnitCode: r.BaseUnitCode, HasPrevious: r.HasPrevious}
		registered := decimal.Zero
		if r.HasPrevious {
			out.FirstCount = false
			registered = r.Registered
		}
		split := domain.SplitLoss(registered, r.SystemQty, r.CountedQty)
		it.Registered, it.Unexplained, it.Surplus, it.Lost = split.Registered, split.Unexplained, split.Surplus, split.Lost
		if r.UnitCost != nil && r.UnitCost.IsPositive() {
			uc := *r.UnitCost
			lost, unexpl := domain.Round2(split.Lost.Mul(uc)), domain.Round2(split.Unexplained.Mul(uc))
			it.UnitCost, it.LostCost, it.UnexplCost = &uc, &lost, &unexpl
			if r.HasPrevious {
				out.Totals.Lost = out.Totals.Lost.Add(lost)
				out.Totals.Registered = out.Totals.Registered.Add(domain.Round2(split.Registered.Mul(uc)))
			}
			out.Totals.Unexplained = out.Totals.Unexplained.Add(unexpl)
		} else if split.Lost.IsPositive() {
			out.Totals.WithoutCost++
		}
		out.Items = append(out.Items, it)
	}
	return out, nil
}

// coverPendingWaste cierra como «la cubrió el conteo» las mermas por aprobar de la sucursal que el
// conteo vio: registradas antes de que capturara alguno de los artículos que restarían. El conteo
// ya reflejó esa pérdida; aprobarlas después la restaría dos veces. Una registrada después de la
// captura no la vio y sigue por aprobar (revisión de arquitectura).
func coverPendingWaste(ctx context.Context, q *db.Queries, countID, branch int64) error {
	pending, err := q.PendingWasteToCover(ctx, branch)
	if err != nil || len(pending) == 0 {
		return err
	}
	lines, err := q.CountedLinesSince(ctx, countID)
	if err != nil {
		return err
	}
	captured := make(map[int64]time.Time, len(lines))
	for _, l := range lines {
		captured[l.IngredientID] = l.CapturedAt
	}
	var graph *domain.StockGraph
	for _, p := range pending {
		var ings []int64
		if p.IngredientID != nil {
			ings = []int64{*p.IngredientID}
		} else if p.ProductID != nil {
			if graph == nil {
				g, err := loadStockGraph(ctx, q)
				if err != nil {
					return err
				}
				graph = &g
			}
			deltas, _ := graph.ExpandSale(domain.SaleLine{ProductID: *p.ProductID, Qty: p.Quantity})
			for _, d := range deltas {
				if d.IngredientID != nil {
					ings = append(ings, *d.IngredientID)
				}
			}
		}
		seen := false
		for _, id := range ings {
			if at, ok := captured[id]; ok && !p.RecordedAt.After(at) {
				seen = true
				break
			}
		}
		if !seen {
			continue
		}
		cid := countID
		if _, err := q.ResolveWasteRecord(ctx, db.ResolveWasteRecordParams{
			Status: string(domain.WasteCovered), CoveredByCountID: &cid, ID: p.ID, FromStatus: string(domain.WastePending),
		}); err != nil {
			return err
		}
	}
	return nil
}

// WasteReasonAdmin es un motivo para administrarlo: también los archivados y los juntados.
type WasteReasonAdmin struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	IsStaffMeal  bool   `json:"isStaffMeal"`
	Archived     bool   `json:"archived"`
	MergedIntoID *int64 `json:"mergedIntoId"`
	Uses         int32  `json:"uses"`
}

// AllWasteReasons lista todos los motivos de la empresa, para administrarlos.
func (s *BackofficeService) AllWasteReasons(ctx context.Context) ([]WasteReasonAdmin, error) {
	rows, err := s.store.QC(ctx).ListAllWasteReasons(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]WasteReasonAdmin, 0, len(rows))
	for _, r := range rows {
		out = append(out, WasteReasonAdmin{ID: r.ID, Name: r.Name, Description: r.Description, IsStaffMeal: r.IsStaffMeal,
			Archived: r.ArchivedAt.Valid, MergedIntoID: r.MergedIntoID, Uses: r.Uses})
	}
	return out, nil
}

// WasteReasonUpdate es lo editable de un motivo.
type WasteReasonUpdate struct {
	Name        string
	Description string
	IsStaffMeal bool
	Archived    bool
}

// UpdateWasteReason renombra, describe, marca «no cuenta como pérdida» o archiva un motivo. Lo ya
// registrado no cambia: cada registro copió la marca al registrarse.
func (s *BackofficeService) UpdateWasteReason(ctx context.Context, id int64, u WasteReasonUpdate, actor int64) error {
	if err := domain.ValidWasteReason(u.Name, u.Description); err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		n, err := q.UpdateWasteReason(ctx, db.UpdateWasteReasonParams{
			Name: strings.TrimSpace(u.Name), Description: strings.TrimSpace(u.Description), IsStaffMeal: u.IsStaffMeal,
			Archived: u.Archived, ID: id,
		})
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("%w: ya hay un motivo con ese nombre", domain.ErrConflict)
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%w: ese motivo no existe o ya se juntó con otro", domain.ErrNotFound)
		}
		return nil
	})
}

// MergeWasteReason junta un motivo con otro: deja de ofrecerse y el reporte suma sus registros bajo
// el que queda. Los registros NO cambian de motivo (revisión de arquitectura): reescribirlos perdería
// para siempre con qué motivo se capturó cada uno. Un solo nivel: el destino no puede estar juntado
// ni archivado.
func (s *BackofficeService) MergeWasteReason(ctx context.Context, id, into, actor int64) error {
	if id == into {
		return fmt.Errorf("%w: un motivo no se junta consigo mismo", domain.ErrValidation)
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockWasteReasonsForMerge(ctx, []int64{id, into}); err != nil {
			return err
		}
		if _, err := q.GetWasteReason(ctx, id); err != nil {
			return notFound(err)
		}
		target, err := q.GetWasteReason(ctx, into)
		if err != nil {
			return notFound(err)
		}
		if target.ArchivedAt.Valid || target.MergedIntoID != nil {
			return fmt.Errorf("%w: se junta con un motivo que se sigue usando", domain.ErrValidation)
		}
		n, err := q.MergeWasteReason(ctx, db.MergeWasteReasonParams{IntoID: &into, ID: id})
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%w: ese motivo ya se juntó con otro", domain.ErrConflict)
		}
		return q.MoveMergedWasteReasons(ctx, db.MoveMergedWasteReasonsParams{IntoID: &into, ID: &id})
	})
}

// wastePeriod convierte dos fechas del local en el intervalo [desde, hasta+1 día).
func (s *BackofficeService) wastePeriod(ctx context.Context, from, to time.Time) (time.Time, time.Time, error) {
	if to.Before(from) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: la fecha final va después de la inicial", domain.ErrValidation)
	}
	loc := s.location(ctx)
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	return start, end, nil
}

// ListWaste lista los registros de un periodo, opcionalmente de un motivo (siguiendo los juntados).
func (s *BackofficeService) ListWaste(ctx context.Context, from, to time.Time, reasonID *int64) ([]WasteRow, error) {
	start, end, err := s.wastePeriod(ctx, from, to)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.QC(ctx).ListWaste(ctx, db.ListWasteParams{FromTs: start, ToTs: end, ReasonID: reasonID})
	if err != nil {
		return nil, err
	}
	out := make([]WasteRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, WasteRow{ID: r.ID, ItemType: string(r.ItemType), ItemName: r.ItemName, Quantity: r.Quantity,
			UnitCode: r.UnitCode, TotalCost: r.TotalCost, Status: r.Status, Reason: r.ReasonName, IsStaffMeal: r.IsStaffMeal,
			ReportReason: r.ReportReasonID, RecordedBy: r.RecordedByName, RecordedAt: r.RecordedAt})
	}
	return out, nil
}

// WasteReasonTotal es lo que restó un motivo en el periodo.
type WasteReasonTotal struct {
	ReasonID    int64           `json:"reasonId"`
	Name        string          `json:"name"`
	IsStaffMeal bool            `json:"isStaffMeal"`
	Records     int32           `json:"records"`
	TotalCost   decimal.Decimal `json:"totalCost"`
	WithoutCost int32           `json:"withoutCost"`
}

// WasteByReason suma lo que restó en el periodo por motivo (el que queda tras juntar), con la
// comida del personal aparte por la marca que se copió al registrar.
func (s *BackofficeService) WasteByReason(ctx context.Context, from, to time.Time) ([]WasteReasonTotal, error) {
	start, end, err := s.wastePeriod(ctx, from, to)
	if err != nil {
		return nil, err
	}
	q := s.store.QC(ctx)
	rows, err := q.WasteByReason(ctx, db.WasteByReasonParams{FromTs: start, ToTs: end})
	if err != nil {
		return nil, err
	}
	reasons, err := q.ListAllWasteReasons(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[int64]string, len(reasons))
	for _, r := range reasons {
		names[r.ID] = r.Name
	}
	out := make([]WasteReasonTotal, 0, len(rows))
	for _, r := range rows {
		out = append(out, WasteReasonTotal{ReasonID: r.ReasonID, Name: names[r.ReasonID], IsStaffMeal: r.IsStaffMeal,
			Records: r.Records, TotalCost: r.TotalCost, WithoutCost: r.WithoutCost})
	}
	return out, nil
}
