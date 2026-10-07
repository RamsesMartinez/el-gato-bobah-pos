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
)

// CompositionItemView es un renglón de la composición, para la hoja «Qué lleva».
type CompositionItemView struct {
	IngredientID   int64           `json:"ingredientId"`
	IngredientName string          `json:"ingredientName"`
	Quantity       decimal.Decimal `json:"quantity"`
	UnitID         int16           `json:"unitId"`
	UnitCode       string          `json:"unitCode"`
}

// CompositionView es lo que lleva un producto o un extra y si está confirmado.
type CompositionView struct {
	// Status: "" sin capturar, "estimated" o "confirmed".
	Status            string                `json:"status"`
	ConfirmedBy       string                `json:"confirmedBy,omitempty"`
	ConfirmedAt       *time.Time            `json:"confirmedAt,omitempty"`
	LinkedProductID   *int64                `json:"linkedProductId,omitempty"`
	LinkedProductName string                `json:"linkedProductName,omitempty"`
	Items             []CompositionItemView `json:"items"`
	// Editable es falso en un producto con existencias propias o un paquete: lo que descuentan no
	// se captura aquí.
	Editable bool   `json:"editable"`
	Reason   string `json:"reason,omitempty"`
}

// CompositionInputItem es un renglón capturado.
type CompositionInputItem struct {
	IngredientID int64           `json:"ingredientId"`
	Quantity     decimal.Decimal `json:"quantity"`
	UnitID       int16           `json:"unitId"`
}

// Composition devuelve lo que lleva un producto o un extra.
func (s *AdminService) Composition(ctx context.Context, kind CompositionKind, id int64) (CompositionView, error) {
	q := s.store.QC(ctx)
	v := CompositionView{Items: []CompositionItemView{}, Editable: true}
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
		switch {
		case p.Type == db.ProductTypeCombo:
			v.Editable, v.Reason = false, "package"
		case p.TrackStock:
			v.Editable, v.Reason = false, "own_stock"
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
func (s *AdminService) SaveComposition(ctx context.Context, kind CompositionKind, id int64,
	items []CompositionInputItem, linkedProduct *int64, actor int64,
) error {
	if kind != CompositionOfProduct && kind != CompositionOfOption {
		return domain.ErrValidation
	}
	if kind == CompositionOfProduct && linkedProduct != nil {
		return fmt.Errorf("%w: un producto no se liga a otro; un paquete se arma con sus productos", domain.ErrValidation)
	}
	q := s.store.QC(ctx)
	in, err := compositionInput(ctx, q, items, linkedProduct)
	if err != nil {
		return err
	}
	if err := in.Validate(); err != nil {
		return err
	}
	if linkedProduct != nil {
		lp, err := q.GetLinkableProduct(ctx, *linkedProduct)
		if err != nil {
			return fmt.Errorf("%w: ese producto no existe", domain.ErrValidation)
		}
		if lp.Type == db.ProductTypeCombo {
			return fmt.Errorf("%w (producto %d)", domain.ErrPackageInPackage, lp.ID)
		}
	}
	if kind == CompositionOfProduct {
		p, err := q.GetProductComposition(ctx, id)
		if err != nil {
			return notFound(err)
		}
		if p.Type == db.ProductTypeCombo || (p.TrackStock && len(items) > 0) {
			return fmt.Errorf("%w: este producto descuenta sus propias existencias o sus productos", domain.ErrValidation)
		}
	}

	return s.store.WithTx(ctx, func(q *db.Queries) error {
		var recipe *int64
		if len(items) > 0 {
			rid, err := q.CreateCompositionRecipe(ctx)
			if err != nil {
				return err
			}
			for i, it := range items {
				if err := q.InsertCompositionItem(ctx, db.InsertCompositionItemParams{
					RecipeID: rid, IngredientID: it.IngredientID, Quantity: domain.Round4(it.Quantity),
					UnitID: it.UnitID, Position: int32(i),
				}); err != nil {
					return err
				}
			}
			recipe = &rid
		}
		var status *string
		var by *int64
		if recipe != nil || linkedProduct != nil {
			confirmed := "confirmed"
			status, by = &confirmed, &actor
		}
		var n int64
		var err error
		if kind == CompositionOfProduct {
			n, err = q.SetProductComposition(ctx, db.SetProductCompositionParams{
				ID: id, RecipeID: recipe, Status: status, ConfirmedBy: by,
			})
		} else {
			n, err = q.SetOptionComposition(ctx, db.SetOptionCompositionParams{
				ID: id, RecipeID: recipe, LinkedProductID: linkedProduct, Status: status, ConfirmedBy: by,
			})
		}
		if err == nil && n == 0 {
			return domain.ErrNotFound
		}
		return err
	})
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
	default:
		return domain.ErrValidation
	}
	if err == nil && n == 0 {
		return fmt.Errorf("%w: no hay una composición estimada que confirmar", domain.ErrConflict)
	}
	return err
}

// compositionInput completa los tipos de unidad para que el dominio valide. Un insumo que no sale
// (de otra empresa, o que no existe) se rechaza aquí.
func compositionInput(ctx context.Context, q *db.Queries, items []CompositionInputItem, linked *int64) (domain.CompositionInput, error) {
	in := domain.CompositionInput{LinkedProductID: linked}
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
