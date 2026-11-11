package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// Las herramientas de la spec 076: dar de alta y editar insumos, y los empaques de un insumo. Igual
// que las de la 072–074, mandan el MISMO cuerpo que la pantalla a la MISMA ruta; el nombre repetido,
// la unidad base y la versión de la ficha los decide la ruta. Lo único propio es la vista previa, que
// pone primero los insumos parecidos con las mismas búsquedas que usa la pantalla.

//nolint:gosec // G101: patrones de rutas; gosec confunde palabras con credenciales.
const (
	pStockArticles     = "/api/v1/stock/articles"
	pArticleSimilar    = "/api/v1/stock/articles/similar"
	pArticleAltNames   = "/api/v1/stock/articles/{id}/alt-names"
	pArticleAltSimilar = "/api/v1/stock/articles/{id}/alt-names/similar"
)

// maxListMatches acota lo que la búsqueda por nombre de la lista de Almacén suma a los parecidos: con
// un nombre corto («Queso») trae decenas, y el aviso es para leerse, no para recorrerse.
const maxListMatches = 5

func init() {
	for name, routes := range map[string][]mcpRoute{
		"ingredient_create": {{http.MethodPost, pStockArticles}, get(pArticleSimilar), get(pStockArticles), get(pArticle)},
		"ingredient_update": {{http.MethodPatch, pArticle}, {http.MethodPut, pArticleAltNames}, get(pArticle),
			get(pArticleSimilar), get(pStockArticles), get(pArticleAltSimilar)},
	} {
		mcpToolRoutes[name] = routes
	}
	mcpListTool["ingredient_create"] = "articles_search"
	mcpListTool["ingredient_update"] = "articles_search"
	register076()
}

// ---- Argumentos ----

type ingredientCreateArgs struct {
	writeCommon
	Name         string  `json:"name" jsonschema:"nombre del insumo, como lo confirma la persona"`
	BaseUnitID   int16   `json:"base_unit_id" jsonschema:"unidad base: g, ml, cm o pieza (de units_list); lo que llega en kg o L es un empaque"`
	CategoryID   *int64  `json:"category_id,omitempty" jsonschema:"categoría del insumo"`
	PlaceID      *int64  `json:"place_id,omitempty" jsonschema:"ubicación en el almacén"`
	MinStock     *string `json:"min_stock,omitempty" jsonschema:"mínimo, en la unidad base, texto decimal"`
	InitialStock *string `json:"initial_stock,omitempty" jsonschema:"existencia inicial en la unidad base; omítela si el insumo llega con una compra"`
	InitialCost  *string `json:"initial_cost,omitempty" jsonschema:"costo por unidad base, texto decimal; omítelo si llega con una compra"`
}

type ingredientUpdateArgs struct {
	writeCommon
	IngredientID  int64     `json:"ingredient_id" jsonschema:"el insumo (de articles_search)"`
	Name          *string   `json:"name,omitempty" jsonschema:"nombre nuevo"`
	CategoryID    *int64    `json:"category_id,omitempty" jsonschema:"categoría nueva"`
	ClearCategory bool      `json:"clear_category,omitempty" jsonschema:"true quita la categoría; no va con category_id"`
	PlaceID       *int64    `json:"place_id,omitempty" jsonschema:"ubicación nueva"`
	ClearPlace    bool      `json:"clear_place,omitempty" jsonschema:"true quita la ubicación; no va con place_id"`
	MinStock      *string   `json:"min_stock,omitempty" jsonschema:"mínimo nuevo en la unidad base, texto decimal"`
	ClearMinStock bool      `json:"clear_min_stock,omitempty" jsonschema:"true quita el mínimo; no va con min_stock"`
	AltNames      *[]string `json:"alt_names,omitempty" jsonschema:"la lista COMPLETA de otros nombres; [] los quita; omítela para dejarlos como están"`
}

type presentationsListArgs struct {
	IngredientID *int64 `json:"ingredient_id,omitempty" jsonschema:"solo los empaques de este insumo (de articles_search)"`
}

// ---- Parecidos ----

// listRows lee una lista {items:[…]} o […].
func listRows(body json.RawMessage) []map[string]any {
	var env struct {
		Items []map[string]any `json:"items"`
	}
	if json.Unmarshal(body, &env) == nil && env.Items != nil {
		return env.Items
	}
	var rows []map[string]any
	_ = json.Unmarshal(body, &rows)
	return rows
}

func rowID(r map[string]any) int64 {
	v, _ := r["id"].(float64)
	return int64(v)
}

// similarArticles son los insumos que quizá ya son `name`, sin `self`: primero los de la búsqueda de
// parecidos de la hoja «Nuevo artículo» y después los que la lista de Almacén encuentra por nombre o
// por otro nombre (la de parecidos no lee otros nombres). Una búsqueda que no se pudo leer se dice en
// las notas: callarla haría creer que no hay duplicados.
func similarArticles(s *mcpSession, ctx context.Context, tool, name string, self int64) (items []map[string]any, notes []string) {
	seen := map[int64]bool{self: true}
	add := func(r map[string]any, via string) {
		id := rowID(r)
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		it := map[string]any{"id": id, "name": r["name"], "base_unit": r["baseUnitCode"], "on_hand": r["onHand"]}
		if alt, _ := r["matchedAlt"].(string); alt != "" {
			it["via_alt_name"] = alt
		}
		if via != "" {
			it["found_by"] = via
		}
		items = append(items, it)
	}
	if b, _, err := s.fetch(ctx, tool, pArticleSimilar, nil, url.Values{"name": {name}}); err != nil {
		notes = append(notes, "No se pudo revisar la búsqueda de nombres parecidos: "+err.Error())
	} else {
		for _, r := range listRows(b) {
			add(r, "")
		}
	}
	if b, _, err := s.fetch(ctx, tool, pStockArticles, nil, url.Values{"q": {name}}); err != nil {
		notes = append(notes, "No se pudo revisar la búsqueda por nombre y otros nombres: "+err.Error())
	} else {
		for i, r := range listRows(b) {
			if i == maxListMatches {
				break
			}
			add(r, "name_or_alt_name")
		}
	}
	return items, notes
}

// maybeExists es el aviso que va primero: «Quizá ya existe: X (id N)», uno por parecido.
func maybeExists(items []map[string]any) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		line := fmt.Sprintf("Quizá ya existe: %v (id %d)", it["name"], it["id"])
		if alt, ok := it["via_alt_name"].(string); ok {
			line += fmt.Sprintf(", que también se llama «%s»", alt)
		}
		out = append(out, line+". Pregunta a la persona si es ese antes de aplicar.")
	}
	return out
}

// ---- Ficha ----

// articleSheet es la ficha del insumo bajo la empresa del token, con la versión que exige la ruta.
type articleSheet struct {
	raw       map[string]any
	updatedAt time.Time
	altNames  []string
}

func readArticleSheet(s *mcpSession, ctx context.Context, tool string, id int64) (articleSheet, error) {
	b, _, err := s.fetch(ctx, tool, pArticle, ids(id), nil)
	if err != nil {
		return articleSheet{}, err
	}
	var sh articleSheet
	var typed struct {
		UpdatedAt time.Time `json:"updatedAt"`
		AltNames  []string  `json:"altNames"`
	}
	if json.Unmarshal(b, &sh.raw) != nil || json.Unmarshal(b, &typed) != nil {
		return articleSheet{}, toolErr("No se pudo leer la ficha del insumo %d.", id)
	}
	sh.updatedAt, sh.altNames = typed.UpdatedAt, typed.AltNames
	return sh, nil
}

// view es lo que la edición puede cambiar, para el antes y el después.
func (sh articleSheet) view() map[string]any {
	out := map[string]any{}
	for _, k := range []string{"id", "name", "categoryId", "category", "countPlaceId", "countPlace", "minStock", "altNames", "baseUnitCode"} {
		out[k] = sh.raw[k]
	}
	return out
}

// sheetBody son los campos de la ficha que se mandan al PATCH; vacío si solo cambian los otros nombres.
func (in ingredientUpdateArgs) sheetBody() map[string]any {
	body := map[string]any{}
	if in.Name != nil {
		body["name"] = *in.Name
	}
	switch {
	case in.ClearCategory:
		body["categoryId"] = nil
	case in.CategoryID != nil:
		body["categoryId"] = *in.CategoryID
	}
	switch {
	case in.ClearPlace:
		body["placeId"] = nil
	case in.PlaceID != nil:
		body["placeId"] = *in.PlaceID
	}
	switch {
	case in.ClearMinStock:
		body["minStock"] = nil
	case in.MinStock != nil:
		body["minStock"] = *in.MinStock
	}
	return body
}

// check revisa la forma de los argumentos; lo que vale o no un cambio lo decide la ruta.
func (in ingredientUpdateArgs) check() error {
	if err := positive("ingredient_id", in.IngredientID); err != nil {
		return err
	}
	if (in.ClearCategory && in.CategoryID != nil) || (in.ClearPlace && in.PlaceID != nil) || (in.ClearMinStock && in.MinStock != nil) {
		return toolErr("Manda el valor nuevo o el clear_* de ese campo, no los dos.")
	}
	if len(in.sheetBody()) == 0 && in.AltNames == nil {
		return toolErr("Manda al menos uno de name, category_id, place_id, min_stock, sus clear_* o alt_names.")
	}
	return nil
}

func register076() {
	writeTool(writeSpec[ingredientCreateArgs]{
		name: "ingredient_create",
		route: func(_ *mcpSession, _ context.Context, in ingredientCreateArgs) (string, string, map[string]int64, url.Values, any, error) {
			if strings.TrimSpace(in.Name) == "" {
				return "", "", nil, nil, nil, toolErr("name es obligatorio: el nombre del insumo.")
			}
			if in.BaseUnitID <= 0 {
				return "", "", nil, nil, nil, toolErr("base_unit_id es obligatorio: g, ml, cm o pieza (de units_list).")
			}
			body := map[string]any{"name": in.Name, "baseUnitId": in.BaseUnitID, "categoryId": in.CategoryID,
				"placeId": in.PlaceID, "minStock": in.MinStock, "initialStock": in.InitialStock, "initialCost": in.InitialCost}
			return http.MethodPost, pStockArticles, nil, nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, _ ingredientCreateArgs, result json.RawMessage) any {
			var r struct {
				ID int64 `json:"id"`
			}
			if result == nil || json.Unmarshal(result, &r) != nil || r.ID == 0 {
				return nil
			}
			return s.soft(ctx, "ingredient_create", pArticle, ids(r.ID), nil)
		},
		preview: func(s *mcpSession, ctx context.Context, in ingredientCreateArgs) (any, []string) {
			items, notes := similarArticles(s, ctx, "ingredient_create", strings.TrimSpace(in.Name), 0)
			if len(items) == 0 && len(notes) == 0 {
				notes = append(notes, "No hay insumos parecidos a este nombre.")
			}
			if in.InitialStock != nil || in.InitialCost != nil {
				notes = append(notes, "Si el insumo se crea para capturar una compra, la existencia y el costo llegan con la "+
					"compra: no los mandes aquí o el almacén los contará dos veces.")
			}
			if items == nil {
				items = []map[string]any{}
			}
			return attentionFirst{attention: maybeExists(items), resolved: map[string]any{"maybe_exists": items}}, notes
		},
	})

	writeTool(writeSpec[ingredientUpdateArgs]{
		name: "ingredient_update",
		route: func(_ *mcpSession, _ context.Context, in ingredientUpdateArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := in.check(); err != nil {
				return "", "", nil, nil, nil, err
			}
			// Lo que se va a mandar, en el orden en que se manda; la versión de la ficha se lee al aplicar.
			would := map[string]any{}
			if b := in.sheetBody(); len(b) > 0 {
				would["1_sheet"] = b
			}
			if in.AltNames != nil {
				would["2_alt_names"] = *in.AltNames
			}
			return http.MethodPatch, pArticle, ids(in.IngredientID), nil, would, nil
		},
		state: func(s *mcpSession, ctx context.Context, in ingredientUpdateArgs, _ json.RawMessage) any {
			sh, err := readArticleSheet(s, ctx, "ingredient_update", in.IngredientID)
			if err != nil {
				return map[string]string{"unavailable": err.Error()}
			}
			return sh.view()
		},
		preview: func(s *mcpSession, ctx context.Context, in ingredientUpdateArgs) (any, []string) {
			const tool = "ingredient_update"
			sh, err := readArticleSheet(s, ctx, tool, in.IngredientID)
			if err != nil {
				return nil, []string{err.Error()}
			}
			var notes []string
			var attention []string
			out := map[string]any{}
			if in.Name != nil {
				items, n := similarArticles(s, ctx, tool, strings.TrimSpace(*in.Name), in.IngredientID)
				notes = append(notes, n...)
				attention = maybeExists(items)
				if items == nil {
					items = []map[string]any{}
				}
				out["maybe_exists"] = items
			}
			if in.AltNames != nil {
				similar := map[string]any{}
				for _, alt := range *in.AltNames {
					if slices.Contains(sh.altNames, alt) {
						continue
					}
					b, _, err := s.fetch(ctx, tool, pArticleAltSimilar, ids(in.IngredientID), url.Values{"name": {alt}})
					if err != nil {
						notes = append(notes, fmt.Sprintf("No se pudo revisar «%s»: %s", alt, err.Error()))
						continue
					}
					rows := listRows(b)
					similar[alt] = rows
					for _, r := range rows {
						attention = append(attention, fmt.Sprintf("El otro nombre «%s» se parece a %v (id %d).", alt, r["name"], rowID(r)))
					}
				}
				out["alt_names_similar"] = similar
			}
			notes = append(notes, "Al aplicar se guarda primero la ficha y después los otros nombres. Si los otros "+
				"nombres se rechazan, la ficha ya quedó cambiada y el error lo dice.")
			return attentionFirst{attention: attention, resolved: out}, notes
		},
		run: func(s *mcpSession, ctx context.Context, in ingredientUpdateArgs) (int, []byte, error) {
			const tool = "ingredient_update"
			// La versión se lee justo antes de escribir. El error de la lectura va crudo para que apply lo
			// traduzca como cualquier rechazo de la ruta (no existe, sin permiso de ver el almacén).
			status, resp, err := s.dispatch(ctx, tool, http.MethodGet, pArticle, ids(in.IngredientID), nil, nil)
			if err != nil || status >= 300 {
				return status, resp, err
			}
			var sh struct {
				UpdatedAt time.Time `json:"updatedAt"`
			}
			if json.Unmarshal(resp, &sh) != nil || sh.UpdatedAt.IsZero() {
				return http.StatusInternalServerError, nil, fmt.Errorf("ficha %d sin versión", in.IngredientID)
			}
			version := sh.UpdatedAt
			var changed []string
			if body := in.sheetBody(); len(body) > 0 {
				body["expectedUpdatedAt"] = version
				status, resp, err := s.dispatch(ctx, tool, http.MethodPatch, pArticle, ids(in.IngredientID), nil, body)
				if err != nil || status >= 300 {
					return status, resp, err
				}
				var r struct {
					UpdatedAt time.Time `json:"updatedAt"`
				}
				_ = json.Unmarshal(resp, &r)
				version = r.UpdatedAt
				changed = append(changed, "ficha")
			}
			if in.AltNames != nil {
				status, resp, err := s.dispatch(ctx, tool, http.MethodPut, pArticleAltNames, ids(in.IngredientID), nil,
					map[string]any{"altNames": *in.AltNames, "expectedUpdatedAt": version})
				if err != nil || status >= 500 {
					return max(status, 500), resp, err
				}
				if status >= 400 {
					if len(changed) > 0 {
						var env errorEnvelope
						_ = json.Unmarshal(resp, &env)
						env.Error.Message = "La ficha ya quedó cambiada; los otros nombres no: " + strings.TrimRight(env.Error.Message, ".")
						resp, _ = json.Marshal(env)
					}
					return status, resp, nil
				}
				changed = append(changed, "otros nombres")
			}
			b, _ := json.Marshal(map[string]any{"id": in.IngredientID, "changed": changed})
			return http.StatusOK, b, nil
		},
	})

	readTool("presentations_list", func(s *mcpSession, ctx context.Context, in presentationsListArgs) (any, error) {
		pattern, params := pPresentations, map[string]int64(nil)
		if in.IngredientID != nil {
			// Presente y no positivo se rechaza: caer a «todos» devolvería otra cosa de lo que se pidió.
			if err := positive("ingredient_id", *in.IngredientID); err != nil {
				return nil, err
			}
			pattern, params = pIngredientPresentations, ids(*in.IngredientID)
		}
		body, _, err := s.fetch(ctx, "presentations_list", pattern, params, nil)
		if err != nil {
			return nil, err
		}
		return forAgent(body), nil
	})
}
