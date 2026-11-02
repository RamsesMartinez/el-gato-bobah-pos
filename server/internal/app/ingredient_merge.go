package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Unir insumos (spec 056): varios insumos que son lo mismo con otra marca pasan a uno, el destino.

// MergeTables dice qué hace la unión con cada tabla que apunta a un insumo. Una tabla nueva que no
// esté aquí hace tronar TestEveryTableThatPointsToAnArticleIsClassified; una clasificada «rechaza»
// sin las dos guardas de la 0104 hace tronar TestEveryRejectingTableHasTheMergeGuard.
var MergeTables = map[string]string{
	"stock_movements":             "traspaso por sucursal ligado a la unión; lo pasado no se toca; lo que llegue después al origen se redirige al destino",
	"stock_levels":                "recibe el traspaso; el origen queda en cero",
	"recipe_items":                "pasan al destino (se suman si la receta ya lo lleva); después, rechaza",
	"ingredient_presentations":    "las activas que cuadran pasan al destino con marca; las demás se quedan archivadas en el origen; después, rechaza",
	"ingredient_purchase_formats": "pasan al destino, convertidos; después, rechaza",
	"supplier_items":              "pasan al destino con el contenido aprendido convertido; después, rechaza",
	"expense_items":               "lo pasado se queda en el origen; uno por recibir bloquea; después, rechaza",
	"waste_records":               "lo pasado se queda en el origen; una pendiente bloquea; después, rechaza",
	"stock_count_lines":           "lo pasado se queda en el origen; un conteo abierto bloquea; después, rechaza",
	"ingredient_cost_corrections": "las pasadas se quedan y se leen desde el destino; después, rechaza",
	"ingredient_replacements":     "registra la unión (kind = merge)",
	"ingredient_distinct_pairs":   "se queda: un insumo unido ya no se propone",
}

// MergeSourceInput es un origen y, si se mide distinto que el destino, su equivalencia: cuánto del
// destino vale una unidad base del origen.
type MergeSourceInput struct {
	ID     int64            `json:"id"`
	Factor *decimal.Decimal `json:"factor"`
}

// MergeRequest es la unión que se pide. Al confirmar, Fingerprint es la de la última vista previa.
type MergeRequest struct {
	Sources     []MergeSourceInput `json:"sources"`
	Fingerprint string             `json:"fingerprint"`
}

// MergeArticle es un insumo de la unión, como lo dice la vista previa.
type MergeArticle struct {
	ID       int64           `json:"id"`
	Name     string          `json:"name"`
	BaseUnit string          `json:"baseUnit"`
	Factor   decimal.Decimal `json:"factor"`
}

// MergeStock es la existencia de un insumo en una sucursal, antes y después.
type MergeStock struct {
	BranchID      int64           `json:"branchId"`
	Branch        string          `json:"branch"`
	IngredientID  int64           `json:"ingredientId"`
	Name          string          `json:"name"`
	BaseUnit      string          `json:"baseUnit"`
	Before        decimal.Decimal `json:"before"`
	After         decimal.Decimal `json:"after"`
	NegativeAfter bool            `json:"negativeAfter"`
}

// MergeRecipe es un renglón de receta que pasa al destino. Merged: se sumó al renglón que la receta ya
// tenía del destino.
type MergeRecipe struct {
	Owner      string          `json:"owner"`
	SourceID   int64           `json:"sourceId"`
	BeforeQty  decimal.Decimal `json:"beforeQty"`
	BeforeUnit string          `json:"beforeUnit"`
	AfterQty   decimal.Decimal `json:"afterQty"`
	AfterUnit  string          `json:"afterUnit"`
	Merged     bool            `json:"merged"`
}

// MergePresentationView es lo que pasa con una presentación de un origen (move, archive, conflict).
type MergePresentationView struct {
	ID        int64           `json:"id"`
	SourceID  int64           `json:"sourceId"`
	Name      string          `json:"name"`
	Brand     string          `json:"brand"`
	QtyInBase decimal.Decimal `json:"qtyInBase"`
	BaseUnit  string          `json:"baseUnit"`
	Action    string          `json:"action"`
}

// MergePreview es lo que pasaría al unir; al confirmar, lo que pasó.
type MergePreview struct {
	Target          MergeArticle            `json:"target"`
	Sources         []MergeArticle          `json:"sources"`
	Stock           []MergeStock            `json:"stock"`
	Recipes         []MergeRecipe           `json:"recipes"`
	Presentations   []MergePresentationView `json:"presentations"`
	SupplierItems   int                     `json:"supplierItems"`
	PurchaseFormats int                     `json:"purchaseFormats"`
	CostBefore      decimal.Decimal         `json:"costBefore"`
	CostAfter       decimal.Decimal         `json:"costAfter"`
	MinStockBefore  *decimal.Decimal        `json:"minStockBefore"`
	MinStockAfter   *decimal.Decimal        `json:"minStockAfter"`
	Fingerprint     string                  `json:"fingerprint"`
}

// MergeIngredients une los orígenes en el destino. Con preview corre todo y lo revierte: lo que se ve
// es exactamente lo que se aplicaría. Al confirmar exige la huella de la última vista previa.
func (s *BackofficeService) MergeIngredients(ctx context.Context, targetID int64, req MergeRequest, preview bool) (MergePreview, error) {
	actor, ok := ActorUserFrom(ctx)
	if !ok {
		return MergePreview{}, fmt.Errorf("%w: unir necesita a la persona que lo hace", domain.ErrForbidden)
	}
	ids := make([]int64, len(req.Sources))
	for i, src := range req.Sources {
		ids[i] = src.ID
	}
	if err := domain.ValidMergeRequest(targetID, ids); err != nil {
		return MergePreview{}, err
	}
	var out MergePreview
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		m := &mergeRun{q: q, actor: actor, targetID: targetID, req: req}
		if err := m.lock(ctx); err != nil {
			return err
		}
		if err := m.check(ctx); err != nil {
			return err
		}
		if err := m.apply(ctx); err != nil {
			return err
		}
		out = m.out
		out.Fingerprint = domain.PreviewFingerprint(m.out)
		if preview {
			return errPreviewRollback
		}
		if req.Fingerprint != out.Fingerprint {
			return domain.ErrMergeStale
		}
		if m.conflict {
			return domain.ErrMergePresentationConflict
		}
		return m.audit(ctx)
	})
	if err != nil && !errors.Is(err, errPreviewRollback) {
		return MergePreview{}, err
	}
	return out, nil
}

type mergeRun struct {
	q        *db.Queries
	actor    int64
	targetID int64
	req      MergeRequest
	target   db.LockArticleNowaitRow
	sources  []db.LockArticleNowaitRow // en el orden de la solicitud
	factors  []decimal.Decimal
	units    map[int16]string
	stock    map[int64][]db.LockArticleStockNowaitRow
	conflict bool
	moved    map[int64][]map[string]any // traspaso por origen, para la bitácora
	out      MergePreview
}

// lock toma, en este orden y sin esperar a nadie más que al candado de captura: el candado de captura
// de la empresa (antes que cualquier fila, como ChangeUnit), los insumos ordenados por id (dos uniones
// a la vez no se cruzan) y sus existencias. Una venta, compra o merma en curso sobre cualquiera hace
// que la unión responda «vuelve a intentar» (research R2).
func (m *mergeRun) lock(ctx context.Context) error {
	if err := m.q.LockCompositionEdits(ctx); err != nil {
		return err
	}
	all := append([]int64{m.targetID}, sourceIDs(m.req)...)
	sorted := slices.Clone(all)
	slices.Sort(sorted)
	arts := make(map[int64]db.LockArticleNowaitRow, len(sorted))
	for _, id := range sorted {
		a, err := m.q.LockArticleNowait(ctx, id)
		if err != nil {
			return notFound(lockErr(err))
		}
		arts[id] = a
	}
	m.stock = make(map[int64][]db.LockArticleStockNowaitRow, len(sorted))
	for _, id := range sorted {
		rows, err := m.q.LockArticleStockNowait(ctx, &id)
		if err != nil {
			return lockErr(err)
		}
		m.stock[id] = rows
	}
	m.target = arts[m.targetID]
	for _, src := range m.req.Sources {
		m.sources = append(m.sources, arts[src.ID])
	}
	return nil
}

func sourceIDs(req MergeRequest) []int64 {
	ids := make([]int64, len(req.Sources))
	for i, s := range req.Sources {
		ids[i] = s.ID
	}
	return ids
}

// check rechaza lo que no se puede unir, diciendo qué falta y de cuál (FR-008).
func (m *mergeRun) check(ctx context.Context) error {
	blocked := func(format string, args ...any) error {
		return fmt.Errorf("%w: "+format, append([]any{domain.ErrMergeBlocked}, args...)...)
	}
	if !m.target.IsActive {
		return blocked("%s está archivado; reactívalo antes de unir en él", m.target.Name)
	}
	if m.target.IsPrep {
		return blocked("%s es un preparado; los preparados no se unen", m.target.Name)
	}
	m.units = map[int16]string{}
	unit := func(id int16) (string, error) {
		if c, ok := m.units[id]; ok {
			return c, nil
		}
		c, err := m.q.UnitCode(ctx, id)
		m.units[id] = c
		return c, err
	}
	if _, err := unit(m.target.BaseUnitID); err != nil {
		return err
	}
	for i, src := range m.sources {
		if target, err := m.q.MergeTargetOf(ctx, src.ID); err == nil {
			return blocked("%s ya se unió a %s", src.Name, target.Name)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if !src.IsActive {
			return blocked("%s está archivado; reactívalo antes de unirlo", src.Name)
		}
		if src.IsPrep {
			return blocked("%s es un preparado; los preparados no se unen", src.Name)
		}
		f, err := domain.MergeFactor(src.BaseUnitID == m.target.BaseUnitID, m.req.Sources[i].Factor)
		if err != nil {
			return fmt.Errorf("%w (%s)", err, src.Name)
		}
		m.factors = append(m.factors, f)
		if err := m.checkConversion(ctx, src, f); err != nil {
			return err
		}
		if _, err := unit(src.BaseUnitID); err != nil {
			return err
		}
		receipts, err := m.q.PendingReceiptsOf(ctx, &src.ID)
		if err != nil {
			return err
		}
		if receipts > 0 {
			return blocked("%s tiene %d compra(s) por recibir; recíbelas o quítalas antes", src.Name, receipts)
		}
		waste, err := m.q.PendingWasteOf(ctx, &src.ID)
		if err != nil {
			return err
		}
		if waste > 0 {
			return blocked("%s tiene %d merma(s) por aprobar; apruébalas o recházalas antes", src.Name, waste)
		}
	}
	for _, a := range append([]db.LockArticleNowaitRow{m.target}, m.sources...) {
		open, err := m.q.OpenCountHasArticle(ctx, a.ID)
		if err != nil {
			return err
		}
		if open {
			return blocked("un conteo abierto ya capturó %s; ciérralo o descártalo antes", a.Name)
		}
	}
	return nil
}

// maxMergeDepth es lo más larga que puede quedar una cadena de uniones (A en B, B en C…): el historial
// y la redirección la recorren hasta 16 niveles, y una más larga se cortaría sin avisar.
const maxMergeDepth = 15

// checkConversion rechaza, antes de escribir nada, la equivalencia que borraría algo o no cabría:
// existencia que redondea a cero (saldría del origen y no entraría al destino), un costo que queda en
// cero o desborda, y una cadena de uniones demasiado larga (revisión de código y de seguridad).
func (m *mergeRun) checkConversion(ctx context.Context, src db.LockArticleNowaitRow, f decimal.Decimal) error {
	sub, err := m.q.MergeSubtree(ctx, src.ID)
	if err != nil {
		return err
	}
	if sub.Depth+1 > maxMergeDepth {
		return fmt.Errorf("%w: %s ya junta demasiadas uniones; únelo en otro insumo", domain.ErrMergeBlocked, src.Name)
	}
	if !domain.ValidConvertedCost(sub.MaxUnitCost, domain.ConvertCost(sub.MaxUnitCost, f)) {
		return fmt.Errorf("%w: con esa equivalencia el costo de %s quedaría en cero o no cabría; revisa la equivalencia", domain.ErrValidation, src.Name)
	}
	for _, row := range m.stock[src.ID] {
		to := domain.ConvertQty(row.OnHand, f)
		if !row.OnHand.IsZero() && (to.IsZero() || !domain.ValidConvertedQty(to)) {
			return fmt.Errorf("%w: con esa equivalencia la existencia de %s quedaría en cero o no cabría; revisa la equivalencia", domain.ErrValidation, src.Name)
		}
	}
	return nil
}

// apply hace la unión dentro de la transacción y arma la vista previa con lo que hizo.
func (m *mergeRun) apply(ctx context.Context) error {
	q := m.q
	targetUnit := m.units[m.target.BaseUnitID]
	m.out = MergePreview{
		Target:  MergeArticle{ID: m.target.ID, Name: m.target.Name, BaseUnit: targetUnit, Factor: decimal.NewFromInt(1)},
		Sources: []MergeArticle{}, Stock: []MergeStock{}, Recipes: []MergeRecipe{}, Presentations: []MergePresentationView{},
		CostBefore: m.target.CurrentCost, MinStockBefore: m.target.MinStock,
	}
	branches, err := q.BranchNames(ctx)
	if err != nil {
		return err
	}
	branchName := make(map[int64]string, len(branches))
	for _, b := range branches {
		branchName[b.ID] = b.Name
	}
	targetAfter := map[int64]decimal.Decimal{}
	for _, row := range m.stock[m.targetID] {
		targetAfter[row.BranchID] = row.OnHand
	}
	m.moved = map[int64][]map[string]any{}

	for i, src := range m.sources {
		f := m.factors[i]
		m.out.Sources = append(m.out.Sources, MergeArticle{ID: src.ID, Name: src.Name, BaseUnit: m.units[src.BaseUnitID], Factor: f})
		m.moved[src.ID] = []map[string]any{}
		repID, err := q.InsertMerge(ctx, db.InsertMergeParams{OldIngredientID: src.ID, NewIngredientID: m.targetID, Factor: f, UserID: m.actor})
		if err != nil {
			return err
		}
		if err := m.moveRecipes(ctx, src, f, targetUnit); err != nil {
			return err
		}
		// El traspaso, sucursal por sucursal, con la sucursal explícita y ligado a la unión.
		srcCost := src.CurrentCost
		dstCost := domain.ConvertCost(srcCost, f)
		for _, row := range m.stock[src.ID] {
			to := domain.ConvertQty(row.OnHand, f)
			m.out.Stock = append(m.out.Stock, MergeStock{BranchID: row.BranchID, Branch: branchName[row.BranchID], IngredientID: src.ID,
				Name: src.Name, BaseUnit: m.units[src.BaseUnitID], Before: row.OnHand, After: decimal.Zero})
			if row.OnHand.IsZero() {
				continue
			}
			if err := q.InsertMergeMove(ctx, db.InsertMergeMoveParams{IngredientID: &src.ID, Quantity: row.OnHand.Neg(), UnitCost: &srcCost,
				BranchID: row.BranchID, UserID: &m.actor, ReplacementID: &repID}); err != nil {
				return err
			}
			if !to.IsZero() {
				if err := q.InsertMergeMove(ctx, db.InsertMergeMoveParams{IngredientID: &m.target.ID, Quantity: to, UnitCost: &dstCost,
					BranchID: row.BranchID, UserID: &m.actor, ReplacementID: &repID}); err != nil {
					return err
				}
			}
			targetAfter[row.BranchID] = targetAfter[row.BranchID].Add(to)
			m.moved[src.ID] = append(m.moved[src.ID], map[string]any{"branchId": row.BranchID, "from": row.OnHand, "to": to})
		}
		if err := m.movePresentations(ctx, src, f); err != nil {
			return err
		}
		formats, err := q.ArticlePurchaseFormats(ctx, src.ID)
		if err != nil {
			return err
		}
		for _, pf := range formats {
			qty := domain.ConvertQty(pf.QtyInBase, f)
			if !qty.IsPositive() || !domain.ValidConvertedQty(qty) {
				return fmt.Errorf("%w: con esa equivalencia un formato de compra de %s quedaría en cero o no cabría", domain.ErrValidation, src.Name)
			}
			if err := q.SetPurchaseFormatQty(ctx, db.SetPurchaseFormatQtyParams{ID: pf.ID, IngredientID: m.targetID, QtyInBase: qty}); err != nil {
				return err
			}
		}
		m.out.PurchaseFormats += len(formats)
		items, err := q.ArticleSupplierItems(ctx, &src.ID)
		if err != nil {
			return err
		}
		for _, si := range items {
			var pack *decimal.Decimal
			if si.PackQtyInBase != nil {
				v := domain.ConvertQty(*si.PackQtyInBase, f)
				if !v.IsPositive() || !domain.ValidConvertedQty(v) {
					return fmt.Errorf("%w: con esa equivalencia lo aprendido de un ticket de %s quedaría en cero o no cabría", domain.ErrValidation, src.Name)
				}
				pack = &v
			}
			if err := q.SetSupplierItem(ctx, db.SetSupplierItemParams{ID: si.ID, IngredientID: &m.target.ID, PackQtyInBase: pack}); err != nil {
				return err
			}
		}
		m.out.SupplierItems += len(items)
	}

	// La existencia del destino en cada sucursal donde había algo, de él o de un origen.
	branchIDs := make([]int64, 0, len(targetAfter))
	before := map[int64]decimal.Decimal{}
	for _, row := range m.stock[m.targetID] {
		before[row.BranchID] = row.OnHand
	}
	for id := range targetAfter {
		branchIDs = append(branchIDs, id)
	}
	slices.Sort(branchIDs)
	for _, id := range branchIDs {
		after := targetAfter[id]
		m.out.Stock = append(m.out.Stock, MergeStock{BranchID: id, Branch: branchName[id], IngredientID: m.targetID, Name: m.target.Name,
			BaseUnit: targetUnit, Before: before[id], After: after, NegativeAfter: after.IsNegative()})
	}

	if err := m.mergeCost(ctx); err != nil {
		return err
	}
	for _, src := range m.sources {
		if err := q.SetArticleActive(ctx, db.SetArticleActiveParams{ID: src.ID, Active: false}); err != nil {
			return err
		}
	}
	// Las recetas cambiaron de insumo y el costo del destino pudo cambiar: lo que las lleva se recostea.
	return recomputeCosts(ctx, q)
}

// moveRecipes pasa al destino todos los renglones de receta del origen (de productos activos o no, de
// extras y de preparados): ninguna receta queda apuntando a un insumo unido (FR-004).
func (m *mergeRun) moveRecipes(ctx context.Context, src db.LockArticleNowaitRow, f decimal.Decimal, targetUnit string) error {
	rows, err := m.q.ArticleRecipeItems(ctx, src.ID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		var existing *domain.MergeRecipeLine
		var existingID int64
		e, err := m.q.RecipeItemOfIngredient(ctx, db.RecipeItemOfIngredientParams{RecipeID: r.RecipeID, IngredientID: m.targetID})
		switch {
		case err == nil:
			existing = &domain.MergeRecipeLine{Qty: e.Quantity, UnitToBase: e.UnitToBase, UnitID: e.UnitID}
			existingID = e.ID
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		plan, err := domain.PlanMergedRecipe(domain.MergeRecipeLine{Qty: r.Quantity, UnitToBase: r.UnitToBase, UnitID: r.UnitID},
			existing, f, m.target.BaseUnitID)
		if err != nil {
			return fmt.Errorf("%w (receta de %s)", err, r.Owner)
		}
		afterUnit := targetUnit
		if plan.UnitID == r.UnitID {
			afterUnit = r.UnitCode
		}
		if plan.Sum {
			if err := m.q.DeleteRecipeItem(ctx, r.ID); err != nil {
				return err
			}
			if err := m.q.SetRecipeItem(ctx, db.SetRecipeItemParams{ID: existingID, IngredientID: m.targetID, Quantity: plan.Qty, UnitID: plan.UnitID}); err != nil {
				return err
			}
		} else if err := m.q.SetRecipeItem(ctx, db.SetRecipeItemParams{ID: r.ID, IngredientID: m.targetID, Quantity: plan.Qty, UnitID: plan.UnitID}); err != nil {
			return err
		}
		m.out.Recipes = append(m.out.Recipes, MergeRecipe{Owner: r.Owner, SourceID: src.ID, BeforeQty: r.Quantity, BeforeUnit: r.UnitCode,
			AfterQty: plan.Qty, AfterUnit: afterUnit, Merged: plan.Sum})
	}
	return nil
}

// movePresentations pasa al destino las presentaciones activas del origen, con marca (research R5).
func (m *mergeRun) movePresentations(ctx context.Context, src db.LockArticleNowaitRow, f decimal.Decimal) error {
	targetUnit, err := m.q.GetUnit(ctx, m.target.BaseUnitID)
	if err != nil {
		return err
	}
	current, err := m.q.ListPresentations(ctx, &m.target.ID)
	if err != nil {
		return err
	}
	keys := []domain.PresentationKey{}
	for _, p := range current {
		if p.Active {
			keys = append(keys, domain.PresentationKey{Name: p.Name, Brand: textoDe(p.Brand), QtyInBase: p.QtyInBase})
		}
	}
	own, err := m.q.ListPresentations(ctx, &src.ID)
	if err != nil {
		return err
	}
	in := []domain.MergePresentation{}
	byID := map[int64]db.ListPresentationsRow{}
	for _, p := range own {
		if !p.Active {
			continue
		}
		qty := domain.ConvertQty(p.QtyInBase, f)
		fits := true
		if src.BaseUnitID != m.target.BaseUnitID {
			fits = presentationFits(ctx, m.q, p, targetUnit, qty)
		}
		in = append(in, domain.MergePresentation{ID: p.ID, Name: p.Name, Brand: textoDe(p.Brand), QtyInBase: qty, Fits: fits})
		byID[p.ID] = p
	}
	for _, d := range domain.PlanMergedPresentations(keys, in, src.Name) {
		m.out.Presentations = append(m.out.Presentations, MergePresentationView{ID: d.ID, SourceID: src.ID, Name: d.Name, Brand: d.Brand,
			QtyInBase: d.QtyInBase, BaseUnit: targetUnit.Code, Action: d.Action})
		switch d.Action {
		case domain.MergeMove:
			brand := d.Brand
			// Una presentación que otra persona creó en el destino mientras tanto choca con el índice
			// único: 409 «ya hay un empaque que se llama así», no 500.
			if err := m.q.MovePresentation(ctx, db.MovePresentationParams{ID: d.ID, IngredientID: m.targetID, QtyInBase: d.QtyInBase, Brand: &brand}); err != nil {
				return presentationErr(err)
			}
		case domain.MergeArchive:
			if err := m.q.ArchivePresentation(ctx, d.ID); err != nil {
				return err
			}
		case domain.MergeConflict:
			m.conflict = true
		}
	}
	return nil
}

// mergeCost fija el costo y el mínimo del destino con las reglas de domain (FR-012).
func (m *mergeRun) mergeCost(ctx context.Context) error {
	all := append([]db.LockArticleNowaitRow{m.target}, m.sources...)
	factors := append([]decimal.Decimal{decimal.NewFromInt(1)}, m.factors...)
	cands := make([]domain.CostCandidate, len(all))
	source := map[int64]string{}
	for i, a := range all {
		c := domain.CostCandidate{ID: a.ID, Factor: factors[i], Current: a.CurrentCost}
		last, err := m.q.LastPurchaseCost(ctx, &a.ID)
		switch {
		case err == nil:
			c.LastPurchaseAt, c.LastPurchaseCost = &last.CreatedAt, &last.UnitCost
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		cands[i] = c
		source[a.ID] = a.CostSource
	}
	cost, from := domain.MergedCost(cands)
	costSource := source[from]
	original := cost
	for _, c := range cands {
		if c.ID != from {
			continue
		}
		original = c.Current
		if c.LastPurchaseAt != nil {
			costSource, original = "compra", *c.LastPurchaseCost
		}
	}
	if !domain.ValidConvertedCost(original, cost) {
		return fmt.Errorf("%w: con esa equivalencia el costo no cabe; revisa la equivalencia", domain.ErrValidation)
	}
	mins := make([]domain.MinStockSource, len(m.sources))
	for i, src := range m.sources {
		mins[i] = domain.MinStockSource{Min: src.MinStock, Factor: m.factors[i]}
	}
	minStock := domain.MergeMinStock(m.target.MinStock, mins)
	m.out.CostAfter, m.out.MinStockAfter = cost, minStock
	return m.q.SetArticleMergedCost(ctx, db.SetArticleMergedCostParams{ID: m.targetID, CurrentCost: cost, CostSource: db.CostSource(costSource), MinStock: minStock})
}

// audit deja la unión en la bitácora del destino y de cada origen, en la misma transacción.
func (m *mergeRun) audit(ctx context.Context) error {
	sources := make([]map[string]any, len(m.sources))
	for i, src := range m.sources {
		sources[i] = map[string]any{"id": src.ID, "name": src.Name, "factor": m.factors[i], "moved": m.moved[src.ID]}
	}
	if err := recordAudit(ctx, m.q, domain.AuditEvent{
		Action: domain.AuditIngredientMerged, EntityType: "ingredient", EntityID: m.targetID,
		Before: map[string]any{"cost": m.out.CostBefore, "minStock": m.out.MinStockBefore},
		After: map[string]any{"sources": sources, "cost": m.out.CostAfter, "minStock": m.out.MinStockAfter,
			"recipes": len(m.out.Recipes), "presentations": m.out.Presentations, "supplierItems": m.out.SupplierItems,
			"purchaseFormats": m.out.PurchaseFormats},
	}); err != nil {
		return err
	}
	for i, src := range m.sources {
		if err := recordAudit(ctx, m.q, domain.AuditEvent{
			Action: domain.AuditIngredientMergedInto, EntityType: "ingredient", EntityID: src.ID,
			Before: map[string]any{"active": true},
			After:  map[string]any{"active": false, "targetId": m.targetID, "target": m.target.Name, "factor": m.factors[i], "moved": m.moved[src.ID]},
		}); err != nil {
			return err
		}
	}
	return nil
}

// maxSimilar es cuántos parecidos se ofrecen al dar de alta un insumo (FR-017).
const maxSimilar = 5

// maxSimilarQuery es el largo del nombre que se compara: el de un insumo, con margen.
const maxSimilarQuery = 80

// maxDuplicateCandidates es el tope de insumos activos para buscar posibles duplicados. Comparar todos
// contra todos es cuadrático: medido, 2,000 nombres largos cuestan ~0.9 s de CPU en un núcleo, y la VM
// de producción atiende a todas las empresas (revisión de seguridad, M2). Un negocio de comida tiene
// cientos; más que esto pide otra forma de buscarlos.
const maxDuplicateCandidates = 3000

// DuplicateItem es un insumo de un grupo de posibles duplicados.
type DuplicateItem struct {
	ID           int64           `json:"id"`
	Name         string          `json:"name"`
	BaseUnitID   int16           `json:"baseUnitId"`
	BaseUnitCode string          `json:"baseUnitCode"`
	OnHand       decimal.Decimal `json:"onHand"`
}

// DuplicateGroup son insumos activos con nombres parecidos.
type DuplicateGroup struct {
	Items []DuplicateItem `json:"items"`
}

// candidates son los insumos que se pueden proponer, con su vista para la pantalla.
func (s *BackofficeService) candidates(ctx context.Context, q *db.Queries) ([]domain.NamedItem, map[int64]DuplicateItem, error) {
	rows, err := q.MergeCandidates(ctx)
	if err != nil {
		return nil, nil, err
	}
	named := make([]domain.NamedItem, len(rows))
	views := make(map[int64]DuplicateItem, len(rows))
	for i, r := range rows {
		named[i] = domain.NamedItem{ID: r.ID, Name: r.Name}
		views[r.ID] = DuplicateItem{ID: r.ID, Name: r.Name, BaseUnitID: r.BaseUnitID, BaseUnitCode: r.BaseUnitCode, OnHand: r.OnHand}
	}
	return named, views, nil
}

// DuplicateGroups son los grupos de posibles duplicados de la empresa, sin los pares descartados (FR-021).
// Nunca nil.
func (s *BackofficeService) DuplicateGroups(ctx context.Context) ([]DuplicateGroup, error) {
	q := s.store.QC(ctx)
	named, views, err := s.candidates(ctx, q)
	if err != nil {
		return nil, err
	}
	if len(named) > maxDuplicateCandidates {
		return nil, fmt.Errorf("%w: hay más de %d insumos activos; busca los duplicados desde la ficha de cada uno", domain.ErrValidation, maxDuplicateCandidates)
	}
	pairs, err := q.DistinctPairs(ctx)
	if err != nil {
		return nil, err
	}
	distinct := make([]domain.DistinctPair, len(pairs))
	for i, p := range pairs {
		distinct[i] = domain.DistinctPair{Low: p.LowIngredientID, High: p.HighIngredientID}
	}
	groups, done := domain.DuplicateGroups(named, distinct, func() bool { return ctx.Err() != nil })
	if !done {
		return nil, ctx.Err()
	}
	out := []DuplicateGroup{}
	for _, g := range groups {
		items := make([]DuplicateItem, len(g))
		for i, it := range g {
			items[i] = views[it.ID]
		}
		out = append(out, DuplicateGroup{Items: items})
	}
	return out, nil
}

// MarkDistinct guarda que dos insumos no son lo mismo: el par ya no se propone (FR-023).
func (s *BackofficeService) MarkDistinct(ctx context.Context, a, b int64) error {
	actor, ok := ActorUserFrom(ctx)
	if !ok {
		return fmt.Errorf("%w: marcar necesita a la persona que lo hace", domain.ErrForbidden)
	}
	if a <= 0 || b <= 0 || a == b {
		return fmt.Errorf("%w: elige dos insumos distintos", domain.ErrValidation)
	}
	err := s.store.QC(ctx).InsertDistinctPair(ctx, db.InsertDistinctPairParams{LowIngredientID: min(a, b), HighIngredientID: max(a, b), UserID: actor})
	if isForeignKeyViolation(err) {
		// La llave compuesta por empresa: un insumo de otra empresa no existe para ésta.
		return domain.ErrNotFound
	}
	return err
}

// SimilarArticles son los insumos activos con nombre parecido a uno que se va a dar de alta (FR-017).
// Nunca nil.
func (s *BackofficeService) SimilarArticles(ctx context.Context, name string) ([]DuplicateItem, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > maxSimilarQuery {
		return nil, fmt.Errorf("%w: el nombre va de 1 a %d letras", domain.ErrValidation, maxSimilarQuery)
	}
	named, views, err := s.candidates(ctx, s.store.QC(ctx))
	if err != nil {
		return nil, err
	}
	out := []DuplicateItem{}
	for _, it := range domain.SimilarNames(name, named, maxSimilar) {
		out = append(out, views[it.ID])
	}
	return out, nil
}
