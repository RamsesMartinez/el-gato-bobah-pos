// Command fudo-import carga el catálogo real de FUDO (references/csv/*.csv) al esquema propio.
// Es para el setup inicial: limpia el catálogo y lo reimporta. Aborta si ya hay órdenes
// (para no romper un sistema en uso) salvo --force.
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

func main() {
	dir := flag.String("dir", "../references", "carpeta con los exports FUDO (usa dir/csv/*.csv)")
	force := flag.Bool("force", false, "reimportar aunque existan órdenes (borra el catálogo)")
	flag.Parse()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL requerido")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("conexión: %v", err)
	}
	defer pool.Close()

	imp := &importer{pool: pool, csvDir: filepath.Join(*dir, "csv")}
	if err := imp.run(ctx, *force); err != nil {
		log.Fatalf("importación falló: %v", err)
	}
}

type importer struct {
	pool   *pgxpool.Pool
	csvDir string

	unitByCode map[string]unit          // code -> {id, kind}
	ingByName  map[string]ingredientRef // nombre normalizado -> ingrediente
	catByKey   map[string]int64         // "cat" o "cat|subcat" -> id
	prodByName map[string]int64         // nombre normalizado -> product id
	groupByF   map[string]int64         // fudo group id -> new group id
	supByName  map[string]int64         // nombre -> supplier id
	fudoCost   map[string]float64       // product name norm -> costo FUDO
}

type unit struct {
	id     int16
	kind   string
	toBase float64
}
type ingredientRef struct {
	id       int64
	baseKind string
}

func (im *importer) run(ctx context.Context, force bool) error {
	// guard
	var orders int
	if err := im.pool.QueryRow(ctx, "select count(*) from orders").Scan(&orders); err != nil {
		return err
	}
	if orders > 0 && !force {
		return fmt.Errorf("hay %d órdenes; usa --force para reimportar (borra el catálogo)", orders)
	}

	if err := im.wipe(ctx); err != nil {
		return fmt.Errorf("limpieza: %w", err)
	}
	if err := im.loadUnits(ctx); err != nil {
		return err
	}
	im.ingByName = map[string]ingredientRef{}
	im.catByKey = map[string]int64{}
	im.prodByName = map[string]int64{}
	im.groupByF = map[string]int64{}
	im.supByName = map[string]int64{}
	im.fudoCost = map[string]float64{}

	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"ingredientes", im.importIngredients},
		{"productos", im.importProducts},
		{"recetas", im.importRecipes},
		{"modificadores", im.importModifiers},
	}
	for _, s := range steps {
		if err := s.fn(ctx); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}

	// recomputar costos con el motor y validar contra la columna Costo de FUDO
	st := &store.Store{Pool: im.pool, Q: db.New(im.pool)}
	costing := app.NewCostingService(st)
	if err := costing.RecomputeAll(ctx); err != nil {
		return fmt.Errorf("costeo: %w", err)
	}
	return im.validateCosts(ctx)
}

func (im *importer) wipe(ctx context.Context) error {
	_, err := im.pool.Exec(ctx, `
		truncate table
			fudo_import_map, product_modifier_groups, modifier_options, modifier_groups,
			combo_slot_products, combo_slots, product_channels, products, categories,
			recipe_items, ingredient_purchase_formats, ingredients, ingredient_categories, recipes,
			suppliers
		restart identity cascade`)
	return err
}

func (im *importer) loadUnits(ctx context.Context) error {
	im.unitByCode = map[string]unit{}
	rows, err := im.pool.Query(ctx, "select id, code, kind, to_base from units")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int16
		var code, kind string
		var toBase float64
		if err := rows.Scan(&id, &code, &kind, &toBase); err != nil {
			return err
		}
		im.unitByCode[code] = unit{id: id, kind: kind, toBase: toBase}
	}
	return rows.Err()
}

// baseUnitFor devuelve la unidad canónica base de un kind (g / ml / pieza).
func (im *importer) baseUnitFor(kind string) int16 {
	switch kind {
	case "masa":
		return im.unitByCode["g"].id
	case "volumen":
		return im.unitByCode["ml"].id
	default:
		return im.unitByCode["pieza"].id
	}
}

// ---- ingredientes ----

func (im *importer) importIngredients(ctx context.Context) error {
	rows, err := readCSV(filepath.Join(im.csvDir, "ingredientes.csv"))
	if err != nil {
		return err
	}
	catCache := map[string]int64{}
	for _, r := range rows {
		name := norm(r["Nombre"])
		if name == "" {
			continue
		}
		u := im.unitFor(r["Unidad"])
		catID := im.ingredientCategory(ctx, catCache, r["Categoría"])
		supID := im.supplier(ctx, r["Proveedor"])
		baseUnit := im.baseUnitFor(u.kind)
		// FUDO Costo es por la Unidad del ingrediente (ej. $/kg); el motor trabaja en
		// base del kind (g/ml/pieza), así que normalizamos: costo por base = Costo / to_base.
		costPerBase := pf(r["Costo"]) / u.toBase

		var id int64
		err := im.pool.QueryRow(ctx, `
			insert into ingredients (name, category_id, base_unit_id, waste_pct, current_cost,
			                         cost_source, supplier_id, track_stock, is_active)
			values ($1,$2,$3,$4,$5,'manual',$6,$7,true)
			returning id`,
			r["Nombre"], nullID(catID), baseUnit, clampPct(pf(r["Merma"])), costPerBase,
			nullID(supID), boolSi(r["Control de Stock"]),
		).Scan(&id)
		if err != nil {
			return err
		}
		im.ingByName[name] = ingredientRef{id: id, baseKind: u.kind}
	}
	log.Printf("ingredientes: %d", len(im.ingByName))
	return nil
}

func (im *importer) ingredientCategory(ctx context.Context, cache map[string]int64, name string) int64 {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0
	}
	if id, ok := cache[name]; ok {
		return id
	}
	var id int64
	_ = im.pool.QueryRow(ctx, `insert into ingredient_categories (name) values ($1) returning id`, name).Scan(&id)
	cache[name] = id
	return id
}

func (im *importer) supplier(ctx context.Context, name string) int64 {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0
	}
	key := norm(name)
	if id, ok := im.supByName[key]; ok {
		return id
	}
	var id int64
	_ = im.pool.QueryRow(ctx, `insert into suppliers (name) values ($1) returning id`, name).Scan(&id)
	im.supByName[key] = id
	return id
}

// ---- productos ----

func (im *importer) importProducts(ctx context.Context) error {
	prodRows, err := readCSV(filepath.Join(im.csvDir, "productos.csv"))
	if err != nil {
		return err
	}
	modRows, err := readCSV(filepath.Join(im.csvDir, "mod_productos.csv"))
	if err != nil {
		return err
	}
	// nombres que son opciones de modificador
	modOptionNames := map[string]bool{}
	for _, r := range modRows {
		modOptionNames[norm(r["Producto"])] = true
	}

	imported, skipped := 0, 0
	for _, r := range prodRows {
		name := r["Nombre"]
		key := norm(name)
		if key == "" {
			continue
		}
		cat := strings.TrimSpace(r["Categoría"])
		sellAlone := boolSi(r["Permitir vender solo"])
		// regla: si es opción de modificador y NO se vende solo (o es de "Otro"), no es producto
		if modOptionNames[key] && (!sellAlone || strings.EqualFold(cat, "Otro")) {
			skipped++
			continue
		}
		catID := im.category(ctx, cat, r["Subcategoría"])
		costSource := "manual" // se cambia a 'receta' en importRecipes si aplica
		manualCost := pf(r["Costo"])

		var id int64
		err := im.pool.QueryRow(ctx, `
			insert into products (name, sku, category_id, price, cost_source, manual_cost,
			                      track_stock, allow_oversell, is_favorite, sort_key, is_active)
			values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			returning id`,
			name, nullStr(r["Código"]), catID, pf(r["Precio"]), costSource, manualCost,
			boolSi(r["Control de Stock"]), boolSi(r["Vender sin stock"]),
			boolSi(r["Favorito"]), sortKey(r["Posición"]), boolSi(r["Activo"]),
		).Scan(&id)
		if err != nil {
			return err
		}
		im.prodByName[key] = id
		im.fudoCost[key] = pf(r["Costo"])
		imported++
	}
	log.Printf("productos: %d importados, %d omitidos (opciones de modificador)", imported, skipped)
	return nil
}

func (im *importer) category(ctx context.Context, cat, subcat string) int64 {
	cat = strings.TrimSpace(cat)
	if cat == "" {
		cat = "Sin categoría"
	}
	rootID := im.getOrCreateCategory(ctx, cat, nil)
	subcat = strings.TrimSpace(subcat)
	if subcat == "" {
		return rootID
	}
	return im.getOrCreateCategory(ctx, subcat, &rootID)
}

func (im *importer) getOrCreateCategory(ctx context.Context, name string, parent *int64) int64 {
	key := name
	if parent != nil {
		key = fmt.Sprintf("%d|%s", *parent, name)
	}
	if id, ok := im.catByKey[key]; ok {
		return id
	}
	var id int64
	_ = im.pool.QueryRow(ctx,
		`insert into categories (name, parent_id) values ($1,$2) returning id`, name, parent).Scan(&id)
	im.catByKey[key] = id
	return id
}

// ---- recetas ----

func (im *importer) importRecipes(ctx context.Context) error {
	rows, err := readCSV(filepath.Join(im.csvDir, "recetas.csv"))
	if err != nil {
		return err
	}
	// agrupar por producto
	byProduct := map[string][]map[string]string{}
	order := []string{}
	for _, r := range rows {
		k := norm(r["Producto"])
		if _, ok := byProduct[k]; !ok {
			order = append(order, k)
		}
		byProduct[k] = append(byProduct[k], r)
	}

	recipes, items, skippedLines := 0, 0, 0
	for _, pkey := range order {
		prodID, ok := im.prodByName[pkey]
		if !ok {
			continue // el producto no se migró (era opción de modificador)
		}
		// crear receta
		var recipeID int64
		if err := im.pool.QueryRow(ctx, `insert into recipes default values returning id`).Scan(&recipeID); err != nil {
			return err
		}
		added := 0
		for _, line := range byProduct[pkey] {
			ing, ok := im.ingByName[norm(line["Ingrediente"])]
			if !ok {
				skippedLines++
				continue
			}
			u := im.unitFor(line["Unidad"])
			if u.kind != ing.baseKind {
				skippedLines++ // el trigger rechazaría distinto kind
				continue
			}
			qty := pf(line["Cantidad"])
			if qty <= 0 {
				skippedLines++
				continue
			}
			_, err := im.pool.Exec(ctx, `
				insert into recipe_items (recipe_id, ingredient_id, quantity, unit_id)
				values ($1,$2,$3,$4) on conflict (recipe_id, ingredient_id) do nothing`,
				recipeID, ing.id, qty, u.id)
			if err != nil {
				return err
			}
			added++
			items++
		}
		if added == 0 {
			_, _ = im.pool.Exec(ctx, `delete from recipes where id=$1`, recipeID)
			continue
		}
		// enlazar receta al producto; los productos con receta derivan disponibilidad,
		// no llevan stock directo (check del schema: not track_stock or recipe_id is null)
		if _, err := im.pool.Exec(ctx,
			`update products set recipe_id=$1, cost_source='receta', track_stock=false where id=$2`,
			recipeID, prodID); err != nil {
			return err
		}
		recipes++
	}
	log.Printf("recetas: %d productos, %d líneas (%d líneas omitidas)", recipes, items, skippedLines)
	return nil
}

// ---- modificadores ----

func (im *importer) importModifiers(ctx context.Context) error {
	grpRows, err := readCSV(filepath.Join(im.csvDir, "mod_grupos.csv"))
	if err != nil {
		return err
	}
	optRows, err := readCSV(filepath.Join(im.csvDir, "mod_productos.csv"))
	if err != nil {
		return err
	}

	// grupos: crear uno por cada ID de grupo FUDO visto
	seen := map[string]bool{}
	ensureGroup := func(fid string) int64 {
		fid = strings.TrimSpace(fid)
		if fid == "" {
			return 0
		}
		if id, ok := im.groupByF[fid]; ok {
			return id
		}
		var id int64
		_ = im.pool.QueryRow(ctx,
			`insert into modifier_groups (name) values ($1) returning id`, "Grupo "+fid).Scan(&id)
		im.groupByF[fid] = id
		return id
	}

	// opciones
	opts := 0
	optSeen := map[string]bool{} // groupid|nombre
	for _, r := range optRows {
		gid := ensureGroup(r["ID Grupo modificador"])
		if gid == 0 {
			continue
		}
		name := strings.TrimSpace(r["Producto"])
		if name == "" {
			continue
		}
		dk := fmt.Sprintf("%d|%s", gid, norm(name))
		if optSeen[dk] {
			continue
		}
		optSeen[dk] = true
		_, err := im.pool.Exec(ctx, `
			insert into modifier_options (group_id, name, price_delta, max_per_line)
			values ($1,$2,$3,$4) on conflict (group_id, name) do nothing`,
			gid, name, pf(r["Precio"]), maxQty(r["Máxima cantidad"]))
		if err != nil {
			return err
		}
		opts++
	}

	// attachments: producto dueño -> grupo
	att := 0
	for _, r := range grpRows {
		owner := norm(r["Modificador"])
		prodID, ok := im.prodByName[owner]
		if !ok {
			continue
		}
		gid := ensureGroup(r["ID Grupo modificador"])
		if gid == 0 {
			continue
		}
		attKey := fmt.Sprintf("%d|%d", prodID, gid)
		if seen[attKey] {
			continue
		}
		seen[attKey] = true
		_, err := im.pool.Exec(ctx, `
			insert into product_modifier_groups (product_id, group_id, title, min_select, max_select)
			values ($1,$2,$3,$4,$5) on conflict (product_id, group_id) do nothing`,
			prodID, gid, nullStr(r["Título"]), minMax(r["Mínima cantidad"], 0), minMax(r["Máxima cantidad"], 1))
		if err != nil {
			return err
		}
		att++
	}
	log.Printf("modificadores: %d grupos, %d opciones, %d asignaciones", len(im.groupByF), opts, att)
	return nil
}

// ---- validación de costos vs FUDO ----

func (im *importer) validateCosts(ctx context.Context) error {
	rows, err := im.pool.Query(ctx,
		`select name, current_cost, cost_source from products where cost_source='receta'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var checked, within, over int
	var worst float64
	var worstName string
	for rows.Next() {
		var name string
		var cost float64
		var src string
		if err := rows.Scan(&name, &cost, &src); err != nil {
			return err
		}
		fc, ok := im.fudoCost[norm(name)]
		if !ok || fc == 0 {
			continue
		}
		checked++
		d := math.Abs(cost - fc)
		if d <= 0.5 {
			within++
		} else {
			over++
			if d > worst {
				worst, worstName = d, name
			}
		}
	}
	log.Printf("validación costos (recetas): %d revisados, %d dentro de $0.50, %d fuera (peor: %s Δ$%.2f)",
		checked, within, over, worstName, worst)
	return rows.Err()
}

// ---- helpers ----

func (im *importer) unitFor(s string) unit {
	code := normUnit(s)
	if u, ok := im.unitByCode[code]; ok {
		return u
	}
	return im.unitByCode["pieza"] // fallback
}

var spaceRe = regexp.MustCompile(`\s+`)

func norm(s string) string {
	return strings.ToUpper(spaceRe.ReplaceAllString(strings.TrimSpace(s), " "))
}

func normUnit(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
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
	case "unid.", "unid", "unidad", "u", "pza", "pieza", "pzas", "":
		return "pieza"
	}
	return "pieza"
}

func readCSV(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	rd := csv.NewReader(f)
	rd.FieldsPerRecord = -1
	header, err := rd.Read()
	if err != nil {
		return nil, err
	}
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
	}
	var out []map[string]string
	for {
		rec, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		m := make(map[string]string, len(header))
		for i, h := range header {
			if i < len(rec) {
				m[h] = rec[i]
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func pf(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

func boolSi(s string) bool { return strings.EqualFold(strings.TrimSpace(s), "Si") }

func nullStr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "nan") {
		return nil
	}
	return &s
}

func nullID(id int64) *int64 {
	if id == 0 {
		return nil
	}
	return &id
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v >= 100 {
		return 99.99
	}
	return v
}

func sortKey(s string) float64 {
	v := pf(s)
	if v == 0 {
		return 1000
	}
	return v / 1e6 // FUDO usa enteros espaciados grandes
}

func maxQty(s string) int16 {
	v := int(pf(s))
	if v < 1 {
		return 1
	}
	if v > 32767 {
		return 32767
	}
	return int16(v)
}

func minMax(s string, def int) int16 {
	t := strings.TrimSpace(s)
	if t == "" || strings.EqualFold(t, "nan") {
		return int16(def)
	}
	v := max(int(pf(s)), 0)
	if v > 32767 {
		v = 32767
	}
	return int16(v)
}
