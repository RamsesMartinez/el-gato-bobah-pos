package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// La configuración de alertas de stock bajo de un artículo (spec 084, historia 3). Guardar dice
// primero qué va a pasar (la confirmación de la pantalla): el efecto se calcula con la misma tabla de
// verdad que aplica la base (domain.LowStockEffect ↔ evaluate_low_stock).

// LowStockInput es la sección «Alertas» de la ficha. MinStock nulo o cero = sin mínimo.
type LowStockInput struct {
	Enabled, OnMin, OnOut, Banner bool
	MinStock                      *decimal.Decimal
	CoverageDays                  *int
	// BranchMinStock: el mínimo de la sucursal actual. Sin Set no se toca; con Set y nil se quita.
	BranchMinStock    Field[decimal.Decimal]
	ExpectedUpdatedAt *time.Time
}

// LowStockResult es lo que pasa (o pasaría) al guardar.
type LowStockResult struct {
	Effect     domain.LowStockChange `json:"effect"`
	OnHand     decimal.Decimal       `json:"onHand"`
	Min        *decimal.Decimal      `json:"min"`
	NeedsCount bool                  `json:"needsCount"`
	UpdatedAt  *time.Time            `json:"updatedAt,omitempty"`
}

// LowStockConfigView es la sección «Alertas» como la pinta la ficha.
type LowStockConfigView struct {
	Enabled      bool             `json:"enabled"`
	OnMin        bool             `json:"onMin"`
	OnOut        bool             `json:"onOut"`
	Banner       bool             `json:"banner"`
	MinStock     *decimal.Decimal `json:"minStock"`
	BranchMin    *decimal.Decimal `json:"branchMinStock"`
	CoverageDays *int             `json:"coverageDays"`
	Asked        bool             `json:"asked"`
	OnHand       decimal.Decimal  `json:"onHand"`
	UnitCode     string           `json:"unitCode"`
	Branches     int              `json:"branches"`
	UpdatedAt    time.Time        `json:"updatedAt"`
}

func validLowStock(in LowStockInput) error {
	if in.MinStock != nil && !domain.ValidStockQtyOrZero(*in.MinStock) {
		return fmt.Errorf("%w: el mínimo va de 0 a %s", domain.ErrValidation, domain.MaxStockQty)
	}
	if v := in.BranchMinStock.Value; in.BranchMinStock.Set && v != nil && !domain.ValidStockQtyOrZero(*v) {
		return fmt.Errorf("%w: el mínimo de la sucursal va de 0 a %s", domain.ErrValidation, domain.MaxStockQty)
	}
	if in.CoverageDays != nil && (*in.CoverageDays < 1 || *in.CoverageDays > 90) {
		return fmt.Errorf("%w: los días de cobertura van de 1 a 90", domain.ErrValidation)
	}
	return nil
}

// LowStockConfig lee la sección «Alertas» de la ficha.
func (s *NotificationsService) LowStockConfig(ctx context.Context, itemType string, id int64) (LowStockConfigView, error) {
	t, err := itemTypeOf(itemType)
	if err != nil {
		return LowStockConfigView{}, err
	}
	var out LowStockConfigView
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		c, err := q.LowStockConfigOf(ctx, db.LowStockConfigOfParams{ItemType: t, ID: id})
		if err != nil {
			return notFound(err)
		}
		branch, err := q.CurrentBranchID(ctx)
		if err != nil {
			return err
		}
		ing, prod := itemIDs(t, id)
		snap, err := q.LowStockSnapshot(ctx, db.LowStockSnapshotParams{BranchID: branch, IngredientID: ing, ProductID: prod})
		if err != nil {
			return err
		}
		n, err := q.CompanyBranchCount(ctx)
		if err != nil {
			return err
		}
		var days *int
		if c.CoverageDays != nil {
			d := int(*c.CoverageDays)
			days = &d
		}
		out = LowStockConfigView{Enabled: c.LowStockEnabled, OnMin: c.LowStockOnMin, OnOut: c.LowStockOnOut, Banner: c.LowStockBanner,
			MinStock: c.MinStock, BranchMin: snap.BranchMin, CoverageDays: days, Asked: c.LowStockAskedAt.Valid,
			OnHand: snap.OnHand, UnitCode: c.UnitCode, Branches: int(n), UpdatedAt: c.UpdatedAt}
		return nil
	})
	return out, err
}

// ConfigureLowStock calcula el efecto y, si no es vista previa, guarda, evalúa y audita. El efecto que
// devuelve es el de la sucursal de la sesión (la que ve quien guarda); las demás se evalúan igual.
func (s *NotificationsService) ConfigureLowStock(ctx context.Context, itemType string, id int64, in LowStockInput, actor int64, preview bool) (LowStockResult, error) {
	t, err := itemTypeOf(itemType)
	if err != nil {
		return LowStockResult{}, err
	}
	if err := validLowStock(in); err != nil {
		return LowStockResult{}, err
	}
	var out LowStockResult
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		var err error
		out, err = configureLowStockIn(ctx, q, t, id, in, actor, preview)
		return err
	})
	return out, err
}

func configureLowStockIn(ctx context.Context, q *db.Queries, t db.StockItemType, id int64, in LowStockInput, actor int64, preview bool) (LowStockResult, error) {
	before, err := q.LowStockConfigOf(ctx, db.LowStockConfigOfParams{ItemType: t, ID: id})
	if err != nil {
		return LowStockResult{}, notFound(err)
	}
	if !before.IsActive {
		return LowStockResult{}, domain.ErrNotFound
	}
	if t == "ingrediente" && in.ExpectedUpdatedAt != nil && !before.UpdatedAt.Equal(*in.ExpectedUpdatedAt) {
		return LowStockResult{}, domain.ErrArticleStale
	}
	branch, err := q.CurrentBranchID(ctx)
	if err != nil {
		return LowStockResult{}, err
	}
	ing, prod := itemIDs(t, id)
	snap, err := q.LowStockSnapshot(ctx, db.LowStockSnapshotParams{BranchID: branch, IngredientID: ing, ProductID: prod})
	if err != nil {
		return LowStockResult{}, err
	}
	min := in.MinStock
	if snap.BranchMin != nil {
		min = snap.BranchMin
	}
	if in.BranchMinStock.Set {
		min = in.MinStock
		if in.BranchMinStock.Value != nil {
			min = in.BranchMinStock.Value
		}
	}
	open := make([]domain.NotificationKind, len(snap.OpenKinds))
	for i, k := range snap.OpenKinds {
		open[i] = domain.NotificationKind(k)
	}
	cfg := domain.LowStockConfig{Enabled: in.Enabled, OnMin: in.OnMin, OnOut: in.OnOut, Min: min}
	out := LowStockResult{Effect: domain.LowStockEffect(cfg, snap.OnHand, open, snap.ManualOut), OnHand: snap.OnHand, Min: min,
		NeedsCount: snap.OnHand.IsNegative()}
	if preview {
		return out, nil
	}
	var days *int16
	if in.CoverageDays != nil {
		d := int16(*in.CoverageDays)
		days = &d
	}
	var minStock *decimal.Decimal
	if in.MinStock != nil {
		m := domain.Round4(*in.MinStock)
		minStock = &m
	}
	var updatedAt time.Time
	if t == "ingrediente" {
		updatedAt, err = q.SetIngredientLowStock(ctx, db.SetIngredientLowStockParams{ID: id, Enabled: in.Enabled, OnMin: in.OnMin,
			OnOut: in.OnOut, Banner: in.Banner, MinStock: minStock, CoverageDays: days})
	} else {
		updatedAt, err = q.SetProductLowStock(ctx, db.SetProductLowStockParams{ID: id, Enabled: in.Enabled, OnMin: in.OnMin,
			OnOut: in.OnOut, Banner: in.Banner, MinStock: minStock, CoverageDays: days})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return LowStockResult{}, domain.ErrNotFound
	}
	if err != nil {
		return LowStockResult{}, err
	}
	out.UpdatedAt = &updatedAt
	if in.BranchMinStock.Set {
		if v := in.BranchMinStock.Value; v != nil {
			m := domain.Round4(*v)
			if t == "ingrediente" {
				err = q.UpsertBranchMinimum(ctx, db.UpsertBranchMinimumParams{BranchID: branch, ItemType: t, IngredientID: ing, MinStock: m, UpdatedBy: actor})
			} else {
				err = q.UpsertBranchProductMinimum(ctx, db.UpsertBranchProductMinimumParams{BranchID: branch, ProductID: prod, MinStock: m, UpdatedBy: actor})
			}
		} else {
			err = q.DeleteBranchMinimum(ctx, db.DeleteBranchMinimumParams{BranchID: branch, IngredientID: ing, ProductID: prod})
		}
		if err != nil {
			return LowStockResult{}, err
		}
	}
	if err := evaluateEverywhere(ctx, q, t, ing, prod); err != nil {
		return LowStockResult{}, err
	}
	return out, auditEntity(ctx, q, domain.AuditLowStockConfigured, string(t), id, &branch, &actor,
		map[string]any{"enabled": before.LowStockEnabled, "onMin": before.LowStockOnMin, "onOut": before.LowStockOnOut,
			"banner": before.LowStockBanner, "minStock": before.MinStock, "coverageDays": before.CoverageDays},
		map[string]any{"enabled": in.Enabled, "onMin": in.OnMin, "onOut": in.OnOut, "banner": in.Banner, "minStock": minStock,
			"coverageDays": days, "branchMinStock": in.BranchMinStock.Value, "effect": out.Effect})
}

// LowStockAsked guarda que ya se preguntó por las alertas de un insumo («No, por ahora»).
func (s *NotificationsService) LowStockAsked(ctx context.Context, id int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		n, err := q.MarkLowStockAsked(ctx, id)
		if err == nil && n == 0 {
			return domain.ErrNotFound
		}
		return err
	})
}

// LowStockBatchItem es un renglón de «Alertas por lote».
type LowStockBatchItem struct {
	ItemType string           `json:"itemType"`
	ID       int64            `json:"id"`
	Enabled  bool             `json:"enabled"`
	MinStock *decimal.Decimal `json:"minStock"`
}

// LowStockBatchResult es el efecto de un renglón.
type LowStockBatchResult struct {
	ItemType string                `json:"itemType"`
	ID       int64                 `json:"id"`
	Effect   domain.LowStockChange `json:"effect"`
	OnHand   decimal.Decimal       `json:"onHand"`
}

// ConfigureBatch enciende o apaga y fija el mínimo de varios artículos en una transacción. Conserva los
// eventos y la franja de cada uno (se cambian en su ficha). La vista previa no guarda nada.
func (s *NotificationsService) ConfigureBatch(ctx context.Context, items []LowStockBatchItem, actor int64, preview bool) ([]LowStockBatchResult, error) {
	if len(items) == 0 || len(items) > 100 {
		return nil, fmt.Errorf("%w: van de 1 a 100 artículos por lote", domain.ErrValidation)
	}
	out := make([]LowStockBatchResult, 0, len(items))
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		for _, it := range items {
			t, err := itemTypeOf(it.ItemType)
			if err != nil {
				return err
			}
			cur, err := q.LowStockConfigOf(ctx, db.LowStockConfigOfParams{ItemType: t, ID: it.ID})
			if err != nil {
				return notFound(err)
			}
			var days *int
			if cur.CoverageDays != nil {
				d := int(*cur.CoverageDays)
				days = &d
			}
			in := LowStockInput{Enabled: it.Enabled, OnMin: cur.LowStockOnMin, OnOut: cur.LowStockOnOut, Banner: cur.LowStockBanner,
				MinStock: it.MinStock, CoverageDays: days}
			if err := validLowStock(in); err != nil {
				return err
			}
			res, err := configureLowStockIn(ctx, q, t, it.ID, in, actor, preview)
			if err != nil {
				return err
			}
			out = append(out, LowStockBatchResult{ItemType: it.ItemType, ID: it.ID, Effect: res.Effect, OnHand: res.OnHand})
		}
		return nil
	})
	return out, err
}

// LowStockRow es un renglón de la tabla «Alertas por lote».
type LowStockRow struct {
	ItemType string           `json:"itemType"`
	ID       int64            `json:"id"`
	Name     string           `json:"name"`
	UnitCode string           `json:"unitCode"`
	Enabled  bool             `json:"enabled"`
	MinStock *decimal.Decimal `json:"minStock"`
	OnHand   decimal.Decimal  `json:"onHand"`
}

// ListLowStock es la tabla de «Alertas por lote». Nunca nil.
func (s *NotificationsService) ListLowStock(ctx context.Context) ([]LowStockRow, error) {
	rows, err := s.store.QC(ctx).ListLowStockConfigs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]LowStockRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, LowStockRow{ItemType: string(r.ItemType), ID: r.ID, Name: r.Name, UnitCode: r.UnitCode,
			Enabled: r.LowStockEnabled, MinStock: r.MinStock, OnHand: r.OnHand})
	}
	return out, nil
}
