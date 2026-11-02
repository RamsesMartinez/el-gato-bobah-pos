package app

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
