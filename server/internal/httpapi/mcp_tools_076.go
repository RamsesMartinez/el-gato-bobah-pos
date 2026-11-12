package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
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
	// Spec 077: las acciones de la hoja «Archivar» de la ficha y su revisión previa.
	pArticleArchive      = "/api/v1/stock/articles/{id}/archive"
	pArticleReactivate   = "/api/v1/stock/articles/{id}/reactivate"
	pArticleArchiveCheck = "/api/v1/stock/articles/{id}/archive-check"
)

// maxListMatches acota lo que la búsqueda por nombre de la lista de Almacén suma a los parecidos: con
// un nombre corto («Queso») trae decenas, y el aviso es para leerse, no para recorrerse.
const maxListMatches = 5

func init() {
	for name, routes := range map[string][]mcpRoute{
		"ingredient_create": {{http.MethodPost, pStockArticles}, get(pArticleSimilar), get(pStockArticles), get(pArticle)},
		"ingredient_update": {{http.MethodPatch, pArticle}, {http.MethodPut, pArticleAltNames}, get(pArticle),
			get(pArticleSimilar), get(pStockArticles), get(pArticleAltSimilar),
			{http.MethodPost, pArticleArchive}, {http.MethodPost, pArticleReactivate}, get(pArticleArchiveCheck)},
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
	Active        *bool     `json:"active,omitempty" jsonschema:"false lo desactiva y true lo reactiva; va solo, sin otros cambios en la misma llamada"`
	ConfirmOnHand bool      `json:"confirm_on_hand,omitempty" jsonschema:"solo con active: false: desactivarlo aunque tenga existencia, sin ajustarla; solo si la persona lo decide"`
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
	for _, k := range []string{"id", "name", "isActive", "categoryId", "category", "countPlaceId", "countPlace", "minStock", "altNames", "baseUnitCode"} {
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
	if in.ConfirmOnHand && (in.Active == nil || *in.Active) {
		return toolErr("confirm_on_hand va solo con active: false.")
	}
	// Desactivar es otra ruta que la ficha y los otros nombres: juntas dejarían estados a medias.
	if in.Active != nil {
		if len(in.sheetBody()) > 0 || in.AltNames != nil {
			return toolErr("active va solo: desactiva o reactiva en una llamada y edita la ficha en otra.")
		}
		return nil
	}
	if (in.ClearCategory && in.CategoryID != nil) || (in.ClearPlace && in.PlaceID != nil) || (in.ClearMinStock && in.MinStock != nil) {
		return toolErr("Manda el valor nuevo o el clear_* de ese campo, no los dos.")
	}
	// El tope es el de la ruta (domain.MaxAltNames), revisado aquí ANTES de la vista previa: ella hace
	// una lectura por nombre y la ruta solo lo revisa al guardar.
	if in.AltNames != nil && len(*in.AltNames) > domain.MaxAltNames {
		return toolErr("alt_names lleva a lo más %d nombres.", domain.MaxAltNames)
	}
	if len(in.sheetBody()) == 0 && in.AltNames == nil {
		return toolErr("Manda al menos uno de name, category_id, place_id, min_stock, sus clear_*, alt_names o active.")
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
			if in.Active != nil {
				pattern, body := in.activeRoute()
				return http.MethodPost, pattern, ids(in.IngredientID), nil, body, nil
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
			if in.Active != nil {
				return activePreview(s, ctx, in)
			}
			sh, err := readArticleSheet(s, ctx, tool, in.IngredientID)
			if err != nil {
				return nil, []string{err.Error()}
			}
			var notes []string
			var attention []string
			out := map[string]any{}
			if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
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
					if _, done := similar[alt]; done || slices.Contains(sh.altNames, alt) {
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
		run: ingredientUpdateRun,
	})

	readTool("articles_search", articlesSearch)

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

// ingredientUpdateRun aplica la edición: ficha y después otros nombres, con la versión leída aquí.
func ingredientUpdateRun(s *mcpSession, ctx context.Context, in ingredientUpdateArgs) (int, []byte, error) {
	const tool = "ingredient_update"
	if in.Active != nil {
		// Un solo despacho a la ruta de la hoja «Archivar»: la existencia, las recetas, el conteo, el
		// nombre tomado y la unión los decide el servicio.
		pattern, body := in.activeRoute()
		return s.dispatch(ctx, tool, http.MethodPost, pattern, ids(in.IngredientID), nil, body)
	}
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
		changed = append(changed, "ficha")
		var r struct {
			UpdatedAt time.Time `json:"updatedAt"`
		}
		if json.Unmarshal(resp, &r) != nil || r.UpdatedAt.IsZero() {
			if in.AltNames != nil {
				return sheetSavedAltNamesUnknown()
			}
		}
		version = r.UpdatedAt
	}
	if in.AltNames != nil {
		// ponytail: la lista completa reemplaza a la que hay con la versión leída en esta llamada, no con
		// la de la vista previa: un otro nombre agregado en la tableta entre las dos se pisa sin STALE.
		// Cerrarlo exige mandar la versión de la vista previa como `fingerprint`, como expense_edit.
		status, resp, err := s.dispatch(ctx, tool, http.MethodPut, pArticleAltNames, ids(in.IngredientID), nil,
			map[string]any{"altNames": *in.AltNames, "expectedUpdatedAt": version})
		if err != nil || status >= 500 {
			if len(changed) > 0 {
				if err != nil {
					slog.ErrorContext(ctx, "mcp_dispatch_failed", "tool", tool, "pattern", pArticleAltNames, "error", err.Error())
				}
				return sheetSavedAltNamesUnknown()
			}
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
}

// sheetSavedAltNamesUnknown es la respuesta cuando la ficha ya se guardó y los otros nombres no se
// sabe. Un 409 y no un 5xx: con el 5xx apply diría «puede que no se haya aplicado» y la persona
// creería que nada cambió; con el 409 la clave queda libre y repetir la llamada termina el trabajo
// (la ficha se vuelve a mandar con los mismos valores y la versión nueva).
func sheetSavedAltNamesUnknown() (int, []byte, error) {
	b, _ := json.Marshal(errorEnvelope{Error: errorBody{Code: "PARTIAL", Message: "La ficha ya quedó cambiada; no se sabe si " +
		"los otros nombres. Revísalo con articles_search y repite la llamada si faltan"}})
	return http.StatusConflict, b, nil
}

// ---- Desactivar y reactivar (spec 077) ----

// activeRoute es la ruta de la hoja «Archivar» y su cuerpo, el mismo de la pantalla.
func (in ingredientUpdateArgs) activeRoute() (string, any) {
	if *in.Active {
		return pArticleReactivate, map[string]any{}
	}
	return pArticleArchive, map[string]any{"confirmOnHand": in.ConfirmOnHand}
}

// archiveCheck es la revisión previa de la hoja «Archivar» (GET …/archive-check).
type archiveCheck struct {
	OnHand []struct {
		Branch string          `json:"branch"`
		Qty    decimal.Decimal `json:"qty"`
	} `json:"onHand"`
	ActiveRecipes []struct {
		Kind string `json:"kind"`
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"activeRecipes"`
	OpenCount       bool `json:"openCount"`
	PendingReceipts int  `json:"pendingReceipts"`
}

var recipeKindLabel = map[string]string{"product": "producto", "option": "extra", "ingredient": "preparado"}

// activePreview es la vista previa de active: lo que la hoja «Archivar» muestra antes del botón. Las
// reglas no se revisan aquí (las aplica el servicio al aplicar): se dicen, para que el agente pregunte
// a la persona antes.
func activePreview(s *mcpSession, ctx context.Context, in ingredientUpdateArgs) (any, []string) {
	const tool = "ingredient_update"
	sh, err := readArticleSheet(s, ctx, tool, in.IngredientID)
	if err != nil {
		return nil, []string{"No se pudo leer la ficha ni revisar su existencia y sus recetas: " + err.Error()}
	}
	active, _ := sh.raw["isActive"].(bool)
	unit, _ := sh.raw["baseUnitCode"].(string)
	resolved := map[string]any{"is_active": active}
	if *in.Active {
		if active {
			return resolved, []string{"El insumo ya está activo; aplicar no cambia nada."}
		}
		return resolved, []string{fmt.Sprintf("Vuelve a ofrecerse en conteos, compras y existencias tal como estaba, con su "+
			"existencia: %v %s.", sh.raw["onHand"], unit)}
	}
	if !active {
		return resolved, []string{"El insumo ya está inactivo; aplicar no cambia nada."}
	}
	notes := []string{"Al desactivarlo deja de salir en conteos, compras y existencias (también en stock_levels); su " +
		"historial se conserva y se reactiva con active: true."}
	b, _, err := s.fetch(ctx, tool, pArticleArchiveCheck, ids(in.IngredientID), nil)
	if err != nil {
		// Callarlo haría creer que no tiene existencia ni recetas.
		return resolved, append(notes, "No se pudo revisar su existencia ni sus recetas: "+err.Error())
	}
	var check archiveCheck
	if json.Unmarshal(b, &check) != nil {
		return resolved, append(notes, "No se pudo leer la revisión de su existencia y sus recetas.")
	}
	resolved["archive_check"] = json.RawMessage(b)
	var attention []string
	if len(check.OnHand) > 0 {
		parts := make([]string, len(check.OnHand))
		for i, o := range check.OnHand {
			parts[i] = fmt.Sprintf("%s %s %s", o.Branch, o.Qty.String(), unit)
		}
		attention = append(attention, "Tiene existencia: "+strings.Join(parts, "; ")+". Registra la merma (waste_record) o "+
			"el ajuste (stock_adjust) antes de desactivarlo: desactivarlo no la ajusta, se queda así y deja de verse en "+
			"existencias. Solo si la persona decide desactivarlo con esa existencia, manda confirm_on_hand: true.")
	}
	for _, r := range check.ActiveRecipes {
		attention = append(attention, fmt.Sprintf("Lo usa la receta activa %s (%s, id %d): quítalo de ella en la pantalla "+
			"antes de desactivarlo. Desactivarlo no cambia la receta ni el producto.", r.Name, recipeKindLabel[r.Kind], r.ID))
	}
	if check.OpenCount {
		attention = append(attention, "Un conteo abierto ya lo contó: ciérralo o descártalo antes de desactivarlo.")
	}
	if check.PendingReceipts > 0 {
		notes = append(notes, fmt.Sprintf("Tiene %d compra(s) por recibir: al llegar, sumarán aunque esté inactivo.", check.PendingReceipts))
	}
	return attentionFirst{attention: attention, resolved: resolved}, notes
}

// ---- articles_search con inactivos (spec 077) ----

// articlesSearch es la búsqueda de la captura de compras y gastos, que solo trae activos, más los
// insumos archivados de Almacén › Archivados cuando se piden: para reactivar uno hay que encontrarlo.
func articlesSearch(s *mcpSession, ctx context.Context, in searchArgs) (any, error) {
	const tool = "articles_search"
	if strings.TrimSpace(in.Query) == "" {
		return nil, toolErr("query es obligatorio: parte del nombre.")
	}
	body, _, err := s.fetch(ctx, tool, pArticles, nil, url.Values{"q": {in.Query}})
	if err != nil {
		return nil, err
	}
	items := listRows(body)
	if items == nil {
		items = []map[string]any{}
	}
	seen := map[int64]bool{}
	for _, it := range items {
		it["active"] = true
		if it["itemType"] == "ingrediente" {
			seen[rowID(it)] = true
		}
	}
	out := map[string]any{"items": items}
	if !in.IncludeInactive {
		return out, nil
	}
	b, status, err := s.fetch(ctx, tool, pStockArticles, nil, url.Values{"q": {in.Query}, "status": {"archived"}})
	switch {
	case status == http.StatusForbidden:
		out["notes"] = []string{"Para incluir los inactivos hace falta el permiso de ver el almacén; estos son solo los activos."}
		return out, nil
	case err != nil:
		out["notes"] = []string{"No se pudieron buscar los inactivos: " + err.Error()}
		return out, nil
	}
	for _, r := range listRows(b) {
		id := rowID(r)
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		items = append(items, map[string]any{"itemType": "ingrediente", "id": id, "name": r["name"], "unitCode": r["baseUnitCode"],
			"altNames": r["altNames"], "onHand": r["onHand"], "active": false})
	}
	out["items"] = items
	return out, nil
}
