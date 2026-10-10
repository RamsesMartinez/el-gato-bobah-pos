package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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
	// Editable es falso en un producto con existencias propias: descuenta él mismo, y no hay nada
	// que capturar.
	Editable bool   `json:"editable"`
	Reason   string `json:"reason,omitempty"`
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
}

// CompositionInputItem es un renglón capturado.
type CompositionInputItem struct {
	IngredientID int64           `json:"ingredientId"`
	Quantity     decimal.Decimal `json:"quantity"`
	UnitID       int16           `json:"unitId"`
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
			v.Editable, v.Reason = false, "own_stock"
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
			v.SameName = append(v.SameName, SameNameOption{ID: t.ID, Group: t.GroupName, Stamp: stampOf(t.CompositionConfirmedAt)})
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
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockCompositionEdits(ctx); err != nil {
			return err
		}
		if err := checkNotStale(ctx, q, kind, id, req.BasedOn); err != nil {
			return err
		}
		names, err := validateComposition(ctx, q, kind, id, req)
		if err != nil {
			return err
		}
		if err := validateTwins(ctx, q, kind, id, req.AlsoOptionIDs); err != nil {
			return err
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
		if recipe != nil || req.LinkedProductID != nil || len(req.Components) > 0 {
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
		return err
	})
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
	if p.TrackStock && (len(req.Items) > 0 || len(req.Components) > 0) {
		return nil, fmt.Errorf("%w: este producto descuenta sus propias existencias", domain.ErrValidation)
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

// ConfirmComposition marca como confirmada una composición estimada, con quién y cuándo.
func (s *AdminService) ConfirmComposition(ctx context.Context, kind CompositionKind, id, actor int64) error {
	q := s.store.QC(ctx)
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
	return err
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
		return err
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
