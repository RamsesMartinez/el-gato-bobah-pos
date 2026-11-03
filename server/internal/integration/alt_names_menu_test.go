//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// T026: el menú del POS lleva los alternativos para buscar, guardarlos invalida el menú de las
// tabletas, y lo que se vende (y se imprime en ticket y comanda) lleva solo el nombre principal.
func TestAltNamesReachTheMenuButNotTheSale(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	admin := app.NewAdminService(st)
	menu := app.NewMenuService(st, nil)
	ice := makeProduct(t, st, "Ice Coffee menú", decimal.RequireFromString("65"), false)
	plain := makeProduct(t, st, "Agua menú", decimal.RequireFromString("20"), false)

	before, err := menu.Version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.SetProductAltNames(ctx, ice, []string{"Café frío menú"}); err != nil {
		t.Fatal(err)
	}
	after, err := menu.Version(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Catalog == before.Catalog {
		t.Fatal("guardar alternativos no subió la versión del catálogo: las tabletas buscarían con el menú viejo")
	}

	doc, err := menu.Build(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Products []map[string]any `json:"products"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	for _, p := range parsed.Products {
		id := int64(p["id"].(float64))
		alts, has := p["altNames"]
		switch id {
		case ice:
			if !has || alts.([]any)[0] != "Café frío menú" {
				t.Fatalf("el menú no trae el alternativo: %v", p)
			}
		case plain:
			if has {
				t.Fatalf("un producto sin alternativos los manda igual (engorda el menú): %v", alts)
			}
		}
	}

	cajero := makeUser(t, st, "cajero_alt_menu", "cajero")
	abrirCajaPrincipal(t, st, cajero)
	orders := app.NewOrdersService(st, clock)
	ord, err := orders.Create(ctx, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines: []domain.OrderLineInput{{ProductID: ice, Qty: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := orders.Detail(ctx, ord.ID)
	if err != nil {
		t.Fatal(err)
	}
	rawOrder, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rawOrder), "Café frío menú") {
		t.Fatalf("el pedido (de donde salen ticket y comanda) trae el alternativo: %s", rawOrder)
	}
	var lineName string
	if err := st.Pool.QueryRow(ctx, `select product_name from order_lines where order_id = $1`, ord.ID).Scan(&lineName); err != nil {
		t.Fatal(err)
	}
	if lineName != "Ice Coffee menú" {
		t.Fatalf("el renglón se guardó como %q", lineName)
	}
	entregarPendientes(t, st)
}
