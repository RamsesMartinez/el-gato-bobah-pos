package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Las herramientas de catálogo de la spec 090: costo de un insumo con motivo, recetas, grupos de
// modificadores y sus opciones, la liga de un grupo con un producto, y el alta de productos. Cada una
// manda el cuerpo de la pantalla a su ruta; nombres repetidos, mínimos imposibles y recetas viejas los
// rechaza el servicio.

//nolint:gosec // G101: patrones de rutas; gosec confunde palabras con credenciales.
const (
	pArticleCost        = "/api/v1/stock/articles/{id}/cost"
	pArticleCostHistory = "/api/v1/stock/articles/{id}/cost-history"
	pProductComposition = "/api/v1/admin/products/{id}/composition"
	pOptionComposition  = "/api/v1/admin/modifier-options/{id}/composition"
	pCategories         = "/api/v1/admin/categories"
	pGroups             = "/api/v1/admin/groups/"
	pGroup              = "/api/v1/admin/groups/{id}"
	pGroupProducts      = "/api/v1/admin/groups/{id}/products"
	pProductGroups      = "/api/v1/admin/products/{id}/groups"
	pProductGroup       = "/api/v1/admin/products/{id}/groups/{groupId}"
)

func init() {
	for name, routes := range map[string][]mcpRoute{
		"ingredient_cost_set": {{http.MethodPost, pArticleCost}, get(pArticle), get(pArticleCostHistory)},
		"recipe_get":          {get(pProductComposition), get(pOptionComposition)},
		"recipe_set": {{http.MethodPut, pProductComposition}, {http.MethodPut, pOptionComposition},
			get(pProductComposition), get(pOptionComposition), get(pArticle)},
		"categories_list":        {get(pCategories)},
		"modifier_groups_list":   {get(pGroups)},
		"modifier_group_create":  {{http.MethodPost, pGroups}, get(pGroups)},
		"modifier_group_update":  {{http.MethodPatch, pGroup}, get(pGroups), get(pGroupProducts)},
		"modifier_option_create": {{http.MethodPost, pGroupOptions}, get(pGroups)},
		"product_groups_get":     {get(pProductGroups)},
		"product_group_link":     {{http.MethodPost, pProductGroups}, get(pProductGroups), get(pGroups)},
		"product_group_unlink":   {{http.MethodDelete, pProductGroup}, get(pProductGroups)},
		"product_create":         {{http.MethodPost, pProducts}, get(pProducts), get(pCategories)},
	} {
		mcpToolRoutes[name] = routes
	}
	for k, v := range map[string]string{
		"ingredient_cost_set": "stock_levels", "recipe_get": "products_list", "recipe_set": "products_list",
		"modifier_group_update": "modifier_groups_list", "modifier_option_create": "modifier_groups_list",
		"product_groups_get": "products_list", "product_group_link": "products_list",
		"product_group_unlink": "product_groups_get", "product_create": "categories_list",
	} {
		mcpListTool[k] = v
	}
	register090Catalog()
}

// ---- Argumentos ----

type costSetArgs struct {
	writeCommon
	IngredientID int64  `json:"ingredient_id" jsonschema:"insumo (de stock_levels o articles_search)"`
	Cost         string `json:"cost" jsonschema:"costo por unidad base (g, ml, cm o pieza), texto decimal de hasta 6 decimales"`
	Reason       string `json:"reason" jsonschema:"por qué se corrige (queda en el historial), de 1 a 200 letras"`
}

type recipeTarget struct {
	ProductID int64 `json:"product_id,omitempty" jsonschema:"producto; o option_id"`
	OptionID  int64 `json:"option_id,omitempty" jsonschema:"extra; o product_id"`
}

func (r recipeTarget) pattern() (string, int64, error) {
	if (r.ProductID == 0) == (r.OptionID == 0) || r.ProductID < 0 || r.OptionID < 0 {
		return "", 0, toolErr("Manda exactamente uno de product_id u option_id.")
	}
	if r.OptionID != 0 {
		return pOptionComposition, r.OptionID, nil
	}
	return pProductComposition, r.ProductID, nil
}

type recipeItemArg struct {
	IngredientID int64  `json:"ingredient_id" jsonschema:"insumo"`
	Quantity     string `json:"quantity" jsonschema:"cuánto lleva, texto decimal, en unit_id"`
	UnitID       int16  `json:"unit_id" jsonschema:"unidad (de units_list)"`
}

type recipeComponentArg struct {
	ProductID int64 `json:"product_id" jsonschema:"producto del paquete"`
	Quantity  int   `json:"quantity" jsonschema:"cuántos"`
}

type recipeSetArgs struct {
	writeCommon
	recipeTarget
	Mode            string               `json:"mode" jsonschema:"items (insumos), package (productos de un paquete), product (un extra que es otro producto) o nothing (no se cuenta)"`
	Items           []recipeItemArg      `json:"items,omitempty" jsonschema:"insumos, con mode items"`
	LinkedProductID *int64               `json:"linked_product_id,omitempty" jsonschema:"el producto que es este extra, con mode product"`
	Components      []recipeComponentArg `json:"components,omitempty" jsonschema:"productos del paquete, con mode package"`
	BasedOn         *string              `json:"based_on,omitempty" jsonschema:"el stamp de recipe_get o de la vista previa; obligatorio al aplicar (\"\" si no había receta)"`
	AlsoInSameName  bool                 `json:"also_in_same_name,omitempty" jsonschema:"true también guarda la receta en los extras que se llaman igual en otros grupos; por omisión false"`
	AlsoBasedOn     map[string]string    `json:"also_based_on,omitempty" jsonschema:"con also_in_same_name, obligatorio al aplicar: el also_based_on de la vista previa (los extras y su stamp)"`
	ActualStock     *string              `json:"actual_stock,omitempty" jsonschema:"si la vista previa lo pide: cuántas hay de verdad, lo dice la persona"`
	SeenProduct     *string              `json:"seen_product_on_hand,omitempty" jsonschema:"si la vista previa lo pide: la existencia del producto que mostró"`
	SeenIngredient  *string              `json:"seen_ingredient_on_hand,omitempty" jsonschema:"si la vista previa lo pide: la existencia del insumo que mostró"`
}

type groupsListArgs struct {
	Search string `json:"search,omitempty" jsonschema:"parte del nombre del grupo"`
	Status string `json:"status,omitempty" jsonschema:"act (activos) o inact (inactivos); vacío = todos"`
}

type groupCreateArgs struct {
	writeCommon
	Name       string `json:"name" jsonschema:"nombre del grupo"`
	DefaultMin int    `json:"default_min,omitempty" jsonschema:"mínimo por omisión (0 = opcional)"`
	DefaultMax int    `json:"default_max,omitempty" jsonschema:"máximo por omisión; vacío = 1"`
}

type groupUpdateArgs struct {
	writeCommon
	GroupID    int64   `json:"group_id" jsonschema:"grupo (de modifier_groups_list)"`
	Name       *string `json:"name,omitempty" jsonschema:"nombre nuevo"`
	DefaultMin *int    `json:"default_min,omitempty" jsonschema:"mínimo por omisión"`
	DefaultMax *int    `json:"default_max,omitempty" jsonschema:"máximo por omisión"`
	Active     *bool   `json:"active,omitempty" jsonschema:"false lo archiva (deja de salir en la venta); true lo reactiva"`
}

type optionCreateArgs struct {
	writeCommon
	GroupID    int64   `json:"group_id" jsonschema:"grupo (de modifier_groups_list)"`
	Name       string  `json:"name" jsonschema:"nombre del extra; no se repite en el grupo"`
	PriceDelta *string `json:"price_delta,omitempty" jsonschema:"lo que suma al precio, texto decimal; vacío = 0"`
	MaxPerLine *int    `json:"max_per_line,omitempty" jsonschema:"cuántas veces cabe en un renglón; vacío = 1"`
}

type categoriesListArgs struct {
	IncludeInactive bool `json:"include_inactive,omitempty" jsonschema:"true agrega las categorías apagadas (para reactivarlas)"`
}

type productIDArgs struct {
	ProductID int64 `json:"product_id" jsonschema:"producto (de products_list)"`
}

type groupLinkArgs struct {
	writeCommon
	ProductID int64  `json:"product_id" jsonschema:"producto (de products_list)"`
	GroupID   int64  `json:"group_id" jsonschema:"grupo (de modifier_groups_list)"`
	Title     string `json:"title,omitempty" jsonschema:"cómo se llama el grupo en este producto; vacío = su nombre"`
	MinSelect *int   `json:"min_select,omitempty" jsonschema:"mínimo en este producto; con max_select, o sin los dos para heredar los del grupo"`
	MaxSelect *int   `json:"max_select,omitempty" jsonschema:"máximo en este producto; con min_select"`
	Position  int    `json:"position,omitempty" jsonschema:"orden del grupo en el producto"`
}

type groupUnlinkArgs struct {
	writeCommon
	ProductID int64 `json:"product_id" jsonschema:"producto"`
	GroupID   int64 `json:"group_id" jsonschema:"grupo que se quita"`
}

type productCreateArgs struct {
	writeCommon
	Name       string  `json:"name" jsonschema:"nombre del producto"`
	CategoryID int64   `json:"category_id" jsonschema:"categoría (de categories_list)"`
	Price      string  `json:"price" jsonschema:"precio, texto decimal"`
	Cost       *string `json:"cost,omitempty" jsonschema:"costo manual, texto decimal; vacío = sin costo capturado"`
	TrackStock bool    `json:"track_stock,omitempty" jsonschema:"true si cuenta su propia existencia (producto de reventa)"`
	Favorite   bool    `json:"favorite,omitempty" jsonschema:"true lo pone en favoritos de la pantalla de venta"`
}

// ---- Ayudas ----

// withFix agrega a un rechazo de la ruta la corrección para el agente, que no ve la hoja de la
// pantalla.
func withFix(resp []byte, fix string) []byte {
	var env errorEnvelope
	if json.Unmarshal(resp, &env) != nil {
		return resp
	}
	env.Error.Message = strings.TrimRight(strings.TrimSpace(env.Error.Message), ".") + ". " + fix
	b, err := json.Marshal(env)
	if err != nil {
		return resp
	}
	return b
}

// groupNow lee un grupo de la lista completa (con inactivos).
func groupNow(s *mcpSession, ctx context.Context, tool string, id int64) (map[string]any, error) {
	b, _, err := s.fetch(ctx, tool, pGroups, nil, url.Values{"limit": {"0"}})
	if err != nil {
		return nil, err
	}
	g := findByID(b, id)
	if g == nil {
		return nil, toolErr("No existe el grupo %d en esta empresa. Búscalo con modifier_groups_list.", id)
	}
	return g, nil
}

func groupInactiveNote(g map[string]any) []string {
	if active, _ := g["isActive"].(bool); !active {
		return []string{fmt.Sprintf("El grupo %v está inactivo: no sale en la pantalla de venta. Reactívalo con "+
			"modifier_group_update y active: true.", g["name"])}
	}
	return nil
}

// similarNames devuelve los renglones de una lista cuyo nombre contiene al buscado o al revés.
func similarNames(b json.RawMessage, name string) []map[string]any {
	var env struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(b, &env)
	want := strings.ToLower(strings.TrimSpace(name))
	var out []map[string]any
	for _, it := range env.Items {
		n, _ := it["name"].(string)
		l := strings.ToLower(n)
		if want != "" && (strings.Contains(l, want) || strings.Contains(want, l)) {
			out = append(out, map[string]any{"id": it["id"], "name": n})
		}
	}
	return out
}

func register090Catalog() {
	// ---- Costo con motivo ----
	writeTool(writeSpec[costSetArgs]{
		name: "ingredient_cost_set",
		refs: refsCostSet,
		route: func(_ *mcpSession, _ context.Context, in costSetArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("ingredient_id", in.IngredientID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if strings.TrimSpace(in.Cost) == "" {
				return "", "", nil, nil, nil, toolErr("cost es obligatorio: el costo por unidad base, como \"0.0125\".")
			}
			if err := requireReason(in.Reason, "corrige el costo"); err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPost, pArticleCost, ids(in.IngredientID), nil, map[string]any{"cost": in.Cost, "reason": strings.TrimSpace(in.Reason)}, nil
		},
		state: func(s *mcpSession, ctx context.Context, in costSetArgs, _ json.RawMessage) any {
			return s.soft(ctx, "ingredient_cost_set", pArticle, ids(in.IngredientID), nil)
		},
		preview: func(s *mcpSession, ctx context.Context, in costSetArgs) (any, []string) {
			hist := s.soft(ctx, "ingredient_cost_set", pArticleCostHistory, ids(in.IngredientID), nil)
			return map[string]any{"cost_history": hist}, []string{"El costo es por unidad base; los productos, extras y " +
				"preparados que lo llevan recalculan su costo al aplicar. No mete mercancía ni crea un gasto."}
		},
	})

	// ---- Recetas ----
	readTool("recipe_get", func(s *mcpSession, ctx context.Context, in recipeTarget) (any, error) {
		pattern, id, err := in.pattern()
		if err != nil {
			return nil, err
		}
		b, _, err := s.fetch(ctx, "recipe_get", pattern, ids(id), nil)
		if err != nil {
			return nil, err
		}
		return forAgent(b), nil
	})

	recipeNow := func(s *mcpSession, ctx context.Context, in recipeSetArgs) (map[string]any, error) {
		pattern, id, err := in.pattern()
		if err != nil {
			return nil, err
		}
		b, _, err := s.fetch(ctx, "recipe_set", pattern, ids(id), nil)
		if err != nil {
			return nil, err
		}
		var v map[string]any
		_ = json.Unmarshal(b, &v)
		return v, nil
	}
	recipeBody := func(in recipeSetArgs) map[string]any {
		items := make([]map[string]any, len(in.Items))
		for i, it := range in.Items {
			items[i] = map[string]any{"ingredientId": it.IngredientID, "quantity": it.Quantity, "unitId": it.UnitID}
		}
		comps := make([]map[string]any, len(in.Components))
		for i, c := range in.Components {
			comps[i] = map[string]any{"productId": c.ProductID, "quantity": c.Quantity}
		}
		body := map[string]any{"mode": in.Mode, "items": items, "components": comps, "linkedProductId": in.LinkedProductID,
			"basedOn": in.BasedOn, "alsoOptionIds": []int64{}, "actualStock": in.ActualStock,
			"seenProductOnHand": in.SeenProduct, "seenIngredientOnHand": in.SeenIngredient}
		// Por omisión NO se copia a los homónimos (la trampa de «También en…»). Si se pide, van los extras y
		// los stamps que la vista previa enseñó, no los de ahora: así una receta que alguien les guardó en
		// medio se rechaza en vez de pisarse, y un homónimo que apareció en medio no recibe nada.
		if in.AlsoInSameName {
			idsOut := []int64{}
			stamps := map[string]string{}
			for k, st := range in.AlsoBasedOn {
				id, err := strconv.ParseInt(k, 10, 64)
				if err != nil || id <= 0 {
					continue
				}
				idsOut = append(idsOut, id)
				stamps[k] = st
			}
			body["alsoOptionIds"], body["alsoBasedOn"] = idsOut, stamps
		}
		return body
	}
	writeTool(writeSpec[recipeSetArgs]{
		name: "recipe_set",
		refs: refsRecipeSet,
		route: func(s *mcpSession, ctx context.Context, in recipeSetArgs) (string, string, map[string]int64, url.Values, any, error) {
			pattern, id, err := in.pattern()
			if err != nil {
				return "", "", nil, nil, nil, err
			}
			switch in.Mode {
			case "items", "package", "product", "nothing":
			default:
				return "", "", nil, nil, nil, toolErr("mode es items, package, product o nothing.")
			}
			// Leerla aquí hace que un id que no existe se diga antes de la vista previa.
			if _, err := recipeNow(s, ctx, in); err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPut, pattern, ids(id), nil, recipeBody(in), nil
		},
		state: func(s *mcpSession, ctx context.Context, in recipeSetArgs, _ json.RawMessage) any {
			cur, err := recipeNow(s, ctx, in)
			if err != nil {
				return map[string]string{"unavailable": err.Error()}
			}
			return cur
		},
		preview: func(s *mcpSession, ctx context.Context, in recipeSetArgs) (any, []string) {
			cur, err := recipeNow(s, ctx, in)
			if err != nil {
				return nil, []string{err.Error()}
			}
			out := map[string]any{"based_on": cur["stamp"]}
			var notes []string
			notes = append(notes, "Al aplicar manda based_on: \""+fmt.Sprint(cur["stamp"])+"\" (la receta como la ves ahora).")
			if twins, _ := cur["sameName"].([]any); len(twins) > 0 {
				if in.AlsoInSameName {
					out["also_saved_in"] = twins
					also := map[string]string{}
					for _, t := range twins {
						m, _ := t.(map[string]any)
						id, _ := m["id"].(float64)
						st, _ := m["stamp"].(string)
						also[strconv.FormatInt(int64(id), 10)] = st
					}
					out["also_based_on"] = also
					notes = append(notes, fmt.Sprintf("La receta también se guarda en %d extras con el mismo nombre. Al aplicar "+
						"manda also_based_on tal como viene aquí.", len(twins)))
				} else {
					notes = append(notes, fmt.Sprintf("Hay %d extras con el mismo nombre en otros grupos; NO se tocan. Para copiarles "+
						"esta receta manda also_in_same_name: true.", len(twins)))
				}
			}
			if own, _ := cur["ownStock"].(bool); own && in.ProductID != 0 {
				q := map[string]any{"product_on_hand": cur["productOnHand"]}
				if in.Mode == "items" && len(in.Items) == 1 {
					q["ingredient"] = s.soft(ctx, "recipe_set", pArticle, ids(in.Items[0].IngredientID), nil)
				}
				out["stock_question"] = q
				notes = append(notes, "El producto cuenta su propia existencia y deja de hacerlo con esta receta. Si el insumo "+
					"también tiene existencia, pregunta a la persona cuántas hay de verdad y manda actual_stock, más "+
					"seen_product_on_hand y seen_ingredient_on_hand con las cifras de stock_question.")
			}
			return out, notes
		},
		guard: func(_ *mcpSession, _ context.Context, in recipeSetArgs) error {
			if in.BasedOn == nil {
				return toolErr("Para aplicar hace falta based_on: el stamp que dio recipe_get o la vista previa (\"\" si no había receta).")
			}
			if in.AlsoInSameName && len(in.AlsoBasedOn) == 0 {
				return toolErr("Con also_in_same_name, aplicar pide also_based_on: el de la vista previa.")
			}
			return nil
		},
		run: func(s *mcpSession, ctx context.Context, in recipeSetArgs) (int, []byte, error) {
			pattern, id, _ := in.pattern()
			cur, err := recipeNow(s, ctx, in)
			if err != nil {
				return 0, nil, err
			}
			status, resp, err := s.dispatch(ctx, "recipe_set", http.MethodPut, pattern, ids(id), nil, recipeBody(in))
			switch {
			case err != nil:
			case status == http.StatusConflict:
				resp = withFix(resp, "Léela de nuevo con recipe_get y repite con su based_on.")
			case status == http.StatusBadRequest && cur["ownStock"] == true:
				resp = withFix(resp, "Pide la vista previa: trae la existencia del producto y del insumo y qué mandar.")
			}
			return status, resp, err
		},
	})

	// ---- Grupos ----
	simpleRead("categories_list", pCategories, func(in categoriesListArgs) (map[string]int64, url.Values, error) {
		if in.IncludeInactive {
			return nil, url.Values{"status": {"all"}}, nil
		}
		return nil, nil, nil
	})
	simpleRead("modifier_groups_list", pGroups, func(in groupsListArgs) (map[string]int64, url.Values, error) {
		q := url.Values{"limit": {"0"}}
		if in.Search != "" {
			q.Set("search", in.Search)
		}
		switch in.Status {
		case "":
		case "act", "inact":
			q.Set("status", in.Status)
		default:
			return nil, nil, toolErr("status es act, inact o vacío.")
		}
		return nil, q, nil
	})

	writeTool(writeSpec[groupCreateArgs]{
		name: "modifier_group_create",
		route: func(_ *mcpSession, _ context.Context, in groupCreateArgs) (string, string, map[string]int64, url.Values, any, error) {
			if strings.TrimSpace(in.Name) == "" {
				return "", "", nil, nil, nil, toolErr("name es obligatorio.")
			}
			return http.MethodPost, pGroups, nil, nil, map[string]any{"name": in.Name, "defaultMin": in.DefaultMin, "defaultMax": in.DefaultMax}, nil
		},
		preview: func(s *mcpSession, ctx context.Context, in groupCreateArgs) (any, []string) {
			b, _, err := s.fetch(ctx, "modifier_group_create", pGroups, nil, url.Values{"limit": {"0"}})
			if err != nil {
				return nil, nil
			}
			if sim := similarNames(b, in.Name); len(sim) > 0 {
				return attentionFirst{attention: []string{"Ya hay grupos con un nombre parecido: revisa si es uno de ellos antes de crear otro."},
					resolved: map[string]any{"similar": sim}}, nil
			}
			return nil, nil
		},
	})

	writeTool(writeSpec[groupUpdateArgs]{
		name: "modifier_group_update",
		refs: refsGroupUpdate,
		route: func(s *mcpSession, ctx context.Context, in groupUpdateArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("group_id", in.GroupID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if in.Name == nil && in.DefaultMin == nil && in.DefaultMax == nil && in.Active == nil {
				return "", "", nil, nil, nil, toolErr("Manda name, default_min, default_max o active.")
			}
			// La pantalla manda el grupo completo: se lee el actual y solo cambia lo pedido.
			g, err := groupNow(s, ctx, "modifier_group_update", in.GroupID)
			if err != nil {
				return "", "", nil, nil, nil, err
			}
			body := map[string]any{"name": g["name"], "active": g["isActive"], "defaultMin": g["defaultMin"], "defaultMax": g["defaultMax"]}
			if in.Name != nil {
				body["name"] = *in.Name
			}
			if in.Active != nil {
				body["active"] = *in.Active
			}
			if in.DefaultMin != nil {
				body["defaultMin"] = *in.DefaultMin
			}
			if in.DefaultMax != nil {
				body["defaultMax"] = *in.DefaultMax
			}
			return http.MethodPatch, pGroup, ids(in.GroupID), nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, in groupUpdateArgs, _ json.RawMessage) any {
			g, err := groupNow(s, ctx, "modifier_group_update", in.GroupID)
			if err != nil {
				return nil
			}
			return g
		},
		preview: func(s *mcpSession, ctx context.Context, in groupUpdateArgs) (any, []string) {
			if in.Active == nil || *in.Active {
				return nil, nil
			}
			b, _, err := s.fetch(ctx, "modifier_group_update", pGroupProducts, ids(in.GroupID), nil)
			if err != nil {
				return nil, []string{"No se pudo revisar en qué productos está: " + err.Error()}
			}
			var env struct {
				Items []map[string]any `json:"items"`
			}
			_ = json.Unmarshal(b, &env)
			var names []string
			for _, p := range env.Items {
				if active, _ := p["active"].(bool); active {
					names = append(names, fmt.Sprint(p["name"]))
				}
			}
			if len(names) == 0 {
				return nil, nil
			}
			what := fmt.Sprintf("%d productos activos", len(names))
			if len(names) == 1 {
				what = "1 producto activo"
			}
			return attentionFirst{attention: []string{fmt.Sprintf("Al archivarlo deja de ofrecerse en %s: %s.",
				what, strings.Join(names, ", "))}, resolved: map[string]any{"products": env.Items}}, nil
		},
	})

	writeTool(writeSpec[optionCreateArgs]{
		name: "modifier_option_create",
		refs: refsOptionCreate,
		route: func(_ *mcpSession, _ context.Context, in optionCreateArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("group_id", in.GroupID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if strings.TrimSpace(in.Name) == "" {
				return "", "", nil, nil, nil, toolErr("name es obligatorio.")
			}
			body := map[string]any{"name": in.Name, "priceDelta": "0", "maxPerLine": 0}
			if in.PriceDelta != nil {
				body["priceDelta"] = *in.PriceDelta
			}
			if in.MaxPerLine != nil {
				body["maxPerLine"] = *in.MaxPerLine
			}
			return http.MethodPost, pGroupOptions, ids(in.GroupID), nil, body, nil
		},
		preview: func(s *mcpSession, ctx context.Context, in optionCreateArgs) (any, []string) {
			g, err := groupNow(s, ctx, "modifier_option_create", in.GroupID)
			if err != nil {
				return nil, []string{err.Error()}
			}
			return map[string]any{"group": g}, groupInactiveNote(g)
		},
	})

	// ---- Liga producto ↔ grupo ----
	simpleRead("product_groups_get", pProductGroups, func(in productIDArgs) (map[string]int64, url.Values, error) {
		if err := positive("product_id", in.ProductID); err != nil {
			return nil, nil, err
		}
		return ids(in.ProductID), nil, nil
	})
	productGroupsState := func(tool string) func(*mcpSession, context.Context, int64) any {
		return func(s *mcpSession, ctx context.Context, id int64) any {
			return s.soft(ctx, tool, pProductGroups, ids(id), nil)
		}
	}
	linkState := productGroupsState("product_group_link")
	writeTool(writeSpec[groupLinkArgs]{
		name: "product_group_link",
		refs: refsGroupLink,
		route: func(_ *mcpSession, _ context.Context, in groupLinkArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("product_id", in.ProductID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if err := positive("group_id", in.GroupID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if (in.MinSelect == nil) != (in.MaxSelect == nil) {
				return "", "", nil, nil, nil, toolErr("Manda min_select y max_select juntos, o ninguno para heredar los del grupo.")
			}
			body := map[string]any{"groupId": in.GroupID, "title": in.Title, "override": in.MinSelect != nil, "position": in.Position}
			if in.MinSelect != nil {
				body["minSelect"], body["maxSelect"] = *in.MinSelect, *in.MaxSelect
			}
			return http.MethodPost, pProductGroups, ids(in.ProductID), nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, in groupLinkArgs, _ json.RawMessage) any {
			return linkState(s, ctx, in.ProductID)
		},
		preview: func(s *mcpSession, ctx context.Context, in groupLinkArgs) (any, []string) {
			g, err := groupNow(s, ctx, "product_group_link", in.GroupID)
			if err != nil {
				return nil, []string{err.Error()}
			}
			return map[string]any{"group": g}, groupInactiveNote(g)
		},
	})
	unlinkState := productGroupsState("product_group_unlink")
	writeTool(writeSpec[groupUnlinkArgs]{
		name: "product_group_unlink",
		refs: refsGroupUnlink,
		route: func(_ *mcpSession, _ context.Context, in groupUnlinkArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("product_id", in.ProductID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if err := positive("group_id", in.GroupID); err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodDelete, pProductGroup, map[string]int64{"id": in.ProductID, "groupId": in.GroupID}, nil, nil, nil
		},
		state: func(s *mcpSession, ctx context.Context, in groupUnlinkArgs, _ json.RawMessage) any {
			return unlinkState(s, ctx, in.ProductID)
		},
	})

	// ---- Productos ----
	writeTool(writeSpec[productCreateArgs]{
		name: "product_create",
		refs: refsProductCreate,
		route: func(_ *mcpSession, _ context.Context, in productCreateArgs) (string, string, map[string]int64, url.Values, any, error) {
			if strings.TrimSpace(in.Name) == "" {
				return "", "", nil, nil, nil, toolErr("name es obligatorio.")
			}
			if err := positive("category_id", in.CategoryID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if strings.TrimSpace(in.Price) == "" {
				return "", "", nil, nil, nil, toolErr("price es obligatorio, texto decimal.")
			}
			body := map[string]any{"name": in.Name, "categoryId": in.CategoryID, "price": in.Price,
				"trackStock": in.TrackStock, "favorite": in.Favorite}
			if in.Cost != nil {
				body["cost"] = *in.Cost
			}
			return http.MethodPost, pProducts, nil, nil, body, nil
		},
		preview: func(s *mcpSession, ctx context.Context, in productCreateArgs) (any, []string) {
			out := map[string]any{}
			var attention, notes []string
			if b, _, err := s.fetch(ctx, "product_create", pProducts, nil, url.Values{"limit": {"0"}, "search": {in.Name}}); err == nil {
				if sim := similarNames(b, in.Name); len(sim) > 0 {
					attention = append(attention, "Ya hay productos con un nombre parecido: revisa si es uno de ellos antes de crear otro.")
					out["similar"] = sim
				}
			}
			if b, _, err := s.fetch(ctx, "product_create", pCategories, nil, nil); err == nil {
				if c := findByID(b, in.CategoryID); c != nil {
					out["category"] = c
				} else {
					notes = append(notes, fmt.Sprintf("No existe la categoría %d; búscala con categories_list.", in.CategoryID))
				}
			}
			return attentionFirst{attention: attention, resolved: out}, notes
		},
	})
}
