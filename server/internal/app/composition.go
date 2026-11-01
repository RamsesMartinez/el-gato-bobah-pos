package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// loadStockGraph carga la composición de la empresa de la sesión (RLS) para expandir ventas.
//
// ponytail: carga el catálogo completo en cada venta —cinco consultas de unos cientos de filas—.
// Si el catálogo crece a miles o la venta se vuelve lenta, se carga solo lo alcanzable desde los
// productos y opciones del pedido.
func loadStockGraph(ctx context.Context, q *db.Queries) (domain.StockGraph, error) {
	g := domain.StockGraph{
		Ingredients:   map[int64]domain.StockIngredient{},
		Recipes:       map[int64][]domain.RecipeLine{},
		Products:      map[int64]domain.StockProduct{},
		ComboDefaults: map[int64][]domain.ComboDefault{},
		Options:       map[int64]domain.StockOption{},
	}
	ings, err := q.ListIngredientsForCosting(ctx)
	if err != nil {
		return g, err
	}
	for _, r := range ings {
		in := domain.StockIngredient{ID: r.ID, IsPrep: r.IsPrep, RecipeID: r.RecipeID}
		if r.YieldQty != nil {
			in.YieldQty = *r.YieldQty
		}
		g.Ingredients[r.ID] = in
	}
	items, err := q.ListRecipeItemsForCosting(ctx)
	if err != nil {
		return g, err
	}
	for _, r := range items {
		g.Recipes[r.RecipeID] = append(g.Recipes[r.RecipeID], domain.RecipeLine{IngredientID: r.IngredientID, QtyBase: r.QtyBase})
	}
	prods, err := q.ListProductsForStock(ctx)
	if err != nil {
		return g, err
	}
	for _, r := range prods {
		g.Products[r.ID] = domain.StockProduct{ID: r.ID, IsCombo: r.Type == db.ProductTypeCombo, TrackStock: r.TrackStock, RecipeID: r.RecipeID}
	}
	slots, err := q.ListComboSlotDefaultsForCosting(ctx)
	if err != nil {
		return g, err
	}
	for _, r := range slots {
		g.ComboDefaults[r.ComboID] = append(g.ComboDefaults[r.ComboID], domain.ComboDefault{ProductID: r.ProductID, MinSelect: int(r.MinSelect)})
	}
	opts, err := q.ListModifierOptionsForCosting(ctx)
	if err != nil {
		return g, err
	}
	for _, r := range opts {
		g.Options[r.ID] = domain.StockOption{ID: r.ID, RecipeID: r.RecipeID, LinkedProductID: r.LinkedProductID}
	}
	return g, nil
}

// CompositionKind dice de qué es la composición: un producto o un extra.
type CompositionKind string

const (
	CompositionOfProduct CompositionKind = "product"
	CompositionOfOption  CompositionKind = "option"
	// CompositionOfIngredient es un insumo que se prepara en el local con otros insumos.
	CompositionOfIngredient CompositionKind = "ingredient"
)

// CompositionItemView es un renglón de la composición, para la hoja de receta.
type CompositionItemView struct {
	IngredientID   int64           `json:"ingredientId"`
	IngredientName string          `json:"ingredientName"`
	Quantity       decimal.Decimal `json:"quantity"`
	UnitID         int16           `json:"unitId"`
	UnitCode       string          `json:"unitCode"`
}

// CompositionComponentView es un producto dentro de un paquete.
type CompositionComponentView struct {
	ProductID   int64  `json:"productId"`
	ProductName string `json:"productName"`
	Quantity    int    `json:"quantity"`
}

// CompositionView es lo que lleva un producto o un extra y si está confirmado.
type CompositionView struct {
	// Status: "" sin capturar, "estimated" o "confirmed".
	Status            string                     `json:"status"`
	ConfirmedBy       string                     `json:"confirmedBy,omitempty"`
	ConfirmedAt       *time.Time                 `json:"confirmedAt,omitempty"`
	LinkedProductID   *int64                     `json:"linkedProductId,omitempty"`
	LinkedProductName string                     `json:"linkedProductName,omitempty"`
	Items             []CompositionItemView      `json:"items"`
	Components        []CompositionComponentView `json:"components"`
	// Yield es cuánto rinde un insumo preparado, en su unidad base (YieldUnitCode).
	Yield         *decimal.Decimal `json:"yield,omitempty"`
	YieldUnitCode string           `json:"yieldUnitCode,omitempty"`
	// Editable es falso en un paquete que deja elegir entre productos: capturarlo lo borraría.
	Editable bool   `json:"editable"`
	Reason   string `json:"reason,omitempty"`
	// OwnStock: el producto todavía cuenta su propia existencia (ProductOnHand). Ya no se ofrece como
	// opción; al elegir otra tarjeta, esa existencia se traslada o se cierra (spec 037).
	OwnStock      bool            `json:"ownStock"`
	ProductOnHand decimal.Decimal `json:"productOnHand"`
	// OwnStockBranches: en cuántas sucursales hay existencia propia distinta de cero.
	OwnStockBranches int `json:"ownStockBranches"`
	// Stamp es cuándo se guardó por última vez; la pantalla lo devuelve al guardar (BasedOn) para
	// saber si otra persona la cambió mientras se editaba.
	Stamp string `json:"stamp"`
	// SameName son los extras que se llaman igual en otro grupo: la pantalla ofrece guardarles la
	// misma receta.
	SameName []SameNameOption `json:"sameName"`
}

// SameNameOption es un extra que se llama igual que otro, en otro grupo.
type SameNameOption struct {
	ID    int64  `json:"id"`
	Group string `json:"group"`
	// Stamp vuelve en AlsoBasedOn: copiarle la receta también pisaría lo que alguien le guardó.
	Stamp string `json:"stamp"`
	// Today es lo que lleva hoy, en una línea: la pantalla lo muestra junto a la casilla para que
	// copiarle otra receta no esconda que es otra cosa.
	Today string `json:"today"`
}

// CompositionInputItem es un renglón capturado.
type CompositionInputItem struct {
	IngredientID int64 `json:"ingredientId"`
	// NewArticle: un insumo que se da de alta con esta receta (spec 042, FR-003): nace en la misma
	// transacción, así que cancelar o fallar el guardado no deja nada creado.
	NewArticle *NewArticle     `json:"newArticle"`
	Quantity   decimal.Decimal `json:"quantity"`
	UnitID     int16           `json:"unitId"`
}

// CompositionComponent es un producto capturado dentro de un paquete.
type CompositionComponent struct {
	ProductID int64 `json:"productId"`
	Quantity  int   `json:"quantity"`
}

// CompositionRequest es lo que se captura: insumos, el producto que es (solo un extra) o los
// productos que lleva (un paquete). Vacía quita la composición.
type CompositionRequest struct {
	Items           []CompositionInputItem `json:"items"`
	LinkedProductID *int64                 `json:"linkedProductId"`
	Components      []CompositionComponent `json:"components"`
	// Yield: cuánto rinde, solo para un insumo preparado y en su unidad base.
	Yield *decimal.Decimal `json:"yield"`
	// AlsoOptionIDs: extras que se llaman igual y reciben la misma receta.
	AlsoOptionIDs []int64 `json:"alsoOptionIds"`
	// BasedOn es el Stamp de la receta que se abrió. Si ya no coincide, otra persona la guardó
	// mientras se editaba: se rechaza en lugar de pisar su cambio. Nil = sin esa revisión.
	BasedOn *string `json:"basedOn"`
	// AlsoBasedOn es el Stamp de cada extra de AlsoOptionIDs, con la misma regla que BasedOn.
	AlsoBasedOn map[int64]string `json:"alsoBasedOn"`
	// Mode es la tarjeta que se eligió: "items" (se prepara aquí o se compra hecho), "package",
	// "product" (un extra que es otro producto) o "nothing" («No se cuenta»). Vacío = como antes de
	// la spec 037, sin revisar que el contenido corresponda a la tarjeta.
	Mode string `json:"mode"`
	// ActualStock, SeenProductOnHand y SeenIngredientOnHand son «¿Cuántas tienes de verdad?» al salir
	// de la existencia propia hacia un insumo que ya tiene existencia (FR-014a): la cifra elegida y
	// las que mostró la pantalla al preguntar.
	ActualStock          *decimal.Decimal `json:"actualStock"`
	SeenProductOnHand    *decimal.Decimal `json:"seenProductOnHand"`
	SeenIngredientOnHand *decimal.Decimal `json:"seenIngredientOnHand"`
}

// Composition devuelve lo que lleva un producto o un extra.
func (s *AdminService) Composition(ctx context.Context, kind CompositionKind, id int64) (CompositionView, error) {
	q := s.store.QC(ctx)
	v := CompositionView{Items: []CompositionItemView{}, Components: []CompositionComponentView{}, SameName: []SameNameOption{}, Editable: true}
	var recipe *int64
	var status, by *string
	var at pgtype.Timestamptz
	switch kind {
	case CompositionOfProduct:
		p, err := q.GetProductComposition(ctx, id)
		if err != nil {
			return v, notFound(err)
		}
		recipe, status, by, at = p.RecipeID, p.CompositionStatus, p.ConfirmedByName, p.CompositionConfirmedAt
		if p.TrackStock {
			oh, err := q.GetProductOnHand(ctx, &id)
			if err != nil {
				return v, err
			}
			v.OwnStock, v.ProductOnHand, v.OwnStockBranches = true, oh.OnHand, int(oh.Branches)
		}
		if p.Type == db.ProductTypeCombo {
			choices, err := q.PackageHasChoices(ctx, id)
			if err != nil {
				return v, err
			}
			if choices {
				v.Editable, v.Reason = false, "package_choices"
			}
			comps, err := q.ListPackageComponents(ctx, id)
			if err != nil {
				return v, err
			}
			for _, c := range comps {
				v.Components = append(v.Components, CompositionComponentView{
					ProductID: c.ProductID, ProductName: c.ProductName, Quantity: max(int(c.MinSelect), 1),
				})
			}
		}
	case CompositionOfOption:
		o, err := q.GetOptionComposition(ctx, id)
		if err != nil {
			return v, notFound(err)
		}
		recipe, status, by, at = o.RecipeID, o.CompositionStatus, o.ConfirmedByName, o.CompositionConfirmedAt
		v.LinkedProductID = o.LinkedProductID
		if o.LinkedProductName != nil {
			v.LinkedProductName = *o.LinkedProductName
		}
		twins, err := q.ListSameNameOptions(ctx, id)
		if err != nil {
			return v, err
		}
		for _, t := range twins {
			v.SameName = append(v.SameName, SameNameOption{ID: t.ID, Group: t.GroupName, Stamp: stampOf(t.CompositionConfirmedAt),
				Today: domain.RecipeToday(t.Status, t.LinkedName, t.Items)})
		}
	case CompositionOfIngredient:
		in, err := q.GetIngredientComposition(ctx, id)
		if err != nil {
			return v, notFound(err)
		}
		recipe, status, by, at = in.RecipeID, in.CompositionStatus, in.ConfirmedByName, in.CompositionConfirmedAt
		v.YieldUnitCode = in.BaseUnitCode
		if in.IsPrep {
			v.Yield = in.YieldQty
		}
	default:
		return v, domain.ErrValidation
	}
	if status != nil {
		v.Status = *status
	}
	if by != nil {
		v.ConfirmedBy = *by
	}
	if at.Valid {
		v.ConfirmedAt = &at.Time
	}
	v.Stamp = stampOf(at)
	if recipe != nil {
		items, err := q.ListCompositionItems(ctx, *recipe)
		if err != nil {
			return v, err
		}
		for _, it := range items {
			v.Items = append(v.Items, CompositionItemView(it))
		}
	}
	return v, nil
}

// SaveComposition guarda lo que lleva un producto o un extra. Quien lo captura lo confirma: es una
// persona decidiendo, no un estimado. Vacía quita la composición y deja de descontar.
//
// Un producto se vuelve paquete al capturarle productos, y deja de serlo al capturarle insumos: el
// tipo sigue a la composición, para que nadie tenga que cambiarlo aparte. El POS vende igual un
// paquete que un producto suelto; solo cambia lo que sale del almacén.
func (s *AdminService) SaveComposition(ctx context.Context, kind CompositionKind, id int64, req CompositionRequest, actor int64) error {
	if kind == CompositionOfIngredient {
		return s.savePreparedIngredient(ctx, id, req, actor)
	}
	if kind != CompositionOfProduct && kind != CompositionOfOption {
		return domain.ErrValidation
	}
	if req.Yield != nil {
		return fmt.Errorf("%w: solo un insumo preparado lleva rendimiento", domain.ErrValidation)
	}
	if kind == CompositionOfProduct && req.LinkedProductID != nil {
		return fmt.Errorf("%w: un producto no se liga a otro; un paquete se arma con sus productos", domain.ErrValidation)
	}
	if kind == CompositionOfOption && len(req.Components) > 0 {
		return fmt.Errorf("%w: un extra que lleva varios productos se liga a un paquete", domain.ErrValidation)
	}
	if err := validateMode(req); err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockCompositionEdits(ctx); err != nil {
			return err
		}
		if err := checkNotStale(ctx, q, kind, id, req.BasedOn); err != nil {
			return err
		}
		if err := resolveNewArticles(ctx, q, req.Items); err != nil {
			return err
		}
		names, err := validateComposition(ctx, q, kind, id, req)
		if err != nil {
			return err
		}
		if err := validateTwins(ctx, q, kind, id, req.AlsoOptionIDs); err != nil {
			return err
		}
		var change *modeChangeAudit
		if kind == CompositionOfProduct {
			if change, err = applyModeChange(ctx, q, id, req, actor); err != nil {
				return err
			}
		}
		if req.BasedOn != nil {
			for _, twin := range req.AlsoOptionIDs {
				stamp := req.AlsoBasedOn[twin]
				if err := checkNotStale(ctx, q, kind, twin, &stamp); err != nil {
					return err
				}
			}
		}
		recipe, err := createRecipe(ctx, q, req.Items)
		if err != nil {
			return err
		}
		var status *string
		var by *int64
		// «No se cuenta» queda confirmado en el mismo guardado: en dos pasos, un corte entre los dos
		// dejaba el producto como «nunca decidido».
		if recipe != nil || req.LinkedProductID != nil || len(req.Components) > 0 || req.Mode == "nothing" {
			confirmed := "confirmed"
			status, by = &confirmed, &actor
		}
		var n int64
		if kind == CompositionOfOption {
			n, err = q.SetOptionComposition(ctx, db.SetOptionCompositionParams{
				ID: id, RecipeID: recipe, LinkedProductID: req.LinkedProductID, Status: status, ConfirmedBy: by,
			})
			// Cada gemelo con su propia copia de la receta: `recipe_id` es único por extra.
			for _, twin := range req.AlsoOptionIDs {
				if err != nil {
					break
				}
				var twinRecipe *int64
				if twinRecipe, err = createRecipe(ctx, q, req.Items); err != nil {
					break
				}
				_, err = q.SetOptionComposition(ctx, db.SetOptionCompositionParams{
					ID: twin, RecipeID: twinRecipe, LinkedProductID: req.LinkedProductID, Status: status, ConfirmedBy: by,
				})
			}
		} else {
			if err := q.DeletePackageSlots(ctx, id); err != nil {
				return err
			}
			for i, c := range req.Components {
				slot, err := q.InsertPackageSlot(ctx, db.InsertPackageSlotParams{
					ComboID: id, Name: names[c.ProductID], Quantity: int16(c.Quantity), Position: int32(i),
				})
				if err != nil {
					return err
				}
				if err := q.InsertPackageSlotProduct(ctx, db.InsertPackageSlotProductParams{SlotID: slot, ProductID: c.ProductID}); err != nil {
					return err
				}
			}
			typ := db.ProductTypeSimple
			if len(req.Components) > 0 {
				typ = db.ProductTypeCombo
			}
			n, err = q.SetProductComposition(ctx, db.SetProductCompositionParams{
				ID: id, ProductType: typ, RecipeID: recipe, Status: status, ConfirmedBy: by,
			})
		}
		if err == nil && n == 0 {
			return domain.ErrNotFound
		}
		if err == nil && change != nil {
			err = change.record(ctx, q, id, actor)
		}
		if err != nil {
			return err
		}
		return recomputeCosts(ctx, q)
	})
}

// validateMode revisa que lo que se guarda corresponda a la tarjeta elegida (FR-014b): «Se prepara
// aquí» o «Es un combo» sin lo que llevan se guardaría como «nunca decidido», que no es lo que se
// eligió.
func validateMode(req CompositionRequest) error {
	empty := len(req.Items) == 0 && len(req.Components) == 0 && req.LinkedProductID == nil
	switch req.Mode {
	case "":
		return nil
	case "items":
		if len(req.Items) == 0 {
			return fmt.Errorf("%w: agrega al menos un insumo", domain.ErrValidation)
		}
	case "package":
		if len(req.Components) == 0 {
			return fmt.Errorf("%w: agrega al menos un producto al combo", domain.ErrValidation)
		}
	case "product":
		if req.LinkedProductID == nil {
			return fmt.Errorf("%w: elige el producto", domain.ErrValidation)
		}
	case "nothing":
		if !empty {
			return fmt.Errorf("%w: «No se cuenta» no lleva nada", domain.ErrValidation)
		}
	default:
		return fmt.Errorf("%w: modo desconocido (%q)", domain.ErrValidation, req.Mode)
	}
	return nil
}

// modeChangeAudit es el antes y el después de qué resta un producto.
type modeChangeAudit struct{ before, after domain.ConsumptionMode }

func (m modeChangeAudit) record(ctx context.Context, q *db.Queries, id, actor int64) error {
	if m.before == m.after {
		return nil
	}
	return auditEntity(ctx, q, domain.AuditProductConsumptionMode, "product", id, nil, &actor,
		map[string]any{"mode": m.before}, map[string]any{"mode": m.after})
}

// targetMode es el modo en que queda un producto con lo que se va a guardar.
func targetMode(req CompositionRequest) domain.ConsumptionMode {
	switch {
	case len(req.Components) > 0:
		return domain.ModePackage
	case len(req.Items) > 0:
		return domain.ModeRecipe
	case req.Mode == "nothing":
		return domain.ModeNone
	}
	return domain.ModeUndecided
}

// applyModeChange sale de la existencia propia si hace falta (research R6): bloquea las existencias
// del producto y del insumo, decide con domain.PlanModeChange, inserta los ajustes y apaga la
// existencia propia antes de que se guarde la receta (el check de products no deja las dos a la
// vez). Devuelve el antes y el después para la bitácora.
func applyModeChange(ctx context.Context, q *db.Queries, id int64, req CompositionRequest, actor int64) (*modeChangeAudit, error) {
	p, err := q.GetProductComposition(ctx, id)
	if err != nil {
		return nil, notFound(err)
	}
	hasItems := false
	if p.RecipeID != nil {
		items, err := q.ListCompositionItems(ctx, *p.RecipeID)
		if err != nil {
			return nil, err
		}
		hasItems = len(items) > 0
	}
	status := ""
	if p.CompositionStatus != nil {
		status = *p.CompositionStatus
	}
	before := domain.ModeOf(domain.ProductModeFacts{TrackStock: p.TrackStock, HasRecipeItems: hasItems,
		IsPackage: p.Type == db.ProductTypeCombo, CompositionStatus: status})
	audit := &modeChangeAudit{before: before, after: targetMode(req)}
	if before != domain.ModeOwnStock {
		return audit, nil
	}
	// Salir de la existencia propia mueve existencias sin vuelta atrás: tiene que venir de una
	// tarjeta elegida, no de un guardado viejo o de un script que mande el cuerpo vacío.
	if req.Mode == "" {
		return nil, fmt.Errorf("%w: elige qué resta este producto", domain.ErrValidation)
	}

	target := domain.ModeTarget{Kind: domain.TargetNothing}
	switch {
	case len(req.Components) > 0:
		target.Kind = domain.TargetPackage
	case len(req.Items) > 0:
		target.Kind = domain.TargetItems
	}
	levels := map[int64]*domain.BranchLevels{}
	var order []int64
	branchLevel := func(b int64) *domain.BranchLevels {
		if l, ok := levels[b]; ok {
			return l
		}
		levels[b] = &domain.BranchLevels{BranchID: b}
		order = append(order, b)
		return levels[b]
	}
	if err := q.LockProductForComposition(ctx, id); err != nil {
		return nil, err
	}
	own, err := q.LockProductStockLevels(ctx, &id)
	if err != nil {
		return nil, err
	}
	for _, l := range own {
		branchLevel(l.BranchID).Product = l.OnHand
	}
	if len(req.Items) == 1 {
		it := req.Items[0]
		factor, err := q.GetUnitToIngredientBase(ctx, db.GetUnitToIngredientBaseParams{UnitID: it.UnitID, IngredientID: it.IngredientID})
		if err != nil {
			return nil, notFound(err)
		}
		ing := it.IngredientID
		target.SingleIngredient = &ing
		target.QtyPerSale = domain.Round4(it.Quantity.Mul(factor))
		rows, err := q.LockIngredientStockLevels(ctx, &ing)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
				return nil, fmt.Errorf("%w: hay una venta en curso con ese insumo; vuelve a intentar en un momento", domain.ErrConflict)
			}
			return nil, err
		}
		for _, l := range rows {
			// Solo cuenta el insumo de las sucursales donde el producto tiene algo que entregar.
			if _, ok := levels[l.BranchID]; ok {
				levels[l.BranchID].Ingredient = l.OnHand
			}
		}
	}
	in := domain.ModeChangeInput{From: before, ProductID: id, Target: target,
		ActualStock: req.ActualStock, SeenProductOnHand: req.SeenProductOnHand, SeenIngredientOnHand: req.SeenIngredientOnHand}
	for _, b := range order {
		in.Levels = append(in.Levels, *levels[b])
	}
	plan, err := domain.PlanModeChange(in)
	if err != nil {
		return nil, err
	}
	note := fmt.Sprintf("Producto %d: deja de contar su propia existencia (%s)", id, audit.after)
	for _, a := range plan.Adjustments {
		arg := db.InsertModeAdjustmentParams{ItemType: db.StockItemType(a.ItemType), Quantity: domain.Round4(a.Qty),
			BranchID: a.BranchID, UserID: &actor, Note: &note}
		aid := a.ID
		if a.ItemType == "producto" {
			arg.ProductID = &aid
		} else {
			arg.IngredientID = &aid
		}
		if err := q.InsertModeAdjustment(ctx, arg); err != nil {
			return nil, err
		}
	}
	if plan.TurnOffTrackStock {
		if err := q.TurnOffProductTrackStock(ctx, id); err != nil {
			return nil, err
		}
	}
	return audit, nil
}

// createRecipe guarda los renglones como una receta nueva; sin renglones, ninguna.
func createRecipe(ctx context.Context, q *db.Queries, items []CompositionInputItem) (*int64, error) {
	if len(items) == 0 {
		return nil, nil
	}
	rid, err := q.CreateCompositionRecipe(ctx)
	if err != nil {
		return nil, err
	}
	for i, it := range items {
		if err := q.InsertCompositionItem(ctx, db.InsertCompositionItemParams{
			RecipeID: rid, IngredientID: it.IngredientID, Quantity: domain.Round4(it.Quantity),
			UnitID: it.UnitID, Position: int32(i),
		}); err != nil {
			return nil, err
		}
	}
	return &rid, nil
}

func stampOf(at pgtype.Timestamptz) string {
	if !at.Valid {
		return ""
	}
	return at.Time.UTC().Format(time.RFC3339Nano)
}

// checkNotStale rechaza guardar sobre una copia vieja: si la receta se guardó después de que la
// pantalla la abrió, quien guarda ahora pisaría ese cambio sin verlo.
func checkNotStale(ctx context.Context, q *db.Queries, kind CompositionKind, id int64, basedOn *string) error {
	if basedOn == nil {
		return nil
	}
	var at pgtype.Timestamptz
	switch kind {
	case CompositionOfProduct:
		p, err := q.GetProductComposition(ctx, id)
		if err != nil {
			return notFound(err)
		}
		at = p.CompositionConfirmedAt
	case CompositionOfOption:
		o, err := q.GetOptionComposition(ctx, id)
		if err != nil {
			return notFound(err)
		}
		at = o.CompositionConfirmedAt
	case CompositionOfIngredient:
		in, err := q.GetIngredientComposition(ctx, id)
		if err != nil {
			return notFound(err)
		}
		at = in.CompositionConfirmedAt
	}
	if stampOf(at) != *basedOn {
		return fmt.Errorf("%w: otra persona cambió esta receta mientras la editabas; ábrela de nuevo para ver su cambio", domain.ErrConflict)
	}
	return nil
}

// validateTwins acepta solo extras que de verdad se llaman igual que el que se guarda.
func validateTwins(ctx context.Context, q *db.Queries, kind CompositionKind, id int64, twins []int64) error {
	if len(twins) == 0 {
		return nil
	}
	if kind != CompositionOfOption {
		return fmt.Errorf("%w: solo un extra se guarda también en otros grupos", domain.ErrValidation)
	}
	same, err := q.ListSameNameOptions(ctx, id)
	if err != nil {
		return err
	}
	ok := map[int64]bool{}
	for _, s := range same {
		ok[s.ID] = true
	}
	// Cada gemelo solo una vez: repetido, crea una receta por cada vez con el candado de la empresa
	// tomado y deja las anteriores huérfanas.
	seen := map[int64]bool{}
	for _, t := range twins {
		if !ok[t] {
			return fmt.Errorf("%w: el extra %d no se llama igual", domain.ErrValidation, t)
		}
		if seen[t] {
			return fmt.Errorf("%w: el extra %d viene repetido", domain.ErrValidation, t)
		}
		seen[t] = true
	}
	return nil
}

// validateComposition revisa la captura de un producto o un extra contra el catálogo. Corre dentro
// de la transacción y detrás de LockCompositionEdits: fuera de ella, dos capturas cruzadas
// (P1 := [P2] y P2 := [P3]) pasaban las dos. Devuelve el nombre de cada componente para sus huecos.
func validateComposition(ctx context.Context, q *db.Queries, kind CompositionKind, id int64, req CompositionRequest) (map[int64]string, error) {
	in, err := compositionInput(ctx, q, req)
	if err != nil {
		return nil, err
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if req.LinkedProductID != nil {
		if _, err := q.GetLinkableProduct(ctx, *req.LinkedProductID); err != nil {
			return nil, fmt.Errorf("%w: ese producto no existe", domain.ErrValidation)
		}
	}
	names := map[int64]string{}
	if kind != CompositionOfProduct {
		return names, nil
	}
	p, err := q.GetProductComposition(ctx, id)
	if err != nil {
		return nil, notFound(err)
	}
	if p.Type == db.ProductTypeCombo {
		choices, err := q.PackageHasChoices(ctx, id)
		if err != nil {
			return nil, err
		}
		if choices {
			return nil, fmt.Errorf("%w: este paquete deja elegir entre productos o tiene un hueco vacío; capturarlo aquí lo borraría", domain.ErrConflict)
		}
	}
	if len(req.Components) > 0 {
		used, err := q.IsPackageComponent(ctx, id)
		if err != nil {
			return nil, err
		}
		if used {
			return nil, fmt.Errorf("%w: este producto va dentro de otro paquete", domain.ErrPackageInPackage)
		}
	}
	for _, c := range req.Components {
		cp, err := q.GetLinkableProduct(ctx, c.ProductID)
		if err != nil {
			return nil, fmt.Errorf("%w: el producto %d no existe", domain.ErrValidation, c.ProductID)
		}
		if c.ProductID == id || cp.Type == db.ProductTypeCombo {
			return nil, fmt.Errorf("%w (producto %d)", domain.ErrPackageInPackage, c.ProductID)
		}
		names[c.ProductID] = cp.Name
	}
	return names, nil
}

// ConfirmComposition marca como confirmada una composición estimada, con quién y cuándo. Recalcula
// el costo: lo confirmado no debe traer el costo de cuando se cargó como estimado.
func (s *AdminService) ConfirmComposition(ctx context.Context, kind CompositionKind, id, actor int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockCompositionEdits(ctx); err != nil {
			return err
		}
		var n int64
		var err error
		switch kind {
		case CompositionOfProduct:
			n, err = q.ConfirmProductComposition(ctx, db.ConfirmProductCompositionParams{ID: id, CompositionConfirmedBy: &actor})
		case CompositionOfOption:
			n, err = q.ConfirmOptionComposition(ctx, db.ConfirmOptionCompositionParams{ID: id, CompositionConfirmedBy: &actor})
		case CompositionOfIngredient:
			n, err = q.ConfirmIngredientComposition(ctx, db.ConfirmIngredientCompositionParams{ID: id, CompositionConfirmedBy: &actor})
		default:
			return domain.ErrValidation
		}
		if err == nil && n == 0 {
			return fmt.Errorf("%w: no hay una composición estimada que confirmar", domain.ErrConflict)
		}
		if err != nil {
			return err
		}
		return recomputeCosts(ctx, q)
	})
}

// savePreparedIngredient guarda un insumo que se prepara en el local: qué insumos lleva y cuánto
// rinde. Sin insumos vuelve a ser uno que se compra hecho. Rechaza los ciclos contra todo el
// catálogo de la empresa, no solo contra lo capturado: A no puede llevar B si B ya lleva A.
func (s *AdminService) savePreparedIngredient(ctx context.Context, id int64, req CompositionRequest, actor int64) error {
	if req.LinkedProductID != nil || len(req.Components) > 0 {
		return fmt.Errorf("%w: un insumo lleva insumos, no productos", domain.ErrValidation)
	}
	if len(req.Items) > 0 && req.Yield == nil {
		return fmt.Errorf("%w: falta cuánto rinde", domain.ErrValidation)
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		// El ciclo se juzga contra el catálogo de DENTRO de la transacción y con el candado: fuera,
		// A := [B] y B := [A] al mismo tiempo pasaban las dos.
		if err := q.LockCompositionEdits(ctx); err != nil {
			return err
		}
		if _, err := q.GetIngredientComposition(ctx, id); err != nil {
			return notFound(err)
		}
		if err := checkNotStale(ctx, q, CompositionOfIngredient, id, req.BasedOn); err != nil {
			return err
		}
		if err := resolveNewArticles(ctx, q, req.Items); err != nil {
			return err
		}
		in, err := compositionInput(ctx, q, req)
		if err != nil {
			return err
		}
		if err := in.Validate(); err != nil {
			return err
		}
		if len(req.Items) > 0 {
			graph, err := loadStockGraph(ctx, q)
			if err != nil {
				return err
			}
			components := make([]int64, 0, len(req.Items))
			for _, it := range req.Items {
				components = append(components, it.IngredientID)
			}
			if err := graph.ValidatePrepIngredient(id, components); err != nil {
				return err
			}
		}
		params := db.SetIngredientCompositionParams{ID: id}
		if len(req.Items) > 0 {
			rid, err := q.CreateCompositionRecipe(ctx)
			if err != nil {
				return err
			}
			for i, it := range req.Items {
				if err := q.InsertCompositionItem(ctx, db.InsertCompositionItemParams{
					RecipeID: rid, IngredientID: it.IngredientID, Quantity: domain.Round4(it.Quantity),
					UnitID: it.UnitID, Position: int32(i),
				}); err != nil {
					return err
				}
			}
			yield := domain.Round4(*req.Yield)
			confirmed := "confirmed"
			params.IsPrep, params.RecipeID, params.YieldQty = true, &rid, &yield
			params.Status, params.ConfirmedBy = &confirmed, &actor
		}
		n, err := q.SetIngredientComposition(ctx, params)
		if err == nil && n == 0 {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		return recomputeCosts(ctx, q)
	})
}

// compositionInput completa los tipos de unidad para que el dominio valide. Un insumo que no sale
// (de otra empresa, o que no existe) se rechaza aquí.
func compositionInput(ctx context.Context, q *db.Queries, req CompositionRequest) (domain.CompositionInput, error) {
	items := req.Items
	in := domain.CompositionInput{LinkedProductID: req.LinkedProductID, Yield: req.Yield}
	for _, c := range req.Components {
		in.Components = append(in.Components, domain.PackageComponent{ProductID: c.ProductID, Qty: c.Quantity})
	}
	if len(items) == 0 {
		return in, nil
	}
	ingIDs := make([]int64, 0, len(items))
	unitIDs := make([]int16, 0, len(items))
	for _, it := range items {
		ingIDs = append(ingIDs, it.IngredientID)
		unitIDs = append(unitIDs, it.UnitID)
	}
	ings, err := q.ListIngredientUnitKinds(ctx, ingIDs)
	if err != nil {
		return in, err
	}
	ingKind := map[int64]string{}
	for _, r := range ings {
		ingKind[r.ID] = r.Kind
	}
	units, err := q.ListUnitKinds(ctx, unitIDs)
	if err != nil {
		return in, err
	}
	unitKind := map[int16]string{}
	for _, r := range units {
		unitKind[r.ID] = r.Kind
	}
	for _, it := range items {
		ik, ok := ingKind[it.IngredientID]
		if !ok {
			return in, fmt.Errorf("%w: el insumo %d no existe", domain.ErrValidation, it.IngredientID)
		}
		uk, ok := unitKind[it.UnitID]
		if !ok {
			return in, fmt.Errorf("%w: la unidad %d no existe", domain.ErrValidation, it.UnitID)
		}
		in.Items = append(in.Items, domain.CompositionItem{
			IngredientID: it.IngredientID, Quantity: it.Quantity, UnitID: it.UnitID,
			UnitKind: uk, IngredientUnitKind: ik,
		})
	}
	return in, nil
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

// saleLineOf arma lo que la expansión necesita de un renglón ya valuado.
func saleLineOf(l domain.BuiltLine) domain.SaleLine {
	sl := domain.SaleLine{ProductID: l.ProductID, Qty: l.Qty}
	for _, m := range l.Modifiers {
		sl.Options = append(sl.Options, domain.SaleOption{OptionID: m.OptionID, Qty: decimal.NewFromInt(int64(m.Qty))})
	}
	return sl
}

// resolveNewArticles da de alta, dentro de la transacción de la receta, los insumos que nacen con ella
// y deja su id en el renglón.
func resolveNewArticles(ctx context.Context, q *db.Queries, items []CompositionInputItem) error {
	for i, it := range items {
		if it.NewArticle == nil {
			continue
		}
		if it.IngredientID != 0 {
			return fmt.Errorf("%w: un insumo nuevo no lleva id", domain.ErrValidation)
		}
		id, err := ensureArticleTx(ctx, q, *it.NewArticle)
		if err != nil {
			return err
		}
		items[i].IngredientID = id
	}
	return nil
}
