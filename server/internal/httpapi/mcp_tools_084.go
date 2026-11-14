package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/shopspring/decimal"
)

// La herramienta de la spec 084: encender o apagar las alertas de stock bajo de varios artículos a la
// vez, con su mínimo. Despacha la MISMA ruta que la tabla «Alertas por lote»; la vista previa es la de
// la ruta (?preview=1), que dice qué avisos se abrirían o cerrarían al guardar.

//nolint:gosec // G101: patrón de ruta.
const pLowStockBatch = "/api/v1/stock/low-stock/batch"

func init() {
	mcpToolRoutes["low_stock_update"] = []mcpRoute{{http.MethodPost, pLowStockBatch}}
	mcpListTool["low_stock_update"] = "articles_search"
	register084()
}

type lowStockItemArg struct {
	ItemType string  `json:"item_type" jsonschema:"ingrediente o producto (producto solo con existencia propia)"`
	ID       int64   `json:"id" jsonschema:"id del insumo o del producto"`
	Enabled  bool    `json:"enabled" jsonschema:"true enciende la alerta de stock bajo; false la apaga"`
	MinStock *string `json:"min_stock,omitempty" jsonschema:"mínimo en la unidad base del artículo, texto decimal; 0 o ausente = sin mínimo (solo avisa al acabarse)"`
}

type lowStockUpdateArgs struct {
	writeCommon
	Items []lowStockItemArg `json:"items" jsonschema:"de 1 a 100 artículos"`
}

func (in lowStockUpdateArgs) body() (map[string]any, error) {
	if len(in.Items) == 0 || len(in.Items) > 100 {
		return nil, toolErr("items lleva de 1 a 100 artículos.")
	}
	items := make([]map[string]any, len(in.Items))
	for i, it := range in.Items {
		if it.ItemType != "ingrediente" && it.ItemType != "producto" {
			return nil, toolErr("item_type es ingrediente o producto.")
		}
		var min *decimal.Decimal
		if it.MinStock != nil {
			v, err := decimal.NewFromString(*it.MinStock)
			if err != nil {
				return nil, toolErr("min_stock es un número decimal en texto, como \"500\".")
			}
			min = &v
		}
		items[i] = map[string]any{"itemType": it.ItemType, "id": it.ID, "enabled": it.Enabled, "minStock": min}
	}
	return map[string]any{"items": items}, nil
}

func register084() {
	writeTool(writeSpec[lowStockUpdateArgs]{
		name: "low_stock_update",
		route: func(_ *mcpSession, _ context.Context, in lowStockUpdateArgs) (string, string, map[string]int64, url.Values, any, error) {
			body, err := in.body()
			if err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPost, pLowStockBatch, nil, nil, body, nil
		},
		state: func(_ *mcpSession, _ context.Context, _ lowStockUpdateArgs, result json.RawMessage) any {
			if result == nil {
				return nil
			}
			return json.RawMessage(result)
		},
		// La vista previa es la de la ruta: lo que rechaza (un artículo ajeno, un mínimo absurdo) es un
		// error para el agente, no una nota de una vista previa «exitosa».
		strictPreview: func(s *mcpSession, ctx context.Context, in lowStockUpdateArgs) (any, []string, error) {
			body, err := in.body()
			if err != nil {
				return nil, nil, err
			}
			status, resp, err := s.dispatch(ctx, "low_stock_update", http.MethodPost, pLowStockBatch, nil, url.Values{"preview": {"1"}}, body)
			if err != nil {
				slog.ErrorContext(ctx, "mcp_dispatch_failed", "tool", "low_stock_update", "pattern", pLowStockBatch, "error", err.Error())
				return nil, nil, toolError{msg: "Error interno del servidor MCP. Avísale al administrador.", status: http.StatusInternalServerError}
			}
			if status >= 400 {
				return nil, nil, toolError{msg: toolHint(status, resp, mcpListTool["low_stock_update"]), status: status}
			}
			return json.RawMessage(resp), []string{"opens: avisos que se abrirán al guardar (ya está en o bajo el mínimo); " +
				"closes: los que se cerrarán. Nada se guarda sin aplicar."}, nil
		},
	})
}
