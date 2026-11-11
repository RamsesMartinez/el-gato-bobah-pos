package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/shopspring/decimal"
)

// Las herramientas de la spec 074: empaques de compra y corregir un gasto ya guardado. Igual que las
// de la 072 y la 073, mandan el MISMO cuerpo que la pantalla a la MISMA ruta; las reglas (marca
// repetida, marca larga, huella, decisiones, permiso de recibir) las decide la ruta.

//nolint:gosec // G101: patrones de rutas; gosec confunde palabras con credenciales.
const (
	pIngredientPresentations = "/api/v1/stock/ingredients/{id}/presentations"
	pPresentation            = "/api/v1/stock/presentations/{id}"
	pExpenseEdit             = "/api/v1/expenses/{id}/edit"
)

func init() {
	for name, routes := range map[string][]mcpRoute{
		"presentation_create": {{http.MethodPost, pIngredientPresentations}, get(pPresentations)},
		"presentation_update": {{http.MethodPut, pPresentation}, get(pPresentations), get(pUnits)},
		// La vista previa es la misma ruta con ?preview=1: la consulta no es parte del camino.
		"expense_edit": {{http.MethodPost, pExpenseEdit}, get(pExpense)},
	} {
		mcpToolRoutes[name] = routes
	}
	// La sugerencia de empaque de la compra lee la unidad base del insumo.
	mcpToolRoutes["purchase_create"] = append(mcpToolRoutes["purchase_create"], get(pArticles), get(pUnits))
	for k, v := range map[string]string{
		"presentation_create": "articles_search", "presentation_update": "presentations_list",
		"expense_edit": "expenses_list",
	} {
		mcpListTool[k] = v
	}
	dateFields["arrived_on"] = true
	register074()
}

// ---- Argumentos ----

type presentationCreateArgs struct {
	writeCommon
	IngredientID int64   `json:"ingredient_id" jsonschema:"insumo (de articles_search)"`
	Name         string  `json:"name" jsonschema:"cómo llega: bolsa, caja, costal…; no el nombre de una unidad"`
	Qty          string  `json:"qty" jsonschema:"cuánto trae un empaque, en unit_id, texto decimal (lo confirma la persona)"`
	UnitID       int16   `json:"unit_id" jsonschema:"unidad de qty (de units_list)"`
	Brand        *string `json:"brand,omitempty" jsonschema:"marca, hasta 40 letras (la confirma la persona)"`
	Approximate  bool    `json:"approximate,omitempty" jsonschema:"true si se compra en otra clase de unidad que el insumo (kilo de algo que se cuenta en piezas)"`
}

type presentationUpdateArgs struct {
	writeCommon
	PresentationID int64   `json:"presentation_id" jsonschema:"empaque (de presentations_list)"`
	Name           *string `json:"name,omitempty" jsonschema:"nombre nuevo"`
	Qty            *string `json:"qty,omitempty" jsonschema:"contenido nuevo, texto decimal; va con unit_id"`
	UnitID         *int16  `json:"unit_id,omitempty" jsonschema:"unidad del contenido nuevo; va con qty"`
	Brand          *string `json:"brand,omitempty" jsonschema:"marca nueva; vacío la quita"`
	Approximate    *bool   `json:"approximate,omitempty" jsonschema:"si se compra en otra clase de unidad"`
	Active         *bool   `json:"active,omitempty" jsonschema:"false: ya no llega así y deja de ofrecerse"`
}

type editLineArg struct {
	Name           string `json:"name" jsonschema:"lo que dice el ticket del renglón"`
	IngredientID   *int64 `json:"ingredient_id,omitempty" jsonschema:"insumo; o product_id; ninguno = renglón sin mercancía"`
	ProductID      *int64 `json:"product_id,omitempty" jsonschema:"producto de reventa; o ingredient_id"`
	PresentationID *int64 `json:"presentation_id,omitempty" jsonschema:"empaque del insumo (de presentations_list); o unit_id"`
	UnitID         *int16 `json:"unit_id,omitempty" jsonschema:"unidad en que se compró; o presentation_id"`
	Quantity       string `json:"quantity" jsonschema:"cuántos empaques o cuánto en la unidad, texto decimal"`
	Amount         string `json:"amount" jsonschema:"importe del renglón, texto decimal"`
	IsGift         bool   `json:"is_gift,omitempty" jsonschema:"true si vino de regalo (importe 0)"`
	ArrivedOn      string `json:"arrived_on,omitempty" jsonschema:"día en que llegó, AAAA-MM-DD; vacío = queda por recibir"`
}

type expenseEditArgs struct {
	writeCommon
	ExpenseID     int64             `json:"expense_id" jsonschema:"el gasto o compra (de expenses_list)"`
	SupplierID    *int64            `json:"supplier_id,omitempty" jsonschema:"proveedor nuevo (de suppliers_list)"`
	ClearSupplier bool              `json:"clear_supplier,omitempty" jsonschema:"true quita el proveedor; no va con supplier_id"`
	CategoryID    *int64            `json:"category_id,omitempty" jsonschema:"categoría nueva"`
	Description   *string           `json:"description,omitempty" jsonschema:"comentario nuevo; vacío lo quita"`
	ExpenseDate   *string           `json:"expense_date,omitempty" jsonschema:"fecha del documento nueva, AAAA-MM-DD"`
	Remove        []int64           `json:"remove,omitempty" jsonschema:"renglones que se quitan (ids de expense_get)"`
	Add           []editLineArg     `json:"add,omitempty" jsonschema:"renglones que entran"`
	Decisions     map[string]string `json:"decisions,omitempty" jsonschema:"las decisiones que pidió la vista previa: {clave: opción}"`
	Fingerprint   string            `json:"fingerprint,omitempty" jsonschema:"obligatorio al aplicar: el fingerprint de la última vista previa"`
}

// ---- Empaques ----

// presentationNow es el empaque por id, leído bajo la empresa del token: uno de otra empresa no existe.
func presentationNow(s *mcpSession, ctx context.Context, tool string, id int64) (map[string]any, error) {
	b, _, err := s.fetch(ctx, tool, pPresentations, nil, nil)
	if err != nil {
		return nil, err
	}
	p := findByID(b, id)
	if p == nil {
		return nil, toolErr("No existe el empaque %d en esta empresa. Búscalo con presentations_list.", id)
	}
	return p, nil
}

// baseUnitID es el id de la unidad con ese código y equivalencia 1: la unidad base de un insumo.
func baseUnitID(units json.RawMessage, code string) (int64, bool) {
	var list []struct {
		ID     int64           `json:"id"`
		Code   string          `json:"code"`
		ToBase decimal.Decimal `json:"toBase"`
	}
	if json.Unmarshal(units, &list) != nil {
		var env struct {
			Items json.RawMessage `json:"items"`
		}
		if json.Unmarshal(units, &env) != nil || json.Unmarshal(env.Items, &list) != nil {
			return 0, false
		}
	}
	for _, u := range list {
		if u.Code == code && u.ToBase.Equal(decimal.NewFromInt(1)) {
			return u.ID, true
		}
	}
	return 0, false
}

func presentationState(s *mcpSession, ctx context.Context, tool string, ingredient int64) any {
	b, _, err := s.fetch(ctx, tool, pPresentations, nil, nil)
	if err != nil {
		return map[string]string{"unavailable": err.Error()}
	}
	return map[string]any{"ingredient_id": ingredient, "presentations": presentationsOf(b, ingredient)}
}

func register074() {
	writeTool(writeSpec[presentationCreateArgs]{
		name: "presentation_create",
		route: func(_ *mcpSession, _ context.Context, in presentationCreateArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("ingredient_id", in.IngredientID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if in.UnitID <= 0 {
				return "", "", nil, nil, nil, toolErr("unit_id es obligatorio: la unidad de qty (de units_list).")
			}
			body := map[string]any{"name": in.Name, "qty": in.Qty, "unitId": in.UnitID, "approximate": in.Approximate}
			if in.Brand != nil {
				body["brand"] = *in.Brand
			}
			return http.MethodPost, pIngredientPresentations, ids(in.IngredientID), nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, in presentationCreateArgs, _ json.RawMessage) any {
			return presentationState(s, ctx, "presentation_create", in.IngredientID)
		},
	})

	writeTool(writeSpec[presentationUpdateArgs]{
		name: "presentation_update",
		route: func(s *mcpSession, ctx context.Context, in presentationUpdateArgs) (string, string, map[string]int64, url.Values, any, error) {
			const tool = "presentation_update"
			if err := positive("presentation_id", in.PresentationID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if in.Name == nil && in.Qty == nil && in.UnitID == nil && in.Brand == nil && in.Approximate == nil && in.Active == nil {
				return "", "", nil, nil, nil, toolErr("Manda al menos uno de name, qty con unit_id, brand, approximate o active.")
			}
			if (in.Qty == nil) != (in.UnitID == nil) {
				return "", "", nil, nil, nil, toolErr("qty y unit_id van juntos: el contenido nuevo y su unidad.")
			}
			cur, err := presentationNow(s, ctx, tool, in.PresentationID)
			if err != nil {
				return "", "", nil, nil, nil, err
			}
			// La ruta reemplaza el registro: lo que el agente no manda se manda como está hoy, nunca vacío.
			approx, _ := cur["approximate"].(bool)
			body := map[string]any{"name": cur["name"], "approximate": approx, "active": cur["active"]}
			if in.Qty != nil {
				body["qty"], body["unitId"] = *in.Qty, *in.UnitID
			} else {
				// El contenido de hoy: un aproximado lo guarda tal cual con su unidad de compra; uno normal,
				// en la unidad base del insumo.
				body["qty"] = cur["qtyInBase"]
				if approx {
					body["unitId"] = cur["purchaseUnitId"]
				} else {
					units, _, err := s.fetch(ctx, tool, pUnits, nil, nil)
					code, _ := cur["baseUnitCode"].(string)
					id, ok := baseUnitID(units, code)
					if err != nil || !ok {
						return "", "", nil, nil, nil, toolErr("No se pudo leer la unidad del empaque: manda qty y unit_id con su contenido.")
					}
					body["unitId"] = id
				}
			}
			if in.Name != nil {
				body["name"] = *in.Name
			}
			if in.Approximate != nil {
				body["approximate"] = *in.Approximate
			}
			if in.Active != nil {
				body["active"] = *in.Active
			}
			if in.Brand != nil {
				body["brand"] = *in.Brand
			}
			return http.MethodPut, pPresentation, ids(in.PresentationID), nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, in presentationUpdateArgs, _ json.RawMessage) any {
			p, err := presentationNow(s, ctx, "presentation_update", in.PresentationID)
			if err != nil {
				return map[string]string{"unavailable": err.Error()}
			}
			return p
		},
	})

	writeTool(writeSpec[expenseEditArgs]{
		name: "expense_edit",
		route: func(_ *mcpSession, _ context.Context, in expenseEditArgs) (string, string, map[string]int64, url.Values, any, error) {
			body, err := expenseEditBody(in)
			if err != nil {
				return "", "", nil, nil, nil, err
			}
			if in.DryRun != nil && !*in.DryRun && strings.TrimSpace(in.Fingerprint) == "" {
				return "", "", nil, nil, nil, toolErr("Para aplicar manda fingerprint: el de la última vista previa (pídela sin dry_run) " +
					"y las decisions que haya pedido.")
			}
			return http.MethodPost, pExpenseEdit, ids(in.ExpenseID), nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, in expenseEditArgs, _ json.RawMessage) any {
			return s.soft(ctx, "expense_edit", pExpense, ids(in.ExpenseID), nil)
		},
		// La vista previa es la de la ruta: la edición completa en una transacción que se revierte. Lo
		// que la ruta rechaza (gasto anulado, renglón ajeno, permiso de recibir) es un error, no una nota.
		strictPreview: func(s *mcpSession, ctx context.Context, in expenseEditArgs) (any, []string, error) {
			body, err := expenseEditBody(in)
			if err != nil {
				return nil, nil, err
			}
			status, resp, err := s.dispatch(ctx, "expense_edit", http.MethodPost, pExpenseEdit, ids(in.ExpenseID),
				url.Values{"preview": {"1"}}, body)
			if err != nil {
				return nil, nil, toolErr("Error interno del servidor MCP. Avísale al administrador.")
			}
			if status >= 400 {
				return nil, nil, toolError{msg: toolHint(status, resp, mcpListTool["expense_edit"]), status: status}
			}
			var notes []string
			for i, l := range in.Add {
				if (l.IngredientID != nil || l.ProductID != nil) && l.ArrivedOn == "" {
					notes = append(notes, fmt.Sprintf("El renglón agregado %d («%s») queda por recibir: no entra al almacén. "+
						"Si ya llegó, manda arrived_on con el día en que llegó.", i+1, strings.TrimSpace(l.Name)))
				}
			}
			var pv struct {
				Preview struct {
					Decisions []struct {
						Key     string   `json:"key"`
						Options []string `json:"options"`
						Chosen  *string  `json:"chosen"`
					} `json:"decisions"`
				} `json:"preview"`
			}
			_ = json.Unmarshal(resp, &pv)
			for _, d := range pv.Preview.Decisions {
				if d.Chosen == nil {
					notes = append(notes, fmt.Sprintf("Falta decidir %s (%s): pregúntalo y mándalo en decisions.",
						d.Key, strings.Join(d.Options, " o ")))
				}
			}
			notes = append(notes, "Para aplicar: dry_run: false, una idempotency_key nueva y el fingerprint de esta vista previa. "+
				"Si algo cambia antes, vuelve a pedir la vista previa.")
			return json.RawMessage(resp), notes, nil
		},
	})
}

// expenseEditBody arma el cuerpo de la pantalla. Solo revisa la forma de los argumentos; lo que vale
// o no una edición lo decide la ruta.
func expenseEditBody(in expenseEditArgs) (map[string]any, error) {
	if err := positive("expense_id", in.ExpenseID); err != nil {
		return nil, err
	}
	if in.ClearSupplier && in.SupplierID != nil {
		return nil, toolErr("Manda supplier_id para cambiar el proveedor o clear_supplier para quitarlo, no los dos.")
	}
	if len(in.Add)+len(in.Remove) > maxPurchaseLines {
		return nil, toolErr("Una edición agrega y quita a lo más %d renglones.", maxPurchaseLines)
	}
	body := map[string]any{"remove": in.Remove, "decisions": in.Decisions, "fingerprint": in.Fingerprint}
	if in.Remove == nil {
		body["remove"] = []int64{}
	}
	header := map[string]any{}
	if in.ClearSupplier {
		header["supplierId"] = nil
	} else if in.SupplierID != nil {
		header["supplierId"] = *in.SupplierID
	}
	if in.CategoryID != nil {
		header["categoryId"] = *in.CategoryID
	}
	if in.Description != nil {
		header["description"] = *in.Description
	}
	if in.ExpenseDate != nil {
		header["expenseDate"] = *in.ExpenseDate
	}
	if len(header) > 0 {
		body["header"] = header
	}
	add := make([]map[string]any, 0, len(in.Add))
	for i, l := range in.Add {
		if strings.TrimSpace(l.Name) == "" {
			return nil, toolErr("El renglón agregado %d necesita name: lo que dice el ticket.", i+1)
		}
		if l.IngredientID != nil && l.ProductID != nil {
			return nil, toolErr("El renglón agregado %d lleva ingredient_id o product_id, no los dos.", i+1)
		}
		it := map[string]any{"description": strings.TrimSpace(l.Name), "quantity": l.Quantity, "amount": l.Amount,
			"isGift": l.IsGift, "presentationId": l.PresentationID, "unitId": l.UnitID, "arrivedOn": l.ArrivedOn, "itemType": ""}
		switch {
		case l.IngredientID != nil:
			it["itemType"], it["ingredientId"] = "ingrediente", *l.IngredientID
		case l.ProductID != nil:
			it["itemType"], it["productId"] = "producto", *l.ProductID
		}
		add = append(add, it)
	}
	body["add"] = add
	return body, nil
}
