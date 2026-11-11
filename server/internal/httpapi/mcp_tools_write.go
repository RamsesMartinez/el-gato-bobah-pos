package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Las escrituras del servidor MCP. Cada una arma el MISMO cuerpo que manda la pantalla a la MISMA
// ruta; lo único propio es cómo se lee el estado antes y después para la vista previa y la bitácora.

type cashMovementArgs struct {
	writeCommon
	RegisterID int64  `json:"register_id" jsonschema:"id de la caja con turno abierto (de cash_registers_list)"`
	Kind       string `json:"kind" jsonschema:"salida o entrada"`
	Amount     string `json:"amount" jsonschema:"importe en pesos, texto decimal (\"200.00\")"`
	ConceptID  *int64 `json:"concept_id,omitempty" jsonschema:"obligatorio en una salida: id de cash_concepts_list"`
	Concept    string `json:"concept,omitempty" jsonschema:"obligatorio en una entrada: de dónde vino el efectivo"`
}

type paymentArg struct {
	MethodID       int16  `json:"method_id" jsonschema:"id del método (de payment_methods_list)"`
	Amount         string `json:"amount" jsonschema:"importe del pago, texto decimal"`
	RegisterID     *int64 `json:"register_id,omitempty" jsonschema:"caja de la que sale; obligatoria en efectivo"`
	Reference      string `json:"reference,omitempty" jsonschema:"referencia del pago (folio, últimos dígitos)"`
	PaidOn         string `json:"paid_on,omitempty" jsonschema:"día del pago, AAAA-MM-DD; vacío = hoy"`
	IncludeInCount *bool  `json:"include_in_count,omitempty" jsonschema:"efectivo: si entra al arqueo de la caja (por omisión sí, como la pantalla)"`
}

func (p paymentArg) body() map[string]any {
	inc := true
	if p.IncludeInCount != nil {
		inc = *p.IncludeInCount
	}
	return map[string]any{"methodId": p.MethodID, "amount": p.Amount, "paidOn": p.PaidOn,
		"registerId": p.RegisterID, "reference": p.Reference, "includeInCount": inc}
}

type expenseCreateArgs struct {
	writeCommon
	CategoryID  int64        `json:"category_id" jsonschema:"id de la categoría (de expense_categories_list)"`
	Amount      string       `json:"amount" jsonschema:"total del gasto en pesos, texto decimal"`
	ExpenseDate string       `json:"expense_date,omitempty" jsonschema:"fecha del documento, AAAA-MM-DD"`
	Description string       `json:"description,omitempty" jsonschema:"qué se compró"`
	SupplierID  *int64       `json:"supplier_id,omitempty" jsonschema:"id del proveedor (de suppliers_list)"`
	DocFolio    string       `json:"doc_folio,omitempty" jsonschema:"folio del ticket o factura"`
	Payments    []paymentArg `json:"payments,omitempty" jsonschema:"pagos (hasta 10); sin pagos el gasto queda pendiente"`
}

type expensePayArgs struct {
	writeCommon
	ExpenseID int64 `json:"expense_id" jsonschema:"id del gasto pendiente"`
	paymentArg
}

type expenseCancelArgs struct {
	writeCommon
	ExpenseID int64  `json:"expense_id" jsonschema:"id del gasto"`
	Reason    string `json:"reason" jsonschema:"por qué se anula"`
}

type supplierCreateArgs struct {
	writeCommon
	Name  string  `json:"name" jsonschema:"nombre del proveedor"`
	Phone *string `json:"phone,omitempty" jsonschema:"teléfono"`
	Notes *string `json:"notes,omitempty" jsonschema:"notas"`
}

type wasteArgs struct {
	writeCommon
	ReasonID     int64  `json:"reason_id" jsonschema:"motivo (de waste_reasons_list)"`
	IngredientID *int64 `json:"ingredient_id,omitempty" jsonschema:"insumo; o product_id"`
	ProductID    *int64 `json:"product_id,omitempty" jsonschema:"producto de reventa; o ingredient_id"`
	Qty          string `json:"qty" jsonschema:"cantidad que se echó a perder, en la unidad base del artículo"`
	UnitID       *int16 `json:"unit_id,omitempty" jsonschema:"unidad en que se capturó, si no es la base"`
	Note         string `json:"note,omitempty" jsonschema:"nota"`
}

type stockAdjustArgs struct {
	writeCommon
	IngredientID *int64 `json:"ingredient_id,omitempty" jsonschema:"insumo; o product_id"`
	ProductID    *int64 `json:"product_id,omitempty" jsonschema:"producto de reventa; o ingredient_id"`
	Quantity     string `json:"quantity" jsonschema:"cuánto sumar (positivo) o restar (negativo), en la unidad base"`
	Reason       string `json:"reason" jsonschema:"por qué se ajusta"`
}

type productUpdateArgs struct {
	writeCommon
	ProductID int64   `json:"product_id" jsonschema:"id del producto (de products_list)"`
	Price     *string `json:"price,omitempty" jsonschema:"precio nuevo, texto decimal"`
	Active    *bool   `json:"active,omitempty" jsonschema:"true lo ofrece, false lo quita del menú"`
}

type optionUpdateArgs struct {
	writeCommon
	OptionID   int64   `json:"option_id" jsonschema:"id del extra (de modifier_options_list)"`
	PriceDelta *string `json:"price_delta,omitempty" jsonschema:"lo que suma al precio, texto decimal"`
	Active     *bool   `json:"active,omitempty" jsonschema:"true lo ofrece, false lo quita"`
}

type platformPriceArgs struct {
	writeCommon
	PlatformID int16  `json:"platform_id" jsonschema:"plataforma (de platform_prices_list)"`
	ProductID  int64  `json:"product_id,omitempty" jsonschema:"producto; o option_id"`
	OptionID   int64  `json:"option_id,omitempty" jsonschema:"extra; o product_id"`
	Price      string `json:"price,omitempty" jsonschema:"precio (producto) o lo que suma (extra), texto decimal; solo en platform_price_set"`
}

// maxExpensePayments acota los pagos de un gasto capturado por un agente.
const maxExpensePayments = 10

func ids(id int64) map[string]int64 { return map[string]int64{"id": id} }

func oneOf(a, b *int64, names string) error {
	if (a == nil) == (b == nil) || (a != nil && *a <= 0) || (b != nil && *b <= 0) {
		return toolErr("Manda exactamente uno de %s, con un id mayor que cero.", names)
	}
	return nil
}

// findByID busca un renglón por id en una lista {items:[…]} o […].
func findByID(body json.RawMessage, id int64) map[string]any {
	var env struct {
		Items []map[string]any `json:"items"`
	}
	if json.Unmarshal(body, &env) != nil || env.Items == nil {
		_ = json.Unmarshal(body, &env.Items)
	}
	for _, it := range env.Items {
		if v, ok := it["id"].(float64); ok && int64(v) == id {
			return it
		}
	}
	return nil
}

func articleState(s *mcpSession, ctx context.Context, tool string, ingredient, product *int64) any {
	if ingredient != nil {
		return s.soft(ctx, tool, pArticle, ids(*ingredient), nil)
	}
	return map[string]string{"note": "La existencia de un producto se consulta con stock_levels."}
}

func registerWrites() {
	writeTool(writeSpec[cashMovementArgs]{
		name: "cash_movement_create",
		route: func(_ *mcpSession, _ context.Context, in cashMovementArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("register_id", in.RegisterID); err != nil {
				return "", "", nil, nil, nil, err
			}
			switch in.Kind {
			case "salida":
				if in.ConceptID == nil {
					return "", "", nil, nil, nil, toolErr("Una salida necesita concept_id. Búscalo con cash_concepts_list.")
				}
			case "entrada":
				if strings.TrimSpace(in.Concept) == "" {
					return "", "", nil, nil, nil, toolErr("Una entrada necesita concept: de dónde vino el efectivo.")
				}
			default:
				return "", "", nil, nil, nil, toolErr("kind es salida o entrada.")
			}
			body := map[string]any{"registerId": in.RegisterID, "kind": in.Kind, "amount": in.Amount,
				"concept": in.Concept, "conceptId": in.ConceptID}
			return http.MethodPost, pCashMovement, nil, nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, in cashMovementArgs, _ json.RawMessage) any {
			return s.soft(ctx, "cash_movement_create", pCashCurrent, nil,
				url.Values{"registerId": {strconv.FormatInt(in.RegisterID, 10)}})
		},
	})

	writeTool(writeSpec[expenseCreateArgs]{
		name: "expense_create",
		route: func(_ *mcpSession, _ context.Context, in expenseCreateArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("category_id", in.CategoryID); err != nil {
				return "", "", nil, nil, nil, err
			}
			// Un gasto real trae uno o dos pagos. Sin tope, cada pago de la vista previa despacha sus
			// lecturas y una sola llamada multiplicaba las peticiones internas por miles.
			if len(in.Payments) > maxExpensePayments {
				return "", "", nil, nil, nil, toolErr("Un gasto lleva a lo más %d pagos.", maxExpensePayments)
			}
			total, err := decimal.NewFromString(in.Amount)
			if err != nil {
				return "", "", nil, nil, nil, toolErr("amount es un importe en pesos, como \"845.50\".")
			}
			payments := make([]map[string]any, 0, len(in.Payments))
			paid := make([]decimal.Decimal, 0, len(in.Payments))
			for _, p := range in.Payments {
				a, err := decimal.NewFromString(p.Amount)
				if err != nil {
					return "", "", nil, nil, nil, toolErr("El importe de cada pago es texto decimal, como \"845.50\".")
				}
				paid = append(paid, a)
				payments = append(payments, p.body())
			}
			status := domain.ExpenseStatusFromPayments(total, paid)
			body := map[string]any{"expenseDate": in.ExpenseDate, "categoryId": in.CategoryID,
				"supplierId": in.SupplierID, "amount": in.Amount, "description": in.Description,
				"status": status, "payments": payments, "docFolio": in.DocFolio}
			return http.MethodPost, pExpenses, nil, nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, _ expenseCreateArgs, result json.RawMessage) any {
			var r struct {
				ID int64 `json:"id"`
			}
			if result == nil || json.Unmarshal(result, &r) != nil || r.ID == 0 {
				return nil
			}
			return s.soft(ctx, "expense_create", pExpense, ids(r.ID), nil)
		},
		preview: func(s *mcpSession, ctx context.Context, in expenseCreateArgs) (any, []string) {
			const tool = "expense_create"
			out := map[string]any{}
			var notes []string
			// lookup resuelve un id en una lista; si no se pudo leer o no existe, lo dice en vez de
			// dejar un null que el agente no sabe leer.
			lists := map[string]json.RawMessage{}
			lookup := func(pattern string, id int64, what, listTool string) any {
				b, ok := lists[pattern]
				var err error
				if !ok {
					b, _, err = s.fetch(ctx, tool, pattern, nil, nil)
					lists[pattern] = b
				}
				if err != nil {
					notes = append(notes, fmt.Sprintf("No se pudo revisar %s: %s", what, err.Error()))
					return nil
				}
				row := findByID(b, id)
				if row == nil {
					notes = append(notes, fmt.Sprintf("No existe %s %d en esta empresa; búscalo con %s.", what, id, listTool))
					return nil
				}
				return row
			}
			out["category"] = lookup(pExpenseCats, in.CategoryID, "la categoría", "expense_categories_list")
			if in.SupplierID != nil {
				out["supplier"] = lookup(pSuppliers, *in.SupplierID, "el proveedor", "suppliers_list")
			}
			if len(in.Payments) > maxExpensePayments {
				return out, notes
			}
			shifts := map[int64]any{}
			pays := []any{}
			for _, p := range in.Payments {
				pay := map[string]any{"method": lookup(pPaymentMethods, int64(p.MethodID), "el método de pago", "payment_methods_list"),
					"amount": p.Amount}
				if p.RegisterID != nil {
					if _, seen := shifts[*p.RegisterID]; !seen {
						shifts[*p.RegisterID] = s.soft(ctx, tool, pCashCurrent, nil,
							url.Values{"registerId": {strconv.FormatInt(*p.RegisterID, 10)}})
					}
					pay["register_shift"] = shifts[*p.RegisterID]
				}
				pays = append(pays, pay)
			}
			out["payments"] = pays
			if len(in.Payments) == 0 {
				notes = append(notes, "Sin pagos: el gasto queda pendiente de pago.")
			}
			return out, notes
		},
	})

	expenseState := func(tool string) func(s *mcpSession, ctx context.Context, id int64) any {
		return func(s *mcpSession, ctx context.Context, id int64) any {
			return s.soft(ctx, tool, pExpense, ids(id), nil)
		}
	}
	payState := expenseState("expense_pay")
	writeTool(writeSpec[expensePayArgs]{
		name: "expense_pay",
		route: func(_ *mcpSession, _ context.Context, in expensePayArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("expense_id", in.ExpenseID); err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPost, pExpensePay, ids(in.ExpenseID), nil, in.body(), nil
		},
		state: func(s *mcpSession, ctx context.Context, in expensePayArgs, _ json.RawMessage) any {
			return payState(s, ctx, in.ExpenseID)
		},
	})
	cancelState := expenseState("expense_cancel")
	writeTool(writeSpec[expenseCancelArgs]{
		name: "expense_cancel",
		route: func(_ *mcpSession, _ context.Context, in expenseCancelArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("expense_id", in.ExpenseID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if strings.TrimSpace(in.Reason) == "" {
				return "", "", nil, nil, nil, toolErr("reason es obligatorio: por qué se anula el gasto.")
			}
			return http.MethodPost, pExpenseCancel, ids(in.ExpenseID), nil, map[string]any{"reason": in.Reason}, nil
		},
		state: func(s *mcpSession, ctx context.Context, in expenseCancelArgs, _ json.RawMessage) any {
			return cancelState(s, ctx, in.ExpenseID)
		},
	})

	writeTool(writeSpec[supplierCreateArgs]{
		name: "supplier_create",
		route: func(_ *mcpSession, _ context.Context, in supplierCreateArgs) (string, string, map[string]int64, url.Values, any, error) {
			if strings.TrimSpace(in.Name) == "" {
				return "", "", nil, nil, nil, toolErr("name es obligatorio.")
			}
			return http.MethodPost, pSuppliers, nil, nil,
				map[string]any{"name": in.Name, "phone": in.Phone, "notes": in.Notes}, nil
		},
		preview: func(s *mcpSession, ctx context.Context, in supplierCreateArgs) (any, []string) {
			b, _, err := s.fetch(ctx, "supplier_create", pSuppliers, nil, nil)
			if err != nil {
				return nil, nil
			}
			var env struct {
				Items []map[string]any `json:"items"`
			}
			_ = json.Unmarshal(b, &env)
			want := strings.ToLower(strings.TrimSpace(in.Name))
			var similar []map[string]any
			for _, it := range env.Items {
				name, _ := it["name"].(string)
				n := strings.ToLower(name)
				if strings.Contains(n, want) || strings.Contains(want, n) {
					similar = append(similar, map[string]any{"id": it["id"], "name": name})
				}
			}
			if len(similar) > 0 {
				return map[string]any{"similar": similar}, []string{"Ya hay proveedores con un nombre parecido: revisa si es uno de ellos antes de crear otro."}
			}
			return nil, nil
		},
	})

	writeTool(writeSpec[wasteArgs]{
		name: "waste_record",
		route: func(_ *mcpSession, _ context.Context, in wasteArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("reason_id", in.ReasonID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if err := oneOf(in.IngredientID, in.ProductID, "ingredient_id o product_id"); err != nil {
				return "", "", nil, nil, nil, err
			}
			body := map[string]any{"reasonId": in.ReasonID, "ingredientId": in.IngredientID,
				"productId": in.ProductID, "qty": in.Qty, "unitId": in.UnitID}
			if in.Note != "" {
				body["note"] = in.Note
			}
			return http.MethodPost, pWaste, nil, nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, in wasteArgs, _ json.RawMessage) any {
			return articleState(s, ctx, "waste_record", in.IngredientID, in.ProductID)
		},
	})

	writeTool(writeSpec[stockAdjustArgs]{
		name: "stock_adjust",
		route: func(_ *mcpSession, _ context.Context, in stockAdjustArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := oneOf(in.IngredientID, in.ProductID, "ingredient_id o product_id"); err != nil {
				return "", "", nil, nil, nil, err
			}
			if strings.TrimSpace(in.Reason) == "" {
				return "", "", nil, nil, nil, toolErr("reason es obligatorio: por qué se ajusta la existencia.")
			}
			itemType := "ingrediente"
			if in.ProductID != nil {
				itemType = "producto"
			}
			body := map[string]any{"itemType": itemType, "ingredientId": in.IngredientID, "productId": in.ProductID,
				"movementType": "ajuste", "quantity": in.Quantity, "reason": in.Reason}
			return http.MethodPost, pStockMoves, nil, nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, in stockAdjustArgs, _ json.RawMessage) any {
			return articleState(s, ctx, "stock_adjust", in.IngredientID, in.ProductID)
		},
	})

	// El producto se edita con el registro completo (así lo manda la pantalla): se lee el actual y
	// solo cambia lo que el agente pidió. Nunca se manda un campo con un valor inventado.
	productNow := func(s *mcpSession, ctx context.Context, id int64) (map[string]any, error) {
		b, _, err := s.fetch(ctx, "product_update", pProducts, nil, url.Values{"limit": {"0"}})
		if err != nil {
			return nil, err
		}
		p := findByID(b, id)
		if p == nil {
			return nil, toolErr("No existe el producto %d en esta empresa. Búscalo con products_list.", id)
		}
		return p, nil
	}
	writeTool(writeSpec[productUpdateArgs]{
		name: "product_update",
		route: func(s *mcpSession, ctx context.Context, in productUpdateArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("product_id", in.ProductID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if in.Price == nil && in.Active == nil {
				return "", "", nil, nil, nil, toolErr("Manda price, active o los dos.")
			}
			cur, err := productNow(s, ctx, in.ProductID)
			if err != nil {
				return "", "", nil, nil, nil, err
			}
			body := map[string]any{"name": cur["name"], "price": cur["price"], "favorite": cur["is_favorite"],
				"active": cur["is_active"], "categoryId": cur["categoryId"], "availableFrom": cur["availableFrom"],
				"availableUntil": cur["availableUntil"], "needsPrep": cur["needsPrep"]}
			if in.Price != nil {
				body["price"] = *in.Price
			}
			if in.Active != nil {
				body["active"] = *in.Active
			}
			return http.MethodPatch, pProduct, ids(in.ProductID), nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, in productUpdateArgs, _ json.RawMessage) any {
			p, err := productNow(s, ctx, in.ProductID)
			if err != nil {
				return nil
			}
			return map[string]any{"id": p["id"], "name": p["name"], "price": p["price"], "active": p["is_active"]}
		},
	})

	// El precio de un extra viaja con su nombre y su tope por renglón: el tope no está en la lista
	// de extras, se lee de las opciones de su grupo para no regresarlo a 1.
	optionNow := func(s *mcpSession, ctx context.Context, id int64) (map[string]any, error) {
		b, _, err := s.fetch(ctx, "modifier_option_update", pOptions, nil, url.Values{"limit": {"0"}})
		if err != nil {
			return nil, err
		}
		o := findByID(b, id)
		if o == nil {
			return nil, toolErr("No existe el extra %d en esta empresa. Búscalo con modifier_options_list.", id)
		}
		group, _ := o["groupId"].(float64)
		gb, _, err := s.fetch(ctx, "modifier_option_update", pGroupOptions, ids(int64(group)), nil)
		if err != nil {
			return nil, err
		}
		full := findByID(gb, id)
		if full == nil {
			return nil, toolErr("No se pudo leer el tope por renglón del extra %d.", id)
		}
		o["maxPerLine"] = full["maxPerLine"]
		return o, nil
	}
	writeTool(writeSpec[optionUpdateArgs]{
		name: "modifier_option_update",
		route: func(s *mcpSession, ctx context.Context, in optionUpdateArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("option_id", in.OptionID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if in.PriceDelta == nil && in.Active == nil {
				return "", "", nil, nil, nil, toolErr("Manda price_delta, active o los dos.")
			}
			body := map[string]any{}
			if in.PriceDelta != nil {
				cur, err := optionNow(s, ctx, in.OptionID)
				if err != nil {
					return "", "", nil, nil, nil, err
				}
				body["name"], body["priceDelta"], body["maxPerLine"] = cur["name"], *in.PriceDelta, cur["maxPerLine"]
			}
			if in.Active != nil {
				body["active"] = *in.Active
			}
			return http.MethodPatch, pOption, ids(in.OptionID), nil, body, nil
		},
		state: func(s *mcpSession, ctx context.Context, in optionUpdateArgs, _ json.RawMessage) any {
			o, err := optionNow(s, ctx, in.OptionID)
			if err != nil {
				return nil
			}
			return map[string]any{"id": o["id"], "name": o["name"], "priceDelta": o["priceDelta"],
				"active": o["active"], "maxPerLine": o["maxPerLine"]}
		},
	})

	priceRoute := func(set bool) func(*mcpSession, context.Context, platformPriceArgs) (string, string, map[string]int64, url.Values, any, error) {
		return func(_ *mcpSession, _ context.Context, in platformPriceArgs) (string, string, map[string]int64, url.Values, any, error) {
			if in.PlatformID <= 0 {
				return "", "", nil, nil, nil, toolErr("platform_id es obligatorio. Búscalo con platform_prices_list.")
			}
			if (in.ProductID == 0) == (in.OptionID == 0) || in.ProductID < 0 || in.OptionID < 0 {
				return "", "", nil, nil, nil, toolErr("Manda exactamente uno de product_id u option_id.")
			}
			if set && in.Price == "" {
				return "", "", nil, nil, nil, toolErr("price es obligatorio: el precio en esa plataforma, texto decimal.")
			}
			pattern := pPriceProduct
			if in.OptionID != 0 {
				pattern = pPriceOption
			}
			if !set {
				q := url.Values{"platformId": {strconv.Itoa(int(in.PlatformID))}}
				if in.OptionID != 0 {
					q.Set("optionId", strconv.FormatInt(in.OptionID, 10))
				} else {
					q.Set("productId", strconv.FormatInt(in.ProductID, 10))
				}
				return http.MethodDelete, pattern, nil, q, nil, nil
			}
			if in.OptionID != 0 {
				return http.MethodPut, pattern, nil, nil,
					map[string]any{"optionId": in.OptionID, "platformId": in.PlatformID, "priceDelta": in.Price}, nil
			}
			return http.MethodPut, pattern, nil, nil,
				map[string]any{"productId": in.ProductID, "platformId": in.PlatformID, "price": in.Price}, nil
		}
	}
	priceState := func(tool string) func(*mcpSession, context.Context, platformPriceArgs, json.RawMessage) any {
		return func(s *mcpSession, ctx context.Context, in platformPriceArgs, _ json.RawMessage) any {
			b, _, err := s.fetch(ctx, tool, pPosMenu, nil, nil)
			if err != nil {
				return nil
			}
			return projectPlatformPrices(b, in.ProductID, in.OptionID)
		}
	}
	writeTool(writeSpec[platformPriceArgs]{name: "platform_price_set", route: priceRoute(true),
		state: priceState("platform_price_set")})
	writeTool(writeSpec[platformPriceArgs]{name: "platform_price_delete", route: priceRoute(false),
		state: priceState("platform_price_delete")})
}
