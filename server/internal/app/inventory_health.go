package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// InventoryHealthService arma «Salud del inventario» (spec 037). La pantalla y la exportación leen
// de aquí y de nada más: si divergen, una de las dos miente.
type InventoryHealthService struct {
	store *store.Store
	now   func() time.Time
}

func NewInventoryHealthService(s *store.Store) *InventoryHealthService {
	return &InventoryHealthService{store: s, now: time.Now}
}

// Today es el día de negocio de hoy, en la zona del local: el periodo por omisión termina ahí. En
// UTC, después de las 18:00 de México ya sería mañana.
func (s *InventoryHealthService) Today(ctx context.Context) time.Time {
	tz, err := s.store.QC(ctx).GetBusinessTimezone(ctx)
	if err != nil {
		tz = domain.DefaultTimezone
	}
	return domain.BusinessDate(s.now(), domain.LoadBusinessLocation(tz))
}

// HealthChannelUnits son las piezas de un canal (mostrador o una plataforma).
type HealthChannelUnits struct {
	Channel string          `json:"channel"`
	Units   decimal.Decimal `json:"units"`
}

// HealthUnsoldItem es un producto o extra que se vendió sin restar.
type HealthUnsoldItem struct {
	Kind   string               `json:"kind"` // "product" o "extra"
	ID     int64                `json:"id"`
	Name   string               `json:"name"`
	Group  string               `json:"group"`
	Status string               `json:"status"` // domain.ConsumptionMode del producto; "extra" en un extra
	Units  []HealthChannelUnits `json:"units"`
	Pieces decimal.Decimal      `json:"pieces"`
	// LinesWithoutDeduction: renglones de venta que no restaron.
	LinesWithoutDeduction int `json:"linesWithoutDeduction"`
	// EstimatedCost: piezas × costo de hoy. Ordena la lista; no es contable y no se suma a nada.
	// Nil cuando nadie ha capturado el costo: un 0 se leería como «aquí no se pierde dinero».
	EstimatedCost *decimal.Decimal `json:"estimatedCost"`
	FixAt         string           `json:"fixAt"`
}

type HealthComboChoice struct {
	ProductID int64  `json:"productId"`
	Name      string `json:"name"`
	Slot      string `json:"slot"`
	Options   int    `json:"options"`
	FixAt     string `json:"fixAt"`
}

type HealthNegative struct {
	IngredientID int64           `json:"ingredientId"`
	Name         string          `json:"name"`
	Unit         string          `json:"unit"`
	OnHand       decimal.Decimal `json:"onHand"`
	// Branch solo viene con más de una sucursal: con una, nombrarla es ruido.
	Branch *string `json:"branch"`
	UsedBy int     `json:"usedBy"`
	FixAt  string  `json:"fixAt"`
}

type HealthNoPurchase struct {
	IngredientID int64  `json:"ingredientId"`
	Name         string `json:"name"`
	Unit         string `json:"unit"`
	FixAt        string `json:"fixAt"`
}

type HealthMalformed struct {
	OwnerKind  string `json:"ownerKind"`
	OwnerID    int64  `json:"ownerId"`
	OwnerName  string `json:"ownerName"`
	Ingredient string `json:"ingredient"`
	Problem    string `json:"problem"`
	FixAt      string `json:"fixAt"`
}

type HealthPendingCount struct {
	Products int    `json:"products"`
	Extras   int    `json:"extras"`
	FixAt    string `json:"fixAt"`
}

// InventoryHealthView es la pantalla completa. Toda lista viaja como [] aunque esté vacía.
type InventoryHealthView struct {
	From             string `json:"from"`
	To               string `json:"to"`
	MultipleBranches bool   `json:"multipleBranches"`
	SalesInPeriod    int    `json:"salesInPeriod"`
	UnsoldDeduction  struct {
		Total                 int                `json:"total"`
		Items                 []HealthUnsoldItem `json:"items"`
		UnpairedPlatformLines int                `json:"unpairedPlatformLines"`
	} `json:"unsoldDeduction"`
	ComboChoices struct {
		Total int                 `json:"total"`
		Items []HealthComboChoice `json:"items"`
	} `json:"comboChoices"`
	NegativeStock struct {
		Total int `json:"total"`
		// Collapsed: la empresa no tiene ninguna compra registrada, y lo guardado solo baja.
		Collapsed bool             `json:"collapsed"`
		Items     []HealthNegative `json:"items"`
	} `json:"negativeStock"`
	NoPurchases struct {
		Total int                `json:"total"`
		Of    int                `json:"of"`
		Items []HealthNoPurchase `json:"items"`
	} `json:"noPurchases"`
	MalformedRecipes struct {
		Total int               `json:"total"`
		Items []HealthMalformed `json:"items"`
	} `json:"malformedRecipes"`
	PendingRecipes struct {
		Undecided HealthPendingCount `json:"undecided"`
		Review    HealthPendingCount `json:"review"`
	} `json:"pendingRecipes"`
}

func recipeFixAt(kind string, id int64) string {
	if kind == "extra" {
		return "/catalogo/recetas?kind=extra&open=option:" + strconv.FormatInt(id, 10)
	}
	return "/catalogo/recetas?kind=product&open=product:" + strconv.FormatInt(id, 10)
}

func ingredientFixAt(id int64) string {
	return "/almacen?tab=insumos&open=" + strconv.FormatInt(id, 10)
}

func healthDate(t pgtype.Date) string { return t.Time.Format("2006-01-02") }

// allRows es el «sin paginar» de la exportación: el archivo trae todo lo que la pantalla cuenta.
const allRows = int32(1 << 30)

// sectionPage es la página de una sección: sin sección pedida, la primera de todas; con una, solo
// esa recibe el desplazamiento y las demás no se consultan (la pantalla pide «Ver 25 más»).
func sectionPage(f domain.HealthFilter, s domain.HealthSection) (lim, off int32, wanted bool) {
	if f.Limit == allRows {
		return allRows, 0, true
	}
	if f.Section == "" {
		return f.Limit, 0, true
	}
	if f.Section == s {
		return f.Limit, f.Offset, true
	}
	return 0, 0, false
}

// Report calcula las cinco secciones.
func (s *InventoryHealthService) Report(ctx context.Context, f domain.HealthFilter) (InventoryHealthView, error) {
	if err := f.Validate(); err != nil {
		return InventoryHealthView{}, err
	}
	return s.report(ctx, f)
}

// report es el cálculo único de la pantalla y de la exportación: con Limit = allRows trae todo.
func (s *InventoryHealthService) report(ctx context.Context, f domain.HealthFilter) (InventoryHealthView, error) {
	q := s.store.QC(ctx)
	from := pgtype.Date{Time: f.Range.From, Valid: true}
	to := pgtype.Date{Time: f.Range.To, Valid: true}
	var since pgtype.Date
	if f.NoPurchasesSince != nil {
		since = pgtype.Date{Time: *f.NoPurchasesSince, Valid: true}
	}

	var v InventoryHealthView
	v.From, v.To = healthDate(from), healthDate(to)
	v.UnsoldDeduction.Items = []HealthUnsoldItem{}
	v.ComboChoices.Items = []HealthComboChoice{}
	v.NegativeStock.Items = []HealthNegative{}
	v.NoPurchases.Items = []HealthNoPurchase{}
	v.MalformedRecipes.Items = []HealthMalformed{}

	var err error
	if v.MultipleBranches, err = q.HealthMultipleBranches(ctx); err != nil {
		return v, err
	}
	sales, err := q.HealthSalesInPeriod(ctx, db.HealthSalesInPeriodParams{FromDate: from, ToDate: to})
	if err != nil {
		return v, err
	}
	v.SalesInPeriod = int(sales)

	if lim, off, ok := sectionPage(f, domain.SectionUnsold); ok {
		rows, err := q.HealthUnsold(ctx, db.HealthUnsoldParams{FromDate: from, ToDate: to, Lim: lim, Off: off})
		if err != nil {
			return v, err
		}
		for _, r := range rows {
			it, err := unsoldItem(r)
			if err != nil {
				return v, err
			}
			v.UnsoldDeduction.Items = append(v.UnsoldDeduction.Items, it)
			v.UnsoldDeduction.Total = int(r.Total)
		}
		// Una página vacía más allá del final no trae count(*) over(): el total se pide aparte, o
		// diría 0 y se leería como «todo sano».
		if len(rows) == 0 && off > 0 {
			first, err := q.HealthUnsold(ctx, db.HealthUnsoldParams{FromDate: from, ToDate: to, Lim: 1, Off: 0})
			if err != nil {
				return v, err
			}
			if len(first) > 0 {
				v.UnsoldDeduction.Total = int(first[0].Total)
			}
		}
		unpaired, err := q.HealthUnpairedLines(ctx, db.HealthUnpairedLinesParams{FromDate: from, ToDate: to})
		if err != nil {
			return v, err
		}
		v.UnsoldDeduction.UnpairedPlatformLines = int(unpaired)
		combos, err := q.HealthComboChoices(ctx)
		if err != nil {
			return v, err
		}
		for _, c := range combos {
			v.ComboChoices.Items = append(v.ComboChoices.Items, HealthComboChoice{
				ProductID: c.ID, Name: c.Name, Slot: c.Slot, Options: int(c.Options), FixAt: recipeFixAt("product", c.ID),
			})
		}
		v.ComboChoices.Total = len(v.ComboChoices.Items)
	}

	anyPurchase, err := q.HealthAnyPurchase(ctx)
	if err != nil {
		return v, err
	}
	v.NegativeStock.Collapsed = !anyPurchase
	if lim, off, ok := sectionPage(f, domain.SectionNegative); ok {
		rows, err := q.HealthNegativeStock(ctx, db.HealthNegativeStockParams{Lim: lim, Off: off})
		if err != nil {
			return v, err
		}
		for _, r := range rows {
			n := HealthNegative{IngredientID: r.IngredientID, Name: r.Name, Unit: r.Unit, OnHand: r.OnHand,
				UsedBy: int(r.UsedBy), FixAt: ingredientFixAt(r.IngredientID)}
			if v.MultipleBranches {
				b := r.Branch
				n.Branch = &b
			}
			v.NegativeStock.Items = append(v.NegativeStock.Items, n)
			v.NegativeStock.Total = int(r.Total)
		}
		if len(rows) == 0 && off > 0 {
			first, err := q.HealthNegativeStock(ctx, db.HealthNegativeStockParams{Lim: 1, Off: 0})
			if err != nil {
				return v, err
			}
			if len(first) > 0 {
				v.NegativeStock.Total = int(first[0].Total)
			}
		}
	}

	// «Sin compras» abre resumida: la lista se trae solo cuando se pide la sección.
	counts, err := q.HealthNoPurchasesCount(ctx, since)
	if err != nil {
		return v, err
	}
	v.NoPurchases.Total, v.NoPurchases.Of = int(counts.Missing), int(counts.OfTotal)
	if f.Section == domain.SectionNoPurchases || f.Limit == allRows {
		lim, off := f.Limit, f.Offset
		if f.Limit == allRows {
			off = 0
		}
		rows, err := q.HealthNoPurchases(ctx, db.HealthNoPurchasesParams{Since: since, Lim: lim, Off: off})
		if err != nil {
			return v, err
		}
		for _, r := range rows {
			v.NoPurchases.Items = append(v.NoPurchases.Items, HealthNoPurchase{
				IngredientID: r.IngredientID, Name: r.Name, Unit: r.Unit, FixAt: ingredientFixAt(r.IngredientID),
			})
		}
	}

	if lim, off, ok := sectionPage(f, domain.SectionMalformed); ok {
		rows, err := q.HealthMalformedRecipes(ctx, db.HealthMalformedRecipesParams{Lim: lim, Off: off})
		if err != nil {
			return v, err
		}
		for _, r := range rows {
			m := HealthMalformed{OwnerKind: r.OwnerKind, OwnerID: r.OwnerID, OwnerName: r.OwnerName,
				Ingredient: r.Ingredient, Problem: r.Problem}
			switch r.OwnerKind {
			case "prep":
				m.FixAt = ingredientFixAt(r.OwnerID)
			default:
				m.FixAt = recipeFixAt(r.OwnerKind, r.OwnerID)
			}
			v.MalformedRecipes.Items = append(v.MalformedRecipes.Items, m)
			v.MalformedRecipes.Total = int(r.Total)
		}
		if len(rows) == 0 && off > 0 {
			first, err := q.HealthMalformedRecipes(ctx, db.HealthMalformedRecipesParams{Lim: 1, Off: 0})
			if err != nil {
				return v, err
			}
			if len(first) > 0 {
				v.MalformedRecipes.Total = int(first[0].Total)
			}
		}
	}

	// Los mismos números que las pestañas de Menú › Recetas: si divergen, una de las dos miente.
	products, err := recipeCounts(ctx, q, domain.RecipeKindProduct, 0, "")
	if err != nil {
		return v, err
	}
	extras, err := recipeCounts(ctx, q, domain.RecipeKindExtra, 0, "")
	if err != nil {
		return v, err
	}
	v.PendingRecipes.Undecided = HealthPendingCount{Products: products.Pending, Extras: extras.Pending,
		FixAt: "/catalogo/recetas?kind=product&status=pending"}
	v.PendingRecipes.Review = HealthPendingCount{Products: products.Review, Extras: extras.Review,
		FixAt: "/catalogo/recetas?kind=product&status=review"}
	return v, nil
}

func unsoldItem(r db.HealthUnsoldRow) (HealthUnsoldItem, error) {
	it := HealthUnsoldItem{
		Kind: r.Kind, ID: r.ID, Name: r.Name, Group: r.Grp, Pieces: r.Pieces,
		LinesWithoutDeduction: int(r.Lines), FixAt: recipeFixAt(r.Kind, r.ID),
		Units: []HealthChannelUnits{},
	}
	if r.HasCost {
		it.EstimatedCost = &r.EstimatedCost
	}
	if err := json.Unmarshal(r.Units, &it.Units); err != nil {
		return it, fmt.Errorf("piezas por canal de %s %d: %w", r.Kind, r.ID, err)
	}
	if r.Kind == "extra" {
		it.Status = "extra"
	} else {
		it.Status = string(domain.ModeOf(domain.ProductModeFacts{
			TrackStock: r.TrackStock, HasRecipeItems: r.HasRecipeItems,
			IsPackage: r.ProductType == "combo", CompositionStatus: r.CompositionStatus,
		}))
	}
	return it, nil
}
