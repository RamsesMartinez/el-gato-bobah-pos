package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Cambiar en qué se mide un artículo (spec 042, US5, FR-009). Sin compras, ventas ni mermas el cambio
// es directo: el mismo artículo con todo lo que guarda en su unidad convertido, y los originales en la
// bitácora. Con ellas se reemplaza: uno nuevo en la otra unidad y el viejo archivado con su historia.

// UnitChangeTables dice qué hace el cambio de unidad con cada tabla que apunta a un artículo. Una tabla
// nueva que no esté aquí hace tronar TestEveryTableThatPointsToAnArticleIsClassified: si guarda
// cantidades en la unidad del artículo y nadie la convierte, el cambio la deja mintiendo.
var UnitChangeTables = map[string]string{
	"stock_movements":             "convierte los ajustes; compras, ventas y mermas obligan al reemplazo",
	"stock_levels":                "se recalcula como la suma de los movimientos",
	"stock_count_lines":           "convierte; un conteo abierto con el artículo lo bloquea",
	"recipe_items":                "convierte la cantidad y la pasa a la unidad base nueva",
	"ingredient_presentations":    "convierte; el que no cuadra se archiva",
	"ingredient_purchase_formats": "convierte",
	"supplier_items":              "convierte el contenido aprendido",
	"waste_records":               "una pendiente bloquea; las demás se convierten",
	"expense_items":               "uno sin anular bloquea (compra por recibir)",
	"ingredient_cost_corrections": "convierte el costo",
	"ingredient_replacements":     "no guarda cantidades del artículo",
	"ingredient_distinct_pairs":   "no guarda cantidades del artículo",
}

// UnitChangeRequest es la clase nueva (su unidad base) y la equivalencia: cuánto de la base nueva vale
// una de la vieja. Overrides corrige cantidades convertidas, por clave («recipe:12», «presentation:3»,
// «stock:1» en un reemplazo).
type UnitChangeRequest struct {
	BaseUnitID int16                      `json:"baseUnitId"`
	Factor     decimal.Decimal            `json:"factor"`
	Overrides  map[string]decimal.Decimal `json:"overrides"`
}

// UnitConversion es una cantidad que el cambio convierte, para revisarla antes de confirmar.
type UnitConversion struct {
	Key   string          `json:"key"`
	Group string          `json:"group"` // stock | recipe | presentation
	Label string          `json:"label"`
	From  decimal.Decimal `json:"from"`
	To    decimal.Decimal `json:"to"`
	Fits  bool            `json:"fits"`
}

// UnitChangePreview es lo que pasaría: el camino y cada cantidad convertida.
type UnitChangePreview struct {
	Path        domain.UnitPath  `json:"path"`
	FromUnit    string           `json:"fromUnit"`
	ToUnit      string           `json:"toUnit"`
	Conversions []UnitConversion `json:"conversions"`
}

type unitPlan struct {
	art     db.LockArticleNowaitRow
	from    db.Unit
	to      db.Unit
	path    domain.UnitPath
	factor  decimal.Decimal
	recipes []db.ArticleRecipeItemsRow
	pres    []db.ListPresentationsRow
	stock   []db.ArticleOnHandByBranchRow
	preview UnitChangePreview
}

// planUnitChange valida y arma la conversión. Va dentro de la transacción del cambio, con el artículo
// ya bloqueado, o sola para la vista previa.
func planUnitChange(ctx context.Context, q *db.Queries, art db.LockArticleNowaitRow, req UnitChangeRequest) (unitPlan, error) {
	p := unitPlan{art: art, factor: req.Factor}
	var err error
	if p.from, err = q.GetUnit(ctx, art.BaseUnitID); err != nil {
		return p, err
	}
	if p.to, err = q.GetUnit(ctx, req.BaseUnitID); err != nil {
		return p, fmt.Errorf("%w: unidad desconocida", domain.ErrValidation)
	}
	if !p.to.ToBase.Equal(decimal.NewFromInt(1)) {
		return p, fmt.Errorf("%w: elige la unidad base de la clase (g, ml, cm o pieza)", domain.ErrValidation)
	}
	if err := domain.ValidUnitChange(string(p.from.Kind), string(p.to.Kind), req.Factor); err != nil {
		return p, err
	}
	id := art.ID
	// Un destino con insumos unidos no cambia de unidad (spec 056): la equivalencia de cada unión habla
	// de la unidad de hoy, y el historial de sus orígenes dejaría de leerse bien.
	if merged, err := q.CountMergedSources(ctx, id); err != nil {
		return p, err
	} else if merged > 0 {
		return p, fmt.Errorf("%w: tiene %d insumo(s) unido(s); su unidad ya no se cambia", domain.ErrMergeBlocked, merged)
	}
	if open, err := q.OpenCountHasArticle(ctx, id); err != nil {
		return p, err
	} else if open {
		return p, fmt.Errorf("%w: un conteo abierto ya lo capturó; ciérralo o descártalo antes", domain.ErrArticleInUse)
	}
	types, err := q.ArticleMovementTypes(ctx, &id)
	if err != nil {
		return p, err
	}
	p.path = domain.UnitChangePath(types)
	receipts, err := q.PendingReceiptsOf(ctx, &id)
	if err != nil {
		return p, err
	}
	if p.path == domain.UnitPathDirect {
		// Sin compras recibidas, cualquier renglón sin anular es una compra que todavía no llega.
		if receipts, err = q.OpenExpenseItemsOf(ctx, &id); err != nil {
			return p, err
		}
	}
	waste, err := q.PendingWasteOf(ctx, &id)
	if err != nil {
		return p, err
	}
	if receipts > 0 || waste > 0 {
		return p, domain.UnitChangePending{Receipts: int(receipts), Waste: int(waste)}
	}
	if p.path == domain.UnitPathReplace && art.IsPrep {
		return p, fmt.Errorf("%w: un preparado con movimientos cambia de unidad desde su receta", domain.ErrConflict)
	}
	if p.recipes, err = q.ArticleRecipeItems(ctx, id); err != nil {
		return p, err
	}
	if p.pres, err = q.ListPresentations(ctx, &id); err != nil {
		return p, err
	}
	if p.stock, err = q.ArticleOnHandByBranch(ctx, &id); err != nil {
		return p, err
	}
	if !domain.ValidConvertedCost(art.CurrentCost, domain.ConvertCost(art.CurrentCost, req.Factor)) {
		return p, fmt.Errorf("%w: con esa equivalencia el costo quedaría en cero o no cabría; revisa la equivalencia", domain.ErrValidation)
	}
	if art.MinStock != nil && !domain.ValidConvertedQty(domain.ConvertQty(*art.MinStock, req.Factor)) {
		return p, fmt.Errorf("%w: con esa equivalencia el mínimo no cabe", domain.ErrValidation)
	}
	p.preview = UnitChangePreview{Path: p.path, FromUnit: p.from.Code, ToUnit: p.to.Code, Conversions: []UnitConversion{}}
	for _, s := range p.stock {
		key := "stock:" + strconv.FormatInt(s.BranchID, 10)
		to := override(req, key, domain.ConvertQty(s.OnHand, req.Factor))
		p.preview.Conversions = append(p.preview.Conversions, UnitConversion{Key: key, Group: "stock", Label: "Existencia · " + s.Branch,
			From: s.OnHand, To: to, Fits: true})
	}
	for _, r := range p.recipes {
		key := "recipe:" + strconv.FormatInt(r.ID, 10)
		to := override(req, key, domain.ConvertRecipeQty(r.Quantity, r.UnitToBase, req.Factor))
		p.preview.Conversions = append(p.preview.Conversions, UnitConversion{Key: key, Group: "recipe", Label: "Receta: " + r.Owner,
			From: r.Quantity.Mul(r.UnitToBase), To: to, Fits: to.IsPositive()})
	}
	for _, pr := range p.pres {
		if !pr.Active {
			continue
		}
		key := "presentation:" + strconv.FormatInt(pr.ID, 10)
		to := override(req, key, domain.ConvertQty(pr.QtyInBase, req.Factor))
		p.preview.Conversions = append(p.preview.Conversions, UnitConversion{Key: key, Group: "presentation", Label: "Empaque: " + pr.Name,
			From: pr.QtyInBase, To: to, Fits: presentationFits(ctx, q, pr, p.to, to)})
	}
	if err := checkConversions(p, req); err != nil {
		return p, err
	}
	return p, nil
}

// checkConversions rechaza como 400 lo que no cabe o no tiene sentido: una cantidad que desborda la
// columna, una corrección de una llave que no está en el plan, una existencia negativa, una receta o
// un empaque en cero, y corregir la existencia en el cambio directo (ahí es la suma de su historia).
func checkConversions(p unitPlan, req UnitChangeRequest) error {
	known := map[string]string{}
	for _, c := range p.preview.Conversions {
		known[c.Key] = c.Group
		if !domain.ValidConvertedQty(c.To) {
			return fmt.Errorf("%w: %s no cabe con esa equivalencia", domain.ErrValidation, c.Label)
		}
	}
	for key, v := range req.Overrides {
		group, ok := known[key]
		switch {
		case !ok:
			return fmt.Errorf("%w: esa cantidad no está en el cambio", domain.ErrValidation)
		case group == "stock" && p.path == domain.UnitPathDirect:
			return fmt.Errorf("%w: en este cambio la existencia sale de su historia; no se corrige a mano", domain.ErrValidation)
		case group == "stock" && !domain.ValidStockQtyOrZero(v):
			return fmt.Errorf("%w: la existencia no puede ser negativa", domain.ErrValidation)
		case group != "stock" && !v.IsPositive():
			return fmt.Errorf("%w: una cantidad de receta o de empaque va mayor que cero", domain.ErrValidation)
		case !domain.ValidConvertedQty(v):
			return fmt.Errorf("%w: esa cantidad no cabe", domain.ErrValidation)
		}
	}
	return nil
}

func override(req UnitChangeRequest, key string, v decimal.Decimal) decimal.Decimal {
	if o, ok := req.Overrides[key]; ok {
		return domain.Round4(o)
	}
	return v
}

// presentationFits: un empaque aproximado cuya unidad de compra queda de la misma clase que la base
// nueva deja de ser aproximado y no cuadra (se archiva); los demás, por su cantidad.
func presentationFits(ctx context.Context, q *db.Queries, pr db.ListPresentationsRow, to db.Unit, qty decimal.Decimal) bool {
	if pr.IsApproximate && pr.PurchaseUnitID != nil {
		pu, err := q.GetUnit(ctx, *pr.PurchaseUnitID)
		if err != nil || pu.Kind == to.Kind {
			return false
		}
		return qty.IsPositive()
	}
	return domain.PresentationFits(string(to.Kind), qty)
}

// PreviewUnitChange dice qué pasaría, sin cambiar nada.
func (s *BackofficeService) PreviewUnitChange(ctx context.Context, id int64, req UnitChangeRequest) (UnitChangePreview, error) {
	var out UnitChangePreview
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		art, err := q.LockArticleNowait(ctx, id)
		if err != nil {
			return notFound(lockErr(err))
		}
		if !art.IsActive {
			return fmt.Errorf("%w: reactívalo antes de cambiar en qué se mide", domain.ErrConflict)
		}
		p, err := planUnitChange(ctx, q, art, req)
		out = p.preview
		return err
	})
	return out, err
}

// ChangeUnit aplica el cambio de unidad por el camino que corresponde, en una sola transacción.
func (s *BackofficeService) ChangeUnit(ctx context.Context, id int64, req UnitChangeRequest) error {
	actor, ok := ActorUserFrom(ctx)
	if !ok {
		return fmt.Errorf("%w: cambiar la unidad necesita a la persona que lo hace", domain.ErrForbidden)
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		// El candado del recálculo antes que cualquier fila (ver recomputeCosts).
		if err := q.LockCompositionEdits(ctx); err != nil {
			return err
		}
		// Artículo y existencias sin esperar: una venta en curso responde «vuelve a intentar» (E6).
		art, err := q.LockArticleNowait(ctx, id)
		if err != nil {
			return notFound(lockErr(err))
		}
		if !art.IsActive {
			return fmt.Errorf("%w: reactívalo antes de cambiar en qué se mide", domain.ErrConflict)
		}
		locked, err := q.LockArticleStockNowait(ctx, &id)
		if err != nil {
			return lockErr(err)
		}
		p, err := planUnitChange(ctx, q, art, req)
		if err != nil {
			return err
		}
		if p.path == domain.UnitPathDirect {
			err = changeUnitDirect(ctx, q, p, req)
		} else {
			err = replaceArticle(ctx, q, p, req, locked, actor)
		}
		if err != nil {
			return err
		}
		// Las recetas quedaron convertidas (y a veces corregidas a mano): lo que las lleva cambia de costo.
		return recomputeCosts(ctx, q)
	})
}

func newBatchID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// changeUnitDirect convierte el mismo artículo. El orden importa: primero la unidad del artículo (los
// triggers de recetas y empaques comparan contra ella), después lo que cuelga de él, y al final la
// existencia, como la suma de los movimientos ya convertidos (Revisión B2 y B3).
func changeUnitDirect(ctx context.Context, q *db.Queries, p unitPlan, req UnitChangeRequest) error {
	f := req.Factor
	id := p.art.ID
	batch := newBatchID()
	detail := func(table string, originals any) error {
		return recordAudit(ctx, q, domain.AuditEvent{
			Action: domain.AuditIngredientUnitChangedDetail, EntityType: "ingredient", EntityID: id,
			Before: map[string]any{"table": table, "unit": p.from.Code, "rows": originals},
			After:  map[string]any{"batchId": batch, "unit": p.to.Code},
		})
	}
	conv := func(v *decimal.Decimal) *decimal.Decimal {
		if v == nil {
			return nil
		}
		r := domain.ConvertQty(*v, f)
		return &r
	}
	cost := func(v *decimal.Decimal) *decimal.Decimal {
		if v == nil {
			return nil
		}
		r := domain.ConvertCost(*v, f)
		return &r
	}
	if err := q.SetArticleUnit(ctx, db.SetArticleUnitParams{
		ID: id, BaseUnitID: p.to.ID, MinStock: conv(p.art.MinStock), CurrentCost: domain.ConvertCost(p.art.CurrentCost, f), YieldQty: conv(p.art.YieldQty),
	}); err != nil {
		return err
	}
	if err := detail("ingredients", map[string]any{"minStock": p.art.MinStock, "currentCost": p.art.CurrentCost, "yieldQty": p.art.YieldQty}); err != nil {
		return err
	}

	recipeOrig := []map[string]any{}
	for _, r := range p.recipes {
		key := "recipe:" + strconv.FormatInt(r.ID, 10)
		to := override(req, key, domain.ConvertRecipeQty(r.Quantity, r.UnitToBase, f))
		if !to.IsPositive() {
			return fmt.Errorf("%w: la receta de %s quedaría en cero; corrige su cantidad", domain.ErrValidation, r.Owner)
		}
		if err := q.SetRecipeItem(ctx, db.SetRecipeItemParams{ID: r.ID, IngredientID: id, Quantity: to, UnitID: p.to.ID}); err != nil {
			return err
		}
		recipeOrig = append(recipeOrig, map[string]any{"id": r.ID, "quantity": r.Quantity, "unit": r.UnitCode})
	}
	presOrig := []map[string]any{}
	for _, pr := range p.pres {
		key := "presentation:" + strconv.FormatInt(pr.ID, 10)
		to := override(req, key, domain.ConvertQty(pr.QtyInBase, f))
		fits := pr.Active && presentationFits(ctx, q, pr, p.to, to)
		params := db.SetPresentationRawParams{ID: pr.ID, Name: pr.Name, QtyInBase: to, PurchaseUnitID: pr.PurchaseUnitID, IsApproximate: pr.IsApproximate, Active: pr.Active && fits}
		if !fits {
			// Archivado, nunca borrado; la fila tiene que seguir siendo válida para la base: un aproximado
			// cuya unidad de compra ya es de la misma clase deja de serlo, y una pieza trae piezas enteras.
			if pr.IsApproximate {
				params.IsApproximate, params.PurchaseUnitID = false, nil
				params.Name = archivedName(pr.Name)
			}
			if p.to.Kind == db.UnitKindPieza {
				params.QtyInBase = decimal.Max(params.QtyInBase.Round(0), decimal.NewFromInt(1))
			}
			if !params.QtyInBase.IsPositive() {
				params.QtyInBase = decimal.NewFromInt(1)
			}
		}
		if err := q.SetPresentationRaw(ctx, params); err != nil {
			return err
		}
		presOrig = append(presOrig, map[string]any{"id": pr.ID, "name": pr.Name, "qtyInBase": pr.QtyInBase, "active": pr.Active,
			"approximate": pr.IsApproximate, "purchaseUnitId": pr.PurchaseUnitID})
	}
	if err := detail("recipe_items", recipeOrig); err != nil {
		return err
	}
	if err := detail("ingredient_presentations", presOrig); err != nil {
		return err
	}

	adj, err := q.ArticleAdjustments(ctx, &id)
	if err != nil {
		return err
	}
	adjOrig := []map[string]any{}
	for _, a := range adj {
		to := domain.ConvertQty(a.Quantity, f)
		if !domain.ValidConvertedQty(to) {
			return fmt.Errorf("%w: un ajuste de su historia no cabe con esa equivalencia", domain.ErrValidation)
		}
		if to.IsZero() { // la columna no admite cero; la diferencia la absorbe la existencia recalculada
			to = decimal.New(1, -4)
			if a.Quantity.IsNegative() {
				to = to.Neg()
			}
		}
		if err := q.SetMovementQtyCost(ctx, db.SetMovementQtyCostParams{ID: a.ID, Quantity: to, UnitCost: cost(a.UnitCost)}); err != nil {
			return err
		}
		adjOrig = append(adjOrig, map[string]any{"id": a.ID, "quantity": a.Quantity, "unitCost": a.UnitCost})
	}
	if err := detail("stock_movements", adjOrig); err != nil {
		return err
	}
	lines, err := q.ArticleCountLines(ctx, id)
	if err != nil {
		return err
	}
	lineOrig := []map[string]any{}
	for _, l := range lines {
		if err := q.SetCountLineQty(ctx, db.SetCountLineQtyParams{CountID: l.CountID, IngredientID: id,
			CountedQty: domain.ConvertQty(l.CountedQty, f), SystemQty: domain.ConvertQty(l.SystemQty, f), UnitCost: cost(l.UnitCost)}); err != nil {
			return err
		}
		lineOrig = append(lineOrig, map[string]any{"countId": l.CountID, "counted": l.CountedQty, "system": l.SystemQty, "unitCost": l.UnitCost})
	}
	if err := detail("stock_count_lines", lineOrig); err != nil {
		return err
	}
	if err := convertSideTables(ctx, q, id, id, f, detail); err != nil {
		return err
	}
	if err := q.RecomputeArticleStock(ctx, &id); err != nil {
		return err
	}
	return recordAudit(ctx, q, domain.AuditEvent{
		Action: domain.AuditIngredientUnitChanged, EntityType: "ingredient", EntityID: id,
		Before: map[string]any{"unit": p.from.Code},
		After: map[string]any{"unit": p.to.Code, "factor": f, "batchId": batch, "adjustments": len(adj),
			"recipes": len(p.recipes), "presentations": len(p.pres), "countLines": len(lines)},
	})
}

// convertSideTables pasa a la unidad nueva lo que queda: formatos de compra, mapeos de proveedor (al
// artículo `to`), mermas no pendientes y correcciones de costo (estas solo en el cambio directo, from
// == to: en un reemplazo cada artículo conserva las suyas).
func convertSideTables(ctx context.Context, q *db.Queries, from, to int64, f decimal.Decimal, detail func(string, any) error) error {
	formats, err := q.ArticlePurchaseFormats(ctx, from)
	if err != nil {
		return err
	}
	orig := []map[string]any{}
	for _, pf := range formats {
		if err := q.SetPurchaseFormatQty(ctx, db.SetPurchaseFormatQtyParams{ID: pf.ID, IngredientID: to, QtyInBase: domain.ConvertQty(pf.QtyInBase, f)}); err != nil {
			return err
		}
		orig = append(orig, map[string]any{"id": pf.ID, "ingredientId": from, "qtyInBase": pf.QtyInBase})
	}
	if err := detail("ingredient_purchase_formats", orig); err != nil {
		return err
	}
	items, err := q.ArticleSupplierItems(ctx, &from)
	if err != nil {
		return err
	}
	orig = []map[string]any{}
	for _, si := range items {
		var pack *decimal.Decimal
		if si.PackQtyInBase != nil {
			v := domain.ConvertQty(*si.PackQtyInBase, f)
			pack = &v
		}
		if err := q.SetSupplierItem(ctx, db.SetSupplierItemParams{ID: si.ID, IngredientID: &to, PackQtyInBase: pack}); err != nil {
			return err
		}
		orig = append(orig, map[string]any{"id": si.ID, "ingredientId": from, "packQtyInBase": si.PackQtyInBase})
	}
	if err := detail("supplier_items", orig); err != nil {
		return err
	}
	if from != to {
		return nil
	}
	waste, err := q.ArticleWasteRecords(ctx, &from)
	if err != nil {
		return err
	}
	orig = []map[string]any{}
	for _, w := range waste {
		var c *decimal.Decimal
		if w.UnitCost != nil {
			v := domain.ConvertCost(*w.UnitCost, f)
			c = &v
		}
		if err := q.SetWasteQtyCost(ctx, db.SetWasteQtyCostParams{ID: w.ID, Quantity: domain.ConvertQty(w.Quantity, f), UnitCost: c}); err != nil {
			return err
		}
		orig = append(orig, map[string]any{"id": w.ID, "quantity": w.Quantity, "unitCost": w.UnitCost})
	}
	if err := detail("waste_records", orig); err != nil {
		return err
	}
	corr, err := q.ArticleCostCorrections(ctx, from)
	if err != nil {
		return err
	}
	orig = []map[string]any{}
	for _, c := range corr {
		if err := q.SetCostCorrection(ctx, db.SetCostCorrectionParams{ID: c.ID,
			PreviousCost: domain.ConvertCost(c.PreviousCost, f), NewCost: domain.ConvertCost(c.NewCost, f)}); err != nil {
			return err
		}
		orig = append(orig, map[string]any{"id": c.ID, "previousCost": c.PreviousCost, "newCost": c.NewCost})
	}
	return detail("ingredient_cost_corrections", orig)
}

// archivedName es el nombre de un empaque que deja de cuadrar: no puede llamarse como una unidad
// («Kilo») si deja de ser aproximado, así que lleva la marca de por qué se archivó.
func archivedName(name string) string {
	n := []rune(name + " (antes)")
	if len(n) > 40 {
		n = n[:40]
	}
	return string(n)
}

// replaceArticle crea uno nuevo en la unidad nueva y archiva el viejo con su historia intacta (E8).
func replaceArticle(ctx context.Context, q *db.Queries, p unitPlan, req UnitChangeRequest, locked []db.LockArticleStockNowaitRow, actor int64) error {
	f := req.Factor
	old := p.art
	// El nombre es único por empresa: el viejo cede el suyo y dice en qué se medía.
	oldName := domain.CleanArticleName(old.Name)
	if err := q.LockIngredientNames(ctx); err != nil {
		return err
	}
	retired, err := retiredName(ctx, q, oldName, p.from.Code)
	if err != nil {
		return err
	}
	if err := q.RenameArticle(ctx, db.RenameArticleParams{ID: old.ID, Name: retired}); err != nil {
		return err
	}
	var minStock *decimal.Decimal
	if old.MinStock != nil {
		v := domain.ConvertQty(*old.MinStock, f)
		minStock = &v
	}
	newID, err := createArticleTx(ctx, q, ArticleCreate{Name: oldName, BaseUnitID: p.to.ID, CategoryID: old.CategoryID, PlaceID: old.CountPlaceID, MinStock: minStock})
	if err != nil {
		return err
	}
	if err := q.SetArticleExtras(ctx, db.SetArticleExtrasParams{ID: newID, PlaceID: old.CountPlaceID, TrackStock: old.TrackStock, IsPackaging: old.IsPackaging}); err != nil {
		return err
	}
	repID, err := q.InsertReplacement(ctx, db.InsertReplacementParams{OldIngredientID: old.ID, NewIngredientID: newID, Factor: f, UserID: actor})
	if err != nil {
		return err
	}
	newCost := domain.ConvertCost(old.CurrentCost, f)
	if newCost.IsPositive() {
		prev, err := q.SetArticleCostManual(ctx, db.SetArticleCostManualParams{ID: newID, Cost: newCost})
		if err != nil {
			return err
		}
		if err := q.InsertCostCorrection(ctx, db.InsertCostCorrectionParams{IngredientID: newID, PreviousCost: prev, NewCost: newCost, UserID: actor}); err != nil {
			return err
		}
	}
	// El traspaso, sucursal por sucursal, con su sucursal explícita y ligado al reemplazo.
	oldCost := old.CurrentCost
	moved := []map[string]any{}
	for _, s := range locked {
		if s.OnHand.IsZero() {
			continue
		}
		to := override(req, "stock:"+strconv.FormatInt(s.BranchID, 10), domain.ConvertQty(s.OnHand, f))
		if err := q.InsertReplacementMove(ctx, db.InsertReplacementMoveParams{IngredientID: &old.ID, Quantity: s.OnHand.Neg(), UnitCost: &oldCost,
			BranchID: s.BranchID, UserID: &actor, ReplacementID: &repID}); err != nil {
			return err
		}
		if !to.IsZero() {
			if err := q.InsertReplacementMove(ctx, db.InsertReplacementMoveParams{IngredientID: &newID, Quantity: to, UnitCost: &newCost,
				BranchID: s.BranchID, UserID: &actor, ReplacementID: &repID}); err != nil {
				return err
			}
		}
		moved = append(moved, map[string]any{"branchId": s.BranchID, "old": s.OnHand, "new": to})
	}
	// Todas, también las de productos inactivos: si se reactivan, venden del artículo nuevo.
	for _, r := range p.recipes {
		to := override(req, "recipe:"+strconv.FormatInt(r.ID, 10), domain.ConvertRecipeQty(r.Quantity, r.UnitToBase, f))
		if !to.IsPositive() {
			return fmt.Errorf("%w: la receta de %s quedaría en cero; corrige su cantidad", domain.ErrValidation, r.Owner)
		}
		if err := q.SetRecipeItem(ctx, db.SetRecipeItemParams{ID: r.ID, IngredientID: newID, Quantity: to, UnitID: p.to.ID}); err != nil {
			return err
		}
	}
	for _, pr := range p.pres {
		if !pr.Active {
			continue
		}
		to := override(req, "presentation:"+strconv.FormatInt(pr.ID, 10), domain.ConvertQty(pr.QtyInBase, f))
		if !presentationFits(ctx, q, pr, p.to, to) {
			// No pasa al nuevo: queda archivado en el viejo, que sigue siendo válido en su unidad.
			if err := q.SetPresentationRaw(ctx, db.SetPresentationRawParams{ID: pr.ID, Name: pr.Name, QtyInBase: pr.QtyInBase,
				PurchaseUnitID: pr.PurchaseUnitID, IsApproximate: pr.IsApproximate, Active: false}); err != nil {
				return err
			}
			continue
		}
		var purchase *int16
		if pr.IsApproximate {
			purchase = pr.PurchaseUnitID
		}
		if _, err := q.CreatePresentation(ctx, db.CreatePresentationParams{IngredientID: newID, Name: pr.Name, QtyInBase: to,
			PurchaseUnitID: purchase, IsApproximate: pr.IsApproximate}); err != nil {
			return err
		}
	}
	if err := convertSideTables(ctx, q, old.ID, newID, f, func(string, any) error { return nil }); err != nil {
		return err
	}
	if err := q.SetArticleActive(ctx, db.SetArticleActiveParams{ID: old.ID, Active: false}); err != nil {
		return err
	}
	return recordAudit(ctx, q, domain.AuditEvent{
		Action: domain.AuditIngredientReplaced, EntityType: "ingredient", EntityID: old.ID,
		Before: map[string]any{"unit": p.from.Code, "name": oldName},
		After:  map[string]any{"newIngredientId": newID, "unit": p.to.Code, "factor": f, "replacementId": repID, "moved": moved},
	})
}

// retiredName es el nombre del artículo viejo de un reemplazo: «X (antes en g)», y si ya existe uno así
// (un segundo reemplazo de la misma unidad), «X (antes en g 2)», «… 3». Va bajo el candado de nombres.
func retiredName(ctx context.Context, q *db.Queries, name, unit string) (string, error) {
	for n := 1; ; n++ {
		suffix := " (antes en " + unit + ")"
		if n > 1 {
			suffix = fmt.Sprintf(" (antes en %s %d)", unit, n)
		}
		keep := []rune(name)
		if len(keep)+len([]rune(suffix)) > domain.MaxArticleName {
			keep = keep[:domain.MaxArticleName-len([]rune(suffix))]
		}
		candidate := strings.TrimSpace(string(keep)) + suffix
		taken, err := q.ArticleNameExists(ctx, candidate)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
}
