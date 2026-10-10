// Package fudoimport carga desde los archivos de FUDO lo que lleva cada producto, extra e insumo,
// como composición ESTIMADA (spec 028). Solo llena lo que falta: lo capturado no se pisa.
//
// Está separado en un plan puro y su aplicación para que la decisión de qué se escribe se pruebe
// sin base de datos y sin los archivos del negocio, que no entran al repositorio.
package fudoimport

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Row es un renglón de un CSV de FUDO, por nombre de columna.
type Row map[string]string

// Sources son los archivos de FUDO que importan aquí.
type Sources struct {
	Recipes         []Row             // recetas.csv: Producto, Ingrediente, Cantidad, Unidad
	SubIngredients  []Row             // subingredientes.csv: Ingrediente, Subingrediente, Cantidad, Unidad
	SubProducts     []Row             // subproductos.csv: Producto, Subproducto, Cantidad, Unidad
	IngredientUnits map[string]string // ingredientes.csv: Nombre → Unidad
}

// Unit es una unidad del POS.
type Unit struct {
	ID     int16
	Kind   string
	ToBase decimal.Decimal
}

// Product, Option e Ingredient son lo que el plan necesita saber del catálogo de UNA empresa.
type Product struct {
	ID             int64
	Name           string
	TrackStock     bool
	IsCombo        bool
	HasComposition bool
}

type Option struct {
	ID             int64
	Name           string
	HasComposition bool
}

type Ingredient struct {
	ID             int64
	Name           string
	BaseKind       string
	HasComposition bool
}

// Catalog es el catálogo de la empresa destino.
type Catalog struct {
	Units       map[string]Unit
	Products    []Product
	Options     []Option
	Ingredients []Ingredient
	// PrepComponents son los insumos que ya lleva cada insumo compuesto del POS: sin ellos, un
	// insumo nuevo podría cerrar un ciclo con uno que ya existía.
	PrepComponents map[int64][]int64
}

// Line es un renglón de receta, ya en la unidad base del insumo.
type Line struct {
	IngredientID int64
	Qty          decimal.Decimal
	UnitID       int16
}

// Prep es la receta de un insumo compuesto y cuánto rinde, en su unidad base.
type Prep struct {
	Lines []Line
	Yield decimal.Decimal
}

// Report es lo que la carga no pudo hacer sola y alguien tiene que revisar.
type Report struct {
	Ambiguous          []string
	MissingIngredients []string
	UnitMismatch       []string
	InvalidQuantity    []string
	NotInCatalog       []string
	Packages           []string
	Cycles             []string
	AlreadyCaptured    int
}

// Plan es lo que se va a escribir.
type Plan struct {
	ProductRecipes  map[int64][]Line
	OptionRecipes   map[int64][]Line
	OptionLinks     map[int64]int64
	PrepIngredients map[int64]Prep
	Report          Report
}

var spaces = regexp.MustCompile(`\s+`)

func norm(s string) string {
	return strings.ToUpper(spaces.ReplaceAllString(strings.TrimSpace(s), " "))
}

// unitCode traduce la unidad como la escribe FUDO al código del POS.
func unitCode(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "kg", "kilogramo", "kilo":
		return "kg"
	case "g", "gr", "gramo", "gramos":
		return "g"
	case "l", "lt", "litro", "litros":
		return "l"
	case "ml", "mililitro":
		return "ml"
	case "fl oz", "floz", "oz", "onza":
		return "floz"
	case "cda", "cucharada":
		return "cda"
	case "cdta", "cucharadita":
		return "cdta"
	}
	return "pieza"
}

// PlanCompositions decide qué composición estimada recibe cada cosa del catálogo que no la tiene.
//
// Un extra que se llama igual que un producto ES ese producto (FUDO modelaba los extras como
// productos) y se liga a él; si no, recibe la receta de FUDO con su nombre. Una receta con un insumo
// que no existe o con una unidad de otro tipo se deja fuera entera: a medias descontaría de menos y
// nadie lo notaría. Los paquetes de FUDO no se cargan: sus cantidades no siempre son piezas por
// paquete (hay renglones de decenas), y un paquete mal armado descuenta de más en cada venta. Se
// reportan para armarlos en Menú › Recetas.
func PlanCompositions(src Sources, cat Catalog) Plan {
	p := Plan{
		ProductRecipes: map[int64][]Line{}, OptionRecipes: map[int64][]Line{},
		OptionLinks: map[int64]int64{}, PrepIngredients: map[int64]Prep{},
	}
	products := map[string][]Product{}
	for _, x := range cat.Products {
		products[norm(x.Name)] = append(products[norm(x.Name)], x)
	}
	ingredients := map[string][]Ingredient{}
	for _, x := range cat.Ingredients {
		ingredients[norm(x.Name)] = append(ingredients[norm(x.Name)], x)
	}
	baseUnit := map[string]int16{}
	for code, u := range cat.Units {
		if u.ToBase.Equal(decimal.NewFromInt(1)) && (code == "g" || code == "ml" || code == "pieza") {
			baseUnit[u.Kind] = u.ID
		}
	}

	type recipe struct {
		name string
		rows []Row
	}
	collect := func(rows []Row, ownerCol string) []recipe {
		var out []recipe
		idx := map[string]int{}
		for _, r := range rows {
			k := norm(r[ownerCol])
			if k == "" {
				continue
			}
			i, ok := idx[k]
			if !ok {
				i = len(out)
				idx[k] = i
				out = append(out, recipe{name: strings.TrimSpace(r[ownerCol])})
			}
			out[i].rows = append(out[i].rows, r)
		}
		return out
	}
	// lines convierte a la unidad base y suma lo repetido. ok=false si la receta no se puede usar.
	lines := func(owner string, rows []Row, ingCol string) ([]Line, bool) {
		var out []Line
		pos := map[int64]int{}
		for _, r := range rows {
			matches := ingredients[norm(r[ingCol])]
			switch {
			case len(matches) == 0:
				p.Report.MissingIngredients = append(p.Report.MissingIngredients, fmt.Sprintf("%s: %s", owner, strings.TrimSpace(r[ingCol])))
				return nil, false
			case len(matches) > 1:
				p.Report.Ambiguous = append(p.Report.Ambiguous, "insumo "+norm(r[ingCol]))
				return nil, false
			}
			ing := matches[0]
			u, ok := cat.Units[unitCode(r["Unidad"])]
			if !ok || u.Kind != ing.BaseKind {
				p.Report.UnitMismatch = append(p.Report.UnitMismatch, fmt.Sprintf("%s: %s en %s", owner, ing.Name, strings.TrimSpace(r["Unidad"])))
				return nil, false
			}
			qty, err := decimal.NewFromString(strings.TrimSpace(r["Cantidad"]))
			if err != nil || !qty.IsPositive() {
				p.Report.InvalidQuantity = append(p.Report.InvalidQuantity, fmt.Sprintf("%s: %s", owner, ing.Name))
				return nil, false
			}
			base := qty.Mul(u.ToBase).Round(4)
			if i, ok := pos[ing.ID]; ok {
				out[i].Qty = out[i].Qty.Add(base)
				continue
			}
			pos[ing.ID] = len(out)
			out = append(out, Line{IngredientID: ing.ID, Qty: base, UnitID: baseUnit[ing.BaseKind]})
		}
		return out, len(out) > 0
	}

	recipesByName := map[string]recipe{}
	for _, rc := range collect(src.Recipes, "Producto") {
		recipesByName[norm(rc.name)] = rc
	}
	usedByOption := map[string]bool{}

	// Extras: primero el producto que son; si no, su receta.
	for _, o := range cat.Options {
		k := norm(o.Name)
		if o.HasComposition {
			if _, ok := recipesByName[k]; ok {
				p.Report.AlreadyCaptured++
			}
			usedByOption[k] = true
			continue
		}
		if ps := products[k]; len(ps) == 1 {
			p.OptionLinks[o.ID] = ps[0].ID
			usedByOption[k] = true
			continue
		}
		rc, ok := recipesByName[k]
		if !ok {
			continue
		}
		usedByOption[k] = true
		if ls, ok := lines(o.Name, rc.rows, "Ingrediente"); ok {
			p.OptionRecipes[o.ID] = ls
		}
	}

	// Productos.
	for _, rc := range collect(src.Recipes, "Producto") {
		k := norm(rc.name)
		ps := products[k]
		switch {
		case len(ps) > 1:
			p.Report.Ambiguous = append(p.Report.Ambiguous, k)
			continue
		case len(ps) == 0:
			if !usedByOption[k] {
				p.Report.NotInCatalog = append(p.Report.NotInCatalog, rc.name)
			}
			continue
		}
		pr := ps[0]
		if pr.HasComposition || pr.TrackStock || pr.IsCombo {
			p.Report.AlreadyCaptured++
			continue
		}
		if ls, ok := lines(rc.name, rc.rows, "Ingrediente"); ok {
			p.ProductRecipes[pr.ID] = ls
		}
	}

	preps := prepGraph{domain.StockGraph{Ingredients: map[int64]domain.StockIngredient{}, Recipes: map[int64][]domain.RecipeLine{}}}
	for id, comps := range cat.PrepComponents {
		preps.addPrep(id, comps)
	}

	// Insumos compuestos: la receta de FUDO es por UNA unidad del insumo, así que rinde esa unidad
	// expresada en la base del POS (1 L → 1000 ml).
	for _, rc := range collect(src.SubIngredients, "Ingrediente") {
		k := norm(rc.name)
		is := ingredients[k]
		switch {
		case len(is) > 1:
			p.Report.Ambiguous = append(p.Report.Ambiguous, "insumo "+k)
			continue
		case len(is) == 0:
			p.Report.MissingIngredients = append(p.Report.MissingIngredients, rc.name)
			continue
		}
		in := is[0]
		if in.HasComposition {
			p.Report.AlreadyCaptured++
			continue
		}
		u, ok := cat.Units[unitCode(fudoUnitOf(src.IngredientUnits, rc.name))]
		if !ok || u.Kind != in.BaseKind {
			p.Report.UnitMismatch = append(p.Report.UnitMismatch, fmt.Sprintf("%s: su unidad en FUDO no es de su tipo", rc.name))
			continue
		}
		if ls, ok := lines(rc.name, rc.rows, "Subingrediente"); ok {
			components := make([]int64, 0, len(ls))
			for _, l := range ls {
				components = append(components, l.IngredientID)
			}
			// La misma regla que la captura a mano: ni directo (A lleva A) ni indirecto (A lleva B y B
			// lleva A), contando lo que ya había en el POS y lo que esta carga ya aceptó.
			if err := preps.ValidatePrepIngredient(in.ID, components); err != nil {
				p.Report.Cycles = append(p.Report.Cycles, rc.name)
				continue
			}
			preps.addPrep(in.ID, components)
			p.PrepIngredients[in.ID] = Prep{Lines: ls, Yield: u.ToBase}
		}
	}

	for _, rc := range collect(src.SubProducts, "Producto") {
		p.Report.Packages = append(p.Report.Packages, rc.name)
	}
	return p
}

// prepGraph es el grafo de insumos compuestos que valida los ciclos. Cada insumo usa su propio id
// como id de receta: aquí solo importa quién lleva a quién, no cuánto.
type prepGraph struct{ domain.StockGraph }

func (g prepGraph) addPrep(id int64, components []int64) {
	rid := id
	g.Ingredients[id] = domain.StockIngredient{ID: id, IsPrep: true, RecipeID: &rid, YieldQty: decimal.NewFromInt(1)}
	lines := make([]domain.RecipeLine, 0, len(components))
	for _, c := range components {
		lines = append(lines, domain.RecipeLine{IngredientID: c, QtyBase: decimal.NewFromInt(1)})
	}
	g.Recipes[rid] = lines
}

func fudoUnitOf(units map[string]string, name string) string {
	for n, u := range units {
		if norm(n) == norm(name) {
			return u
		}
	}
	return ""
}
