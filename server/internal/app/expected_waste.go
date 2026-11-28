package app

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Merma esperada (spec 096): el % general de la empresa, el de un producto y el de un artículo. Cada
// cambio recalcula los costos en la misma transacción, como guardar una receta: nadie lee un costo
// hecho con el % de antes. Las ventas pasadas guardan su costo en el renglón y no se tocan.

// SetGeneralExpectedWaste fija la merma general; nil la quita (sin merma general).
func (s *CostingService) SetGeneralExpectedWaste(ctx context.Context, pct *decimal.Decimal, userID int64) error {
	if err := domain.ValidExpectedWastePct(pct); err != nil {
		return err
	}
	company, _ := store.CompanyFrom(ctx)
	return s.withRecompute(ctx, func(q *db.Queries) error {
		prev, err := q.SetGeneralExpectedWaste(ctx, db.SetGeneralExpectedWasteParams{Pct: pct, UpdatedBy: &userID})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: captura primero los datos del negocio", domain.ErrConflict)
		}
		if err != nil || samePct(prev, pct) {
			return err
		}
		return auditEntity(ctx, q, domain.AuditSettingsExpectedWaste, "business_settings", company, nil, &userID,
			map[string]any{"pct": prev}, map[string]any{"pct": pct})
	})
}

// SetProductExpectedWaste fija la merma propia de un producto; nil = usa la general.
func (s *CostingService) SetProductExpectedWaste(ctx context.Context, productID int64, pct *decimal.Decimal, userID int64) error {
	if err := domain.ValidExpectedWastePct(pct); err != nil {
		return err
	}
	return s.withRecompute(ctx, func(q *db.Queries) error {
		prev, err := q.SetProductExpectedWaste(ctx, db.SetProductExpectedWasteParams{ID: productID, Pct: pct})
		if err != nil {
			return notFound(err)
		}
		if samePct(prev, pct) {
			return nil
		}
		return auditEntity(ctx, q, domain.AuditProductExpectedWaste, "product", productID, nil, &userID,
			map[string]any{"pct": prev}, map[string]any{"pct": pct})
	})
}

// SetArticleExpectedWaste fija la merma de un artículo; nil = sin captura, 0 = capturada en cero.
// reason es el motivo que manda el MCP (spec 099); la pantalla manda "" y la bitácora no lo lleva.
func (s *CostingService) SetArticleExpectedWaste(ctx context.Context, ingredientID int64, pct *decimal.Decimal, userID int64, reason string) error {
	if err := domain.ValidExpectedWastePct(pct); err != nil {
		return err
	}
	reason, err := domain.NormalizeExpectedWasteReason(reason)
	if err != nil {
		return err
	}
	return s.withRecompute(ctx, func(q *db.Queries) error {
		prev, err := q.SetIngredientExpectedWaste(ctx, db.SetIngredientExpectedWasteParams{ID: ingredientID, Pct: pct})
		if err != nil {
			return notFound(err)
		}
		if samePct(prev, pct) {
			return nil
		}
		after := map[string]any{"pct": pct}
		if reason != "" {
			after["reason"] = reason
		}
		return auditEntity(ctx, q, domain.AuditIngredientExpectedWaste, "ingredient", ingredientID, nil, &userID,
			map[string]any{"pct": prev}, after)
	})
}

// WastePoint es la merma de un lado de la vista previa. Pct nil = sin captura; entonces no hay un
// costo efectivo único (depende del producto que lo use) y EffectiveCost también va nil.
type WastePoint struct {
	Pct           *decimal.Decimal `json:"pct"`
	EffectiveCost *float64         `json:"effectiveCost"`
}

// WasteFallback dice de dónde sale la merma de un lado sin captura.
type WasteFallback struct {
	ProductOrGeneral bool     `json:"productOrGeneral"`
	GeneralPct       *float64 `json:"generalPct"`
}

// WasteUse es algo que usa el insumo, con su costo antes y después (4 decimales, el redondeo de
// current_cost). Manual: producto de costo manual, que usa el insumo pero su costo no se mueve.
type WasteUse struct {
	Kind   string  `json:"kind"`
	ID     int64   `json:"id"`
	Name   string  `json:"name"`
	Before float64 `json:"before"`
	After  float64 `json:"after"`
	Manual bool    `json:"manual,omitempty"`
}

// ExpectedWastePreview es lo que el MCP enseña antes de cambiar la merma de un insumo (spec 099).
// Uses es siempre un arreglo.
type ExpectedWastePreview struct {
	IngredientID int64         `json:"ingredientId"`
	Name         string        `json:"name"`
	IsActive     bool          `json:"isActive"`
	BaseUnit     string        `json:"baseUnit"`
	UnitCost     float64       `json:"unitCost"`
	Current      WastePoint    `json:"current"`
	Next         WastePoint    `json:"next"`
	Fallback     WasteFallback `json:"fallback"`
	Unchanged    bool          `json:"unchanged"`
	Uses         []WasteUse    `json:"uses"`
}

// ArticleExpectedWastePreview simula el cambio sobre el grafo de costeo en memoria: no toma el
// candado de composición ni escribe costos para luego deshacerlos (research R3).
func (s *CostingService) ArticleExpectedWastePreview(ctx context.Context, ingredientID int64, pct *decimal.Decimal) (ExpectedWastePreview, error) {
	if err := domain.ValidExpectedWastePct(pct); err != nil {
		return ExpectedWastePreview{}, err
	}
	q := s.store.QC(ctx)
	info, err := q.IngredientWasteInfo(ctx, ingredientID)
	if err != nil {
		return ExpectedWastePreview{}, notFound(err)
	}
	g, _, _, _, err := loadGraph(ctx, q)
	if err != nil {
		return ExpectedWastePreview{}, err
	}
	unit := g.IngredientCost(ingredientID)
	point := func(p *decimal.Decimal) WastePoint {
		if p == nil {
			return WastePoint{}
		}
		eff := round4(unit * (1 + p.InexactFloat64()/100))
		return WastePoint{Pct: p, EffectiveCost: &eff}
	}
	impacts := domain.ExpectedWasteImpact(g, g.WithIngredientWaste(ingredientID, pctF(pct)), ingredientID)
	names, err := costingNames(ctx, q, impacts)
	if err != nil {
		return ExpectedWastePreview{}, err
	}
	uses := make([]WasteUse, len(impacts))
	for i, im := range impacts {
		uses[i] = WasteUse{Kind: im.Kind, ID: im.ID, Name: names[im.Kind][im.ID],
			Before: round4(im.Before), After: round4(im.After), Manual: im.Manual}
	}
	return ExpectedWastePreview{
		IngredientID: info.ID, Name: info.Name, IsActive: info.IsActive, BaseUnit: info.BaseUnit,
		UnitCost: round4(unit), Current: point(info.ExpectedWastePct), Next: point(pct),
		Fallback:  WasteFallback{ProductOrGeneral: info.ExpectedWastePct == nil || pct == nil, GeneralPct: g.General()},
		Unchanged: samePct(info.ExpectedWastePct, pct), Uses: uses,
	}, nil
}

func costingNames(ctx context.Context, q *db.Queries, impacts []domain.CostImpact) (map[string]map[int64]string, error) {
	ids := map[string][]int64{"product": {}, "option": {}, "prep": {}}
	for _, im := range impacts {
		ids[im.Kind] = append(ids[im.Kind], im.ID)
	}
	rows, err := q.CostingNames(ctx, db.CostingNamesParams{ProductIds: ids["product"], OptionIds: ids["option"], PrepIds: ids["prep"]})
	if err != nil {
		return nil, err
	}
	out := map[string]map[int64]string{"product": {}, "option": {}, "prep": {}}
	for _, r := range rows {
		out[r.Kind][r.ID] = r.Name
	}
	return out, nil
}

func round4(x float64) float64 { return math.Round(x*1e4) / 1e4 }

// withRecompute corre el cambio y el recálculo en una transacción, con el candado de composición
// tomado primero (el mismo orden que RecomputeAll: candado y después filas).
func (s *CostingService) withRecompute(ctx context.Context, change func(q *db.Queries) error) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockCompositionEdits(ctx); err != nil {
			return err
		}
		if err := change(q); err != nil {
			return err
		}
		return recomputeCosts(ctx, q)
	})
}

// samePct compara dos % nullable: nil y 0 son distintos («sin captura» contra «capturado en 0»).
func samePct(a, b *decimal.Decimal) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

// SetExpectedWasteDeducts enciende o apaga que la merma esperada descuente del almacén (spec 096,
// subida 2). Solo cambia lo que se venda después: ninguna venta ni existencia pasada se toca.
func (s *CostingService) SetExpectedWasteDeducts(ctx context.Context, deducts bool, userID int64) error {
	company, _ := store.CompanyFrom(ctx)
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		prev, err := q.SetExpectedWasteDeducts(ctx, db.SetExpectedWasteDeductsParams{Deducts: deducts, UpdatedBy: &userID})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: captura primero los datos del negocio", domain.ErrConflict)
		}
		if err != nil || prev == deducts {
			return err
		}
		return auditEntity(ctx, q, domain.AuditSettingsExpectedWasteDeducts, "business_settings", company, nil, &userID,
			map[string]any{"deducts": prev}, map[string]any{"deducts": deducts})
	})
}
