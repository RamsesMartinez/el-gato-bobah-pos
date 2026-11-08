package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Productos con existencia propia en el conteo (spec 069, issue #252): la lata que se vende tal
// cual se cuenta igual que un artículo —a ciegas, guardada al capturar con lo que el sistema tenía,
// y ajustada al cerrar contra eso—, en piezas enteras.

// productKind es la clase de unidad de un producto con existencia propia: siempre piezas.
const productKind = string(db.UnitKindPieza)

// CountProductItem es un producto por contar. Sin lo que el sistema tiene: el conteo es a ciegas.
type CountProductItem struct {
	ProductID  int64            `json:"productId"`
	Name       string           `json:"name"`
	Category   string           `json:"category"`
	CountedQty *decimal.Decimal `json:"countedQty"`
	// AltNames: otros nombres para encontrarlo en la lista del conteo (spec 063). Siempre un arreglo.
	AltNames []string `json:"altNames"`
}

// CountProductAdjustmentView es un producto contado al cerrar: lo que el sistema tenía, lo contado
// y la diferencia, en piezas.
type CountProductAdjustmentView struct {
	ProductID int64           `json:"productId"`
	Name      string          `json:"name"`
	System    decimal.Decimal `json:"system"`
	Counted   decimal.Decimal `json:"counted"`
	Delta     decimal.Decimal `json:"delta"`
}

// CountProductView es un producto sin contar, o uno contado que ya no lleva su existencia.
type CountProductView struct {
	ProductID int64  `json:"productId"`
	Name      string `json:"name"`
}

func countProducts(ctx context.Context, q *db.Queries, countID int64) ([]CountProductItem, error) {
	rows, err := q.CountProducts(ctx, countID)
	if err != nil {
		return nil, err
	}
	out := make([]CountProductItem, len(rows))
	for i, r := range rows {
		out[i] = CountProductItem{ProductID: r.ID, Name: r.Name, Category: r.Category, CountedQty: r.CountedQty,
			AltNames: nonNilStrings(r.AltNames)}
	}
	return out, nil
}

// CaptureCountProduct guarda lo contado de un producto con existencia propia, con lo que el sistema
// tiene ahora en la sucursal del conteo. Recapturar reemplaza las dos. Un producto que no lleva su
// propia existencia no se cuenta: no encontrado.
func (s *BackofficeService) CaptureCountProduct(ctx context.Context, countID, productID int64, in CountCapture, userID int64) (decimal.Decimal, error) {
	counted := domain.Round4(in.Qty)
	if !domain.ValidCountPieces(in.Qty) {
		return counted, fmt.Errorf("%w: cuenta piezas enteras", domain.ErrValidation)
	}
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		c, err := openCount(ctx, q, countID)
		if err != nil {
			return err
		}
		p, err := q.GetProductForCount(ctx, productID)
		if err != nil {
			return notFound(err)
		}
		if !p.IsActive || !domain.CountsOwnStock(domain.ProductModeFacts{TrackStock: p.TrackStock, IsPackage: p.IsPackage}) {
			return fmt.Errorf("%w: este producto no lleva su propia existencia", domain.ErrNotFound)
		}
		// La fila de existencia primero: un producto que nunca se movió en la sucursal no la tiene, y
		// sin fila el candado no toma nada. Después el candado y, en otra sentencia, la lectura.
		if err := q.EnsureProductStockLevel(ctx, db.EnsureProductStockLevelParams{BranchID: c.BranchID, ProductID: productID}); err != nil {
			return err
		}
		if err := q.LockProductStockLevelForCapture(ctx, db.LockProductStockLevelForCaptureParams{ProductID: productID, BranchID: c.BranchID}); err != nil {
			return err
		}
		seen, err := q.ProductOnHandForCapture(ctx, db.ProductOnHandForCaptureParams{ProductID: productID, BranchID: c.BranchID})
		if err != nil {
			return err
		}
		limits, err := countLimits(ctx, q)
		if err != nil {
			return err
		}
		if err := limits.CheckCountMax(productKind, counted); err != nil {
			return err
		}
		if in.Confirmable && !in.Confirmed {
			ref := domain.CountReference{Expected: seen.OnHand, BaseKind: productKind}
			last, err := q.LastClosedCountedProductQty(ctx, db.LastClosedCountedProductQtyParams{BranchID: c.BranchID, ProductID: productID})
			switch {
			case err == nil:
				ref.LastCounted = &last
			case !errors.Is(err, pgx.ErrNoRows):
				return err
			}
			if pair, ok := limits.ForKind(productKind); ok {
				ref.Confirm = pair.Confirm
			}
			if domain.CountNeedsConfirm(counted, ref) {
				return domain.ErrCountNeedsConfirm
			}
		}
		return q.UpsertCountProductLine(ctx, db.UpsertCountProductLineParams{
			CountID: countID, ProductID: productID, CountedQty: counted, SystemQty: seen.OnHand, CapturedBy: userID,
			LastMovementID: &seen.LastMovementID,
		})
	})
	return counted, err
}

// UncaptureCountProduct quita lo contado de un producto: vuelve a «no contado».
func (s *BackofficeService) UncaptureCountProduct(ctx context.Context, countID, productID int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if _, err := openCount(ctx, q, countID); err != nil {
			return err
		}
		_, err := q.DeleteCountProductLine(ctx, db.DeleteCountProductLineParams{CountID: countID, ProductID: productID})
		return err
	})
}

// lockCountedProducts toma sin esperar las existencias de todos los productos contados, también
// las de los que se van a saltar: un cambio de modo en curso las tiene tomadas, y el cierre se
// reintenta en vez de leer un modo que está por cambiar. Va ANTES de leer las cifras.
//
// El conteo ya está bloqueado (openCount), así que ninguna captura cambia sus renglones entre esta
// lectura y la del plan.
func lockCountedProducts(ctx context.Context, q *db.Queries, countID, branchID int64) error {
	lines, err := q.CountProductLines(ctx, countID)
	if err != nil || len(lines) == 0 {
		return err
	}
	ids := make([]int64, len(lines))
	for i, l := range lines {
		ids[i] = l.ProductID
	}
	_, err = q.LockProductStockLevelsInBranchNowait(ctx, db.LockProductStockLevelsInBranchNowaitParams{Ids: ids, BranchID: branchID})
	return lockErr(err)
}

// productPlan es la parte de productos de la vista previa y del cierre. Un producto contado que
// ya no lleva su propia existencia se salta: ajustarlo le reviviría el contador (FR-009).
func productPlan(ctx context.Context, q *db.Queries, countID int64, out *CountCloseView) (plan []domain.CountAdjustment, processed []int64, err error) {
	lines, err := q.CountProductLines(ctx, countID)
	if err != nil {
		return nil, nil, err
	}
	in := make([]domain.CountLine, 0, len(lines))
	names := make(map[int64]string, len(lines))
	for _, l := range lines {
		if !l.CountsOwnStock {
			out.SkippedProducts = append(out.SkippedProducts, CountProductView{ProductID: l.ProductID, Name: l.Name})
			continue
		}
		in = append(in, domain.CountLine{ItemID: l.ProductID, Counted: l.CountedQty, System: l.SystemQty})
		names[l.ProductID] = l.Name
		processed = append(processed, l.ProductID)
	}
	plan = domain.PlanCountClose(in)
	for _, a := range plan {
		out.AdjustedProducts = append(out.AdjustedProducts, CountProductAdjustmentView{ProductID: a.ItemID, Name: names[a.ItemID],
			System: a.System, Counted: a.Counted, Delta: a.Delta})
	}
	nc, err := q.NotCountedProducts(ctx, countID)
	if err != nil {
		return nil, nil, err
	}
	for _, n := range nc {
		out.NotCountedProducts = append(out.NotCountedProducts, CountProductView{ProductID: n.ID, Name: n.Name})
	}
	return plan, processed, nil
}

// closeProducts inserta el ajuste de cada producto con diferencia, ligado al conteo, y guarda el
// costo de los que procesó.
func closeProducts(ctx context.Context, q *db.Queries, countID, branchID, userID int64, plan []domain.CountAdjustment, processed []int64) error {
	reason := "conteo"
	for _, a := range plan {
		if err := q.InsertCountProductMovement(ctx, db.InsertCountProductMovementParams{
			ProductID: &a.ItemID, Quantity: a.Delta, StockCountID: &countID, UserID: &userID, Reason: &reason, BranchID: branchID,
		}); err != nil {
			return err
		}
	}
	if len(processed) == 0 {
		return nil
	}
	return q.SaveCountProductLineCosts(ctx, db.SaveCountProductLineCostsParams{CountID: countID, Ids: processed})
}
