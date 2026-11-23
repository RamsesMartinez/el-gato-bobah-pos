package app

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// RealWasteView es la merma real de 90 días de un artículo en un almacén (spec 096).
type RealWasteView struct {
	WarehouseID   int64                 `json:"warehouseId"`
	WarehouseName string                `json:"warehouseName"`
	IsSales       bool                  `json:"isSales"`
	Wasted        decimal.Decimal       `json:"wasted"`
	Consumed      decimal.Decimal       `json:"consumed"`
	Ratio         *float64              `json:"ratio"`
	State         domain.RealWasteState `json:"state"`
}

// realWaste devuelve, por artículo, su merma real en cada almacén donde tuvo ventas o merma en 90
// días. Incluye lo de los artículos que se le unieron, con su factor (R14 H1). Siempre un arreglo por
// artículo pedido, aunque vacío.
func realWaste(ctx context.Context, q *db.Queries, ingredientIDs []int64) (map[int64][]RealWasteView, error) {
	out := make(map[int64][]RealWasteView, len(ingredientIDs))
	if len(ingredientIDs) == 0 {
		return out, nil
	}
	rawEdges, err := q.ListMergeEdges(ctx)
	if err != nil {
		return nil, err
	}
	edges := make([]domain.MergeEdge, len(rawEdges))
	for i, e := range rawEdges {
		edges[i] = domain.MergeEdge{Old: e.OldIngredientID, New: e.NewIngredientID, Factor: e.Factor.InexactFloat64()}
	}
	chains := map[int64]map[int64]float64{}
	var all []int64
	for _, id := range ingredientIDs {
		chains[id] = domain.MergeChain(id, edges)
		for src := range chains[id] {
			all = append(all, src)
		}
	}
	sales, err := q.ListRealWasteConsumption(ctx, all)
	if err != nil {
		return nil, err
	}
	waste, err := q.ListRealWasteRecords(ctx, all)
	if err != nil {
		return nil, err
	}
	whs, err := q.ListWarehouses(ctx, true)
	if err != nil {
		return nil, err
	}
	type key struct{ ing, wh int64 }
	consumed, wasted := map[key]float64{}, map[key]float64{}
	for target, chain := range chains {
		for _, s := range sales {
			if f, ok := chain[s.IngredientID]; ok {
				consumed[key{target, s.WarehouseID}] += s.Consumed.InexactFloat64() * f
			}
		}
		for _, w := range waste {
			if f, ok := chain[w.IngredientID]; ok {
				wasted[key{target, w.WarehouseID}] += w.Wasted.InexactFloat64() * f
			}
		}
	}
	for _, id := range ingredientIDs {
		views := []RealWasteView{}
		for _, wh := range whs {
			k := key{id, wh.ID}
			c, w := consumed[k], wasted[k]
			if c == 0 && w == 0 && !wh.IsSales {
				continue // un almacén sin nada de este artículo no se lista; el de venta siempre
			}
			ratio, state := domain.RealWaste(w, c)
			views = append(views, RealWasteView{
				WarehouseID: wh.ID, WarehouseName: wh.Name, IsSales: wh.IsSales,
				Wasted: decimal.NewFromFloat(w).Round(4), Consumed: decimal.NewFromFloat(c).Round(4),
				Ratio: ratio, State: state,
			})
		}
		out[id] = views
	}
	return out, nil
}

// ArticleRealWaste es la merma real de un artículo, para su ficha.
func (s *CostingService) ArticleRealWaste(ctx context.Context, ingredientID int64) ([]RealWasteView, error) {
	q := s.store.QC(ctx)
	// Uno de otra empresa no se distingue de uno que no existe (RLS no lo deja ver).
	if labels, err := q.ListIngredientLabels(ctx, []int64{ingredientID}); err != nil {
		return nil, err
	} else if len(labels) == 0 {
		return nil, domain.ErrNotFound
	}
	m, err := realWaste(ctx, q, []int64{ingredientID})
	if err != nil {
		return nil, err
	}
	return m[ingredientID], nil
}

// CostLineView es un renglón del desglose con su nombre y su merma real.
type CostLineView struct {
	IngredientID int64              `json:"ingredientId"`
	Name         string             `json:"name"`
	BaseUnit     string             `json:"baseUnit"`
	IsPrep       bool               `json:"isPrep"`
	QtyBase      decimal.Decimal    `json:"qtyBase"`
	UnitCost     decimal.Decimal    `json:"unitCost"`
	Base         decimal.Decimal    `json:"base"`
	Pct          float64            `json:"pct"`
	PctSource    domain.WasteSource `json:"pctSource"`
	Waste        decimal.Decimal    `json:"waste"`
	RealWaste    []RealWasteView    `json:"realWaste"`
}

// CostComponentView es un producto dentro de un combo.
type CostComponentView struct {
	ProductID int64           `json:"productId"`
	Name      string          `json:"name"`
	Qty       int             `json:"qty"`
	Cost      decimal.Decimal `json:"cost"`
	Waste     decimal.Decimal `json:"waste"`
}

// ProductCostingView es de dónde sale el costo de un producto (GET /products/{id}/costing).
type ProductCostingView struct {
	ProductID  int64               `json:"productId"`
	Cost       decimal.Decimal     `json:"cost"`
	Source     string              `json:"source"`
	Base       decimal.Decimal     `json:"base"`
	Waste      decimal.Decimal     `json:"waste"`
	GeneralPct *float64            `json:"generalPct"`
	ProductPct *float64            `json:"productPct"`
	Lines      []CostLineView      `json:"lines"`
	Components []CostComponentView `json:"components"`
}

// ProductCosting arma el desglose del costo de un producto con la merma real de cada artículo.
// withRealWaste: quien no puede ver el almacén recibe realWaste vacío (no nulo) en cada renglón.
func (s *CostingService) ProductCosting(ctx context.Context, productID int64, withRealWaste bool) (ProductCostingView, error) {
	q := s.store.QC(ctx)
	g, _, prods, _, err := loadGraph(ctx, q)
	if err != nil {
		return ProductCostingView{}, err
	}
	var prodPct *float64
	found := false
	for _, p := range prods {
		if p.ID == productID {
			found, prodPct = true, pctF(p.ExpectedWastePct)
		}
	}
	if !found {
		return ProductCostingView{}, domain.ErrNotFound
	}
	b := g.ProductBreakdown(productID)
	v := ProductCostingView{
		ProductID: productID, Source: b.Source, Cost: d4(b.Total), Base: d4(b.Base), Waste: d4(b.Waste),
		GeneralPct: g.General(), ProductPct: prodPct,
		Lines: make([]CostLineView, 0, len(b.Lines)), Components: make([]CostComponentView, 0, len(b.Components)),
	}
	ids := make([]int64, 0, len(b.Lines))
	for _, l := range b.Lines {
		ids = append(ids, l.IngredientID)
	}
	labels, err := q.ListIngredientLabels(ctx, ids)
	if err != nil {
		return ProductCostingView{}, err
	}
	name, unit := map[int64]string{}, map[int64]string{}
	for _, l := range labels {
		name[l.ID], unit[l.ID] = l.Name, l.BaseUnit
	}
	real := map[int64][]RealWasteView{}
	if withRealWaste {
		if real, err = realWaste(ctx, q, ids); err != nil {
			return ProductCostingView{}, err
		}
	}
	for _, l := range b.Lines {
		rw := real[l.IngredientID]
		if rw == nil {
			rw = []RealWasteView{}
		}
		v.Lines = append(v.Lines, CostLineView{
			IngredientID: l.IngredientID, Name: name[l.IngredientID], BaseUnit: unit[l.IngredientID], IsPrep: l.IsPrep,
			QtyBase: d4(l.QtyBase), UnitCost: decimal.NewFromFloat(l.UnitCost).Round(6), Base: d4(l.Base),
			Pct: l.Pct, PctSource: l.Source, Waste: d4(l.Waste), RealWaste: rw,
		})
	}
	pids := make([]int64, 0, len(b.Components))
	for _, c := range b.Components {
		pids = append(pids, c.ProductID)
	}
	plabels, err := q.ListProductLabels(ctx, pids)
	if err != nil {
		return ProductCostingView{}, err
	}
	pname := map[int64]string{}
	for _, l := range plabels {
		pname[l.ID] = l.Name
	}
	for _, c := range b.Components {
		v.Components = append(v.Components, CostComponentView{ProductID: c.ProductID, Name: pname[c.ProductID], Qty: c.Qty, Cost: d4(c.Cost), Waste: d4(c.Waste)})
	}
	return v, nil
}

func d4(x float64) decimal.Decimal { return decimal.NewFromFloat(x).Round(4) }
