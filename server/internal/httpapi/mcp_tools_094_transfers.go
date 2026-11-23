package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Las herramientas de traspaso entre almacenes (spec 094, historia 5). Despachan la MISMA ruta que la
// pantalla. La clave del traspaso se deriva de la clave de idempotencia de la herramienta: un reintento
// del agente con la misma clave llega a la ruta como el mismo traspaso, aunque la capa del MCP no lo
// hubiera guardado.

//nolint:gosec // G101: patrón de ruta.
const pTransfers = "/api/v1/stock/transfers"

func init() {
	mcpToolRoutes["stock_transfer"] = []mcpRoute{{http.MethodPost, pTransfers}, get(pWarehouses)}
	mcpToolRoutes["stock_transfers_list"] = []mcpRoute{get(pTransfers)}
	mcpListTool["stock_transfer"] = "warehouses_list"
	register094Transfers()
}

type transferLineArg struct {
	IngredientID *int64 `json:"ingredient_id,omitempty" jsonschema:"insumo (de stock_levels o articles_search); o product_id"`
	ProductID    *int64 `json:"product_id,omitempty" jsonschema:"producto con existencia propia; o ingredient_id"`
	Quantity     string `json:"quantity" jsonschema:"cuánto se mueve, positivo, en la unidad base del artículo"`
}

type stockTransferArgs struct {
	writeCommon
	FromWarehouseID int64             `json:"from_warehouse_id" jsonschema:"almacén de donde sale (de warehouses_list)"`
	ToWarehouseID   int64             `json:"to_warehouse_id" jsonschema:"almacén a donde entra (de warehouses_list)"`
	Lines           []transferLineArg `json:"lines" jsonschema:"de 1 a 100 artículos, sin repetir"`
	Note            string            `json:"note,omitempty" jsonschema:"nota opcional, hasta 200 letras"`
	ConfirmNegative bool              `json:"confirm_negative,omitempty" jsonschema:"true solo si la persona confirma que la mercancía SÍ está en el origen aunque el sistema diga que no alcanza"`
}

func (in stockTransferArgs) body(key string) (map[string]any, error) {
	if err := positive("from_warehouse_id", in.FromWarehouseID); err != nil {
		return nil, err
	}
	if err := positive("to_warehouse_id", in.ToWarehouseID); err != nil {
		return nil, err
	}
	if in.FromWarehouseID == in.ToWarehouseID {
		return nil, toolErr("El origen y el destino son el mismo almacén.")
	}
	if len(in.Lines) == 0 || len(in.Lines) > 100 {
		return nil, toolErr("lines lleva de 1 a 100 artículos.")
	}
	lines := make([]map[string]any, len(in.Lines))
	for i, l := range in.Lines {
		if err := oneOf(l.IngredientID, l.ProductID, "ingredient_id o product_id"); err != nil {
			return nil, err
		}
		line := map[string]any{"itemType": "ingrediente", "ingredientId": l.IngredientID, "qty": l.Quantity}
		if l.ProductID != nil {
			line = map[string]any{"itemType": "producto", "productId": l.ProductID, "qty": l.Quantity}
		}
		lines[i] = line
	}
	b := map[string]any{"fromWarehouseId": in.FromWarehouseID, "toWarehouseId": in.ToWarehouseID, "lines": lines,
		"confirmNegative": in.ConfirmNegative, "clientKey": key}
	if strings.TrimSpace(in.Note) != "" {
		b["note"] = in.Note
	}
	return b, nil
}

func refsStockTransfer(in stockTransferArgs) []domain.Ref {
	var l refList
	l.add(domain.RefWarehouse, in.FromWarehouseID)
	l.add(domain.RefWarehouse, in.ToWarehouseID)
	for _, ln := range in.Lines {
		l.opt(domain.RefIngredient, ln.IngredientID)
		if ln.ProductID != nil {
			l.product(*ln.ProductID)
		}
	}
	return l
}

func register094Transfers() {
	const tool = "stock_transfer"
	writeTool(writeSpec[stockTransferArgs]{
		name: tool,
		refs: refsStockTransfer,
		route: func(s *mcpSession, _ context.Context, in stockTransferArgs) (string, string, map[string]int64, url.Values, any, error) {
			body, err := in.body(s.derived(tool, in.IdempotencyKey).String())
			if err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPost, pTransfers, nil, nil, body, nil
		},
		state: func(_ *mcpSession, _ context.Context, _ stockTransferArgs, result json.RawMessage) any {
			if result == nil {
				return map[string]string{"note": "Las existencias de cada almacén se consultan con stock_levels y warehouse_id."}
			}
			return json.RawMessage(result)
		},
		// La vista previa nombra el origen y el destino: un id cruzado mueve la mercancía al revés.
		preview: func(s *mcpSession, ctx context.Context, in stockTransferArgs) (any, []string) {
			var notes []string
			out := map[string]any{"lines": len(in.Lines)}
			b, _, err := s.fetch(ctx, tool, pWarehouses, nil, nil)
			if err != nil {
				return out, []string{"No se pudo leer la lista de almacenes: " + err.Error()}
			}
			for key, id := range map[string]int64{"from": in.FromWarehouseID, "to": in.ToWarehouseID} {
				if w := findByID(b, id); w != nil {
					out[key] = w
				} else {
					notes = append(notes, fmt.Sprintf("No existe el almacén %d en la sucursal (o está archivado); búscalo con warehouses_list.", id))
				}
			}
			if from, to := out["from"], out["to"]; from != nil && to != nil {
				notes = append(notes, fmt.Sprintf("Sale de «%v» y entra a «%v».", from.(map[string]any)["name"], to.(map[string]any)["name"]))
			}
			if !in.ConfirmNegative {
				notes = append(notes, "Si el origen no alcanza, la ruta responde NEEDS_CONFIRM con lo disponible; "+
					"confirm_negative: true solo si la persona confirma que la mercancía sí está ahí.")
			}
			return out, notes
		},
	})
	simpleRead("stock_transfers_list", pTransfers, func(in limitArg) (map[string]int64, url.Values, error) {
		if in.Limit == 0 {
			return nil, nil, nil
		}
		return nil, url.Values{"limit": {strconv.Itoa(in.Limit)}}, nil
	})
}
