package app

import (
	"context"
	"errors"
	"fmt"

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
func (s *CostingService) SetArticleExpectedWaste(ctx context.Context, ingredientID int64, pct *decimal.Decimal, userID int64) error {
	if err := domain.ValidExpectedWastePct(pct); err != nil {
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
		return auditEntity(ctx, q, domain.AuditIngredientExpectedWaste, "ingredient", ingredientID, nil, &userID,
			map[string]any{"pct": prev}, map[string]any{"pct": pct})
	})
}

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
