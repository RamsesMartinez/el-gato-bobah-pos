package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Las herramientas de la spec 099: la merma esperada de un insumo y el alta y edición de categorías
// del menú. Las tres mandan a la ruta lo mismo que mandaría una pantalla; las reglas (tope del %,
// árbol de dos niveles, no apagar con productos activos) las aplica el servicio.

//nolint:gosec // G101: patrones de rutas; gosec confunde palabras con credenciales.
const (
	pArticleExpectedWaste = "/api/v1/stock/articles/{id}/expected-waste"
	pArticleWastePreview  = "/api/v1/stock/articles/{id}/expected-waste/preview"
)

// pctPattern es el % de merma que acepta la herramienta: de 0 a 99.99, dos decimales. El tope
// exclusivo de 100 es el de domain.ValidExpectedWastePct. Sin cero a la izquierda: el % viaja como
// número JSON y "08" no lo es (la vista previa lo aceptaba y aplicar tronaba).
var pctPattern = regexp.MustCompile(`^(0|[1-9][0-9]?)(\.[0-9]{1,2})?$`)

func init() {
	for name, routes := range map[string][]mcpRoute{
		"ingredient_expected_waste_set": {{http.MethodPut, pArticleExpectedWaste}, get(pArticleWastePreview), get(pArticle)},
		"category_create":               {{http.MethodPost, pCategories}, get(pCategories)},
		"category_update":               {{http.MethodPatch, pCategory}, get(pCategories)},
	} {
		mcpToolRoutes[name] = routes
	}
	for k, v := range map[string]string{
		"ingredient_expected_waste_set": "articles_search", "category_create": "categories_list", "category_update": "categories_list",
	} {
		mcpListTool[k] = v
	}
	register099()
}

// ---- Argumentos ----

type expectedWasteSetArgs struct {
	writeCommon
	IngredientID int64   `json:"ingredient_id" jsonschema:"insumo (de articles_search o stock_levels)"`
	Pct          *string `json:"pct,omitempty" jsonschema:"merma esperada en %, texto decimal de 0 a 99.99 (\"8\", \"7.5\"); \"0\" es capturada en cero. O clear"`
	Clear        bool    `json:"clear,omitempty" jsonschema:"true quita la captura: el insumo hereda la merma del producto o la general. O pct"`
	Reason       string  `json:"reason" jsonschema:"por qué se cambia (queda en la bitácora), de 1 a 200 letras"`
}

// body arma lo que va a la ruta. El % viaja como número JSON: la ruta rechaza el texto.
func (in expectedWasteSetArgs) body() (map[string]any, error) {
	if err := positive("ingredient_id", in.IngredientID); err != nil {
		return nil, err
	}
	if (in.Pct == nil) == !in.Clear {
		return nil, toolErr("Manda pct o clear: true, uno de los dos. Sin ninguno no se sabe si quitar la captura o fijar un %%.")
	}
	if err := requireReason(in.Reason, "cambia la merma esperada"); err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(in.Reason)
	if in.Clear {
		return map[string]any{"pct": nil, "reason": reason}, nil
	}
	if !pctPattern.MatchString(*in.Pct) {
		return nil, toolErr("pct va de 0 a menos de 100, con hasta dos decimales (\"8\", \"7.5\").")
	}
	return map[string]any{"pct": json.RawMessage(*in.Pct), "reason": reason}, nil
}

func (in expectedWasteSetArgs) previewQuery() url.Values {
	if in.Clear {
		return url.Values{"pct": {"none"}}
	}
	return url.Values{"pct": {*in.Pct}}
}

type categoryCreateArgs struct {
	writeCommon
	Name     string `json:"name" jsonschema:"nombre de la categoría"`
	ParentID *int64 `json:"parent_id,omitempty" jsonschema:"categoría raíz bajo la que va (de categories_list); vacío = raíz"`
}

func (in categoryCreateArgs) body() (map[string]any, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, toolErr("name es obligatorio: el nombre de la categoría.")
	}
	return map[string]any{"name": strings.TrimSpace(in.Name), "parentId": in.ParentID}, nil
}

type categoryUpdateArgs struct {
	writeCommon
	CategoryID int64   `json:"category_id" jsonschema:"categoría (de categories_list; include_inactive para ver las apagadas)"`
	Name       *string `json:"name,omitempty" jsonschema:"nombre nuevo"`
	Active     *bool   `json:"active,omitempty" jsonschema:"false la apaga (deja de salir en la venta); true la reactiva"`
	ParentID   *int64  `json:"parent_id,omitempty" jsonschema:"moverla bajo esta categoría raíz; o make_root"`
	MakeRoot   bool    `json:"make_root,omitempty" jsonschema:"true le quita el padre y la vuelve raíz; o parent_id"`
}

func (in categoryUpdateArgs) body() (map[string]any, error) {
	if err := positive("category_id", in.CategoryID); err != nil {
		return nil, err
	}
	if in.ParentID != nil && in.MakeRoot {
		return nil, toolErr("Manda parent_id o make_root, no los dos.")
	}
	b := map[string]any{}
	if in.Name != nil {
		b["name"] = strings.TrimSpace(*in.Name)
	}
	if in.Active != nil {
		b["isActive"] = *in.Active
	}
	if in.ParentID != nil {
		b["parentId"] = *in.ParentID
	}
	if in.MakeRoot {
		b["parentId"] = nil
	}
	if len(b) == 0 {
		return nil, toolErr("No hay nada que cambiar: manda name, active, parent_id o make_root.")
	}
	return b, nil
}

// ---- Ayudas ----

// categoryRow es un renglón de GET /admin/categories?status=all.
type categoryRow struct {
	ID                   int64  `json:"id"`
	Name                 string `json:"name"`
	ParentID             *int64 `json:"parentId"`
	IsActive             bool   `json:"isActive"`
	ProductCount         int    `json:"productCount"`
	InactiveProductCount int    `json:"inactiveProductCount"`
}

func allCategories(s *mcpSession, ctx context.Context, tool string) (json.RawMessage, []categoryRow, error) {
	b, _, err := s.fetch(ctx, tool, pCategories, nil, url.Values{"status": {"all"}})
	if err != nil {
		return nil, nil, err
	}
	rows, err := parseCategoryRows(b)
	if err != nil {
		return nil, nil, err
	}
	return b, rows, nil
}

// parseCategoryRows lee la lista de categorías. Un cuerpo ilegible es un error, no «no hay
// categorías»: leído así, la vista previa diría «no existe la categoría N» de una que sí existe.
func parseCategoryRows(b []byte) ([]categoryRow, error) {
	var env struct {
		Items []categoryRow `json:"items"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	return env.Items, nil
}

func findCategory(rows []categoryRow, id int64) *categoryRow {
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i]
		}
	}
	return nil
}

// parentProblem dice, antes de aplicar, por qué la ruta rechazaría ese padre (la regla es de
// domain.CheckCategoryChange; aquí solo se adelanta para que el agente no aplique a ciegas).
func parentProblem(rows []categoryRow, parentID int64, selfID int64, active bool) string {
	p := findCategory(rows, parentID)
	switch {
	case p == nil:
		return fmt.Sprintf("No existe la categoría %d en esta empresa; búscala con categories_list.", parentID)
	case parentID == selfID:
		return "Una categoría no puede ser su propio padre."
	case p.ParentID != nil:
		return fmt.Sprintf("«%s» ya es subcategoría: el padre debe ser una categoría raíz.", p.Name)
	case active && !p.IsActive:
		return fmt.Sprintf("«%s» está inactiva: actívala primero con category_update y active: true.", p.Name)
	}
	return ""
}

func register099() {
	// ---- Merma esperada de un insumo ----
	writeTool(writeSpec[expectedWasteSetArgs]{
		name: "ingredient_expected_waste_set",
		refs: refsExpectedWasteSet,
		route: func(_ *mcpSession, _ context.Context, in expectedWasteSetArgs) (string, string, map[string]int64, url.Values, any, error) {
			body, err := in.body()
			if err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPut, pArticleExpectedWaste, ids(in.IngredientID), nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, in expectedWasteSetArgs, _ json.RawMessage) any {
			return s.soft(ctx, "ingredient_expected_waste_set", pArticle, ids(in.IngredientID), nil)
		},
		strictPreview: func(s *mcpSession, ctx context.Context, in expectedWasteSetArgs) (any, []string, error) {
			b, _, err := s.fetch(ctx, "ingredient_expected_waste_set", pArticleWastePreview, ids(in.IngredientID), in.previewQuery())
			if err != nil {
				return nil, nil, err
			}
			var p struct {
				IsActive  bool `json:"isActive"`
				Unchanged bool `json:"unchanged"`
				Fallback  struct {
					ProductOrGeneral bool `json:"productOrGeneral"`
				} `json:"fallback"`
			}
			_ = json.Unmarshal(b, &p)
			notes := []string{"Costo efectivo = costo × (1 + % / 100). «Sin captura» (pct null) no es 0 %: sin captura el " +
				"insumo hereda la merma del producto o la general; 0 % es capturado en cero y no hereda.",
				"Aplicar recalcula el costo de los productos, extras y preparados de «uses»; no mueve existencias ni crea un gasto. " +
					"Una pérdida que ya ocurrió (un bote que caducó) va con waste_record, no aquí."}
			if p.Fallback.ProductOrGeneral {
				notes = append(notes, "Un lado sin captura no tiene un costo efectivo único: depende del producto; mira el costo por producto en «uses».")
			}
			if p.Unchanged {
				notes = append(notes, "El insumo ya tiene ese valor: aplicar no cambia nada ni deja renglón en la bitácora.")
			}
			if !p.IsActive {
				notes = append(notes, "El insumo está inactivo: su merma solo afecta recetas que todavía lo lleven.")
			}
			return json.RawMessage(b), notes, nil
		},
	})

	// ---- Categorías del menú ----
	writeTool(writeSpec[categoryCreateArgs]{
		name: "category_create",
		refs: refsCategoryCreate,
		route: func(_ *mcpSession, _ context.Context, in categoryCreateArgs) (string, string, map[string]int64, url.Values, any, error) {
			body, err := in.body()
			if err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPost, pCategories, nil, nil, body, nil
		},
		preview: func(s *mcpSession, ctx context.Context, in categoryCreateArgs) (any, []string) {
			b, rows, err := allCategories(s, ctx, "category_create")
			if err != nil {
				return map[string]string{"unavailable": err.Error()}, nil
			}
			var attention []string
			similar := similarNames(b, in.Name)
			if similar == nil {
				similar = []map[string]any{}
			}
			if len(similar) > 0 {
				attention = append(attention, "Ya hay categorías con nombre parecido (activas o inactivas): si es una de ellas, "+
					"úsala (o reactívala con category_update) y no crees otra.")
			}
			resolved := map[string]any{"similar": similar}
			if in.ParentID != nil {
				if msg := parentProblem(rows, *in.ParentID, 0, true); msg != "" {
					attention = append(attention, msg+" Aplicar se rechazaría.")
				}
				resolved["parent"] = findCategory(rows, *in.ParentID)
			}
			return attentionFirst{attention: attention, resolved: resolved},
				[]string{"Nace activa y sin productos; dar de alta productos en ella es product_create."}
		},
	})

	writeTool(writeSpec[categoryUpdateArgs]{
		name: "category_update",
		refs: refsCategoryUpdate,
		route: func(_ *mcpSession, _ context.Context, in categoryUpdateArgs) (string, string, map[string]int64, url.Values, any, error) {
			body, err := in.body()
			if err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPatch, pCategory, ids(in.CategoryID), nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, in categoryUpdateArgs, _ json.RawMessage) any {
			_, rows, err := allCategories(s, ctx, "category_update")
			if err != nil {
				return map[string]string{"unavailable": err.Error()}
			}
			return findCategory(rows, in.CategoryID)
		},
		preview: func(s *mcpSession, ctx context.Context, in categoryUpdateArgs) (any, []string) {
			_, rows, err := allCategories(s, ctx, "category_update")
			if err != nil {
				return map[string]string{"unavailable": err.Error()}, nil
			}
			cur := findCategory(rows, in.CategoryID)
			if cur == nil {
				return nil, nil
			}
			children := []map[string]any{}
			var activeChildren []string
			for _, r := range rows {
				if r.ParentID != nil && *r.ParentID == cur.ID {
					children = append(children, map[string]any{"id": r.ID, "name": r.Name, "isActive": r.IsActive})
					if r.IsActive {
						activeChildren = append(activeChildren, r.Name)
					}
				}
			}
			willBeActive := cur.IsActive
			if in.Active != nil {
				willBeActive = *in.Active
			}
			var attention []string
			if cur.IsActive && !willBeActive {
				if cur.ProductCount > 0 {
					attention = append(attention, fmt.Sprintf("Tiene %d productos activos: aplicar se rechazaría. Muévelos a otra "+
						"categoría o desactívalos con product_update.", cur.ProductCount))
				}
				if len(activeChildren) > 0 {
					attention = append(attention, "Tiene subcategorías activas ("+strings.Join(activeChildren, ", ")+
						"): aplicar se rechazaría. Desactívalas o muévelas primero.")
				}
			}
			newParent := cur.ParentID
			switch {
			case in.ParentID != nil:
				newParent = in.ParentID
				if len(children) > 0 {
					attention = append(attention, "Tiene subcategorías: no puede quedar dentro de otra.")
				}
			case in.MakeRoot:
				newParent = nil
			}
			parentChanges := in.ParentID != nil || (willBeActive && !cur.IsActive)
			if newParent != nil && parentChanges {
				if msg := parentProblem(rows, *newParent, cur.ID, willBeActive); msg != "" {
					attention = append(attention, msg+" Aplicar se rechazaría.")
				}
			}
			var notes []string
			if cur.IsActive && !willBeActive {
				notes = append(notes, fmt.Sprintf("Deja de salir en la venta y en el menú en línea; conserva sus %d productos inactivos.",
					cur.InactiveProductCount))
			}
			return attentionFirst{attention: attention, resolved: map[string]any{"current": cur, "children": children}}, notes
		},
	})
}
