//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
)

// Las herramientas de la spec 099 por la puerta del MCP: vista previa sin escribir, aplicar con
// clave, repetir sin duplicar, y la bitácora diciendo que vino del agente.

func mcp099Router(w mcpWorld) func(*httpapi.Deps) {
	return func(d *httpapi.Deps) { d.Costing = app.NewCostingService(w.app) }
}

func auditDevice(t *testing.T, w mcpWorld, action string, entity int64) string {
	t.Helper()
	var device string
	if err := w.owner.Pool.QueryRow(context.Background(), `select coalesce(device::text, '') from audit_events
		where action = $1 and entity_id = $2 order by id desc limit 1`, action, entity).Scan(&device); err != nil {
		t.Fatalf("sin renglón %s: %v", action, err)
	}
	return device
}

func TestMCPIngredientExpectedWaste(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	router := mcpRouter(w.app, w.jm, mcp099Router(w))
	tok := w.token(t, w.company, w.admin, "read", "write:catalog")
	k := seedCostCatalog(t, w.owner, w.company, " mcp")

	t.Run("la vista previa enseña antes, después y usos, y no escribe", func(t *testing.T) {
		res := callTool(t, router, tok, "ingredient_expected_waste_set", map[string]any{"ingredient_id": k.leche, "pct": "10", "reason": "rota poco"})
		previewed(t, res)
		for _, want := range []string{"Frappé costo mcp", "Extra leche", `"unchanged":false`, "Sin captura"} {
			if !strings.Contains(res.text, want) {
				t.Errorf("la vista previa no dice %q: %s", want, res.text)
			}
		}
		if n := scalar(t, w, `select count(*) from ingredients where id = $1 and expected_waste_pct is not null`, k.leche); n != 0 {
			t.Fatal("la vista previa guardó el %")
		}
	})

	t.Run("argumentos que se rechazan antes de despachar", func(t *testing.T) {
		refused(t, callTool(t, router, tok, "ingredient_expected_waste_set", map[string]any{"ingredient_id": k.leche, "reason": "x"}), "pct o clear")
		refused(t, callTool(t, router, tok, "ingredient_expected_waste_set", map[string]any{"ingredient_id": k.leche, "pct": "8"}), "reason")
		refused(t, callTool(t, router, tok, "ingredient_expected_waste_set", map[string]any{"ingredient_id": k.leche, "pct": "100", "reason": "x"}), "menos de 100")
	})

	t.Run("aplicar guarda lo que anunció la vista previa, una sola vez", func(t *testing.T) {
		args := applyArgs(map[string]any{"ingredient_id": k.leche, "pct": "10", "reason": "rota poco"}, "merma-leche-0001")
		applied(t, callTool(t, router, tok, "ingredient_expected_waste_set", args))
		wantCost(t, w.owner, "el frappé quedó en el «después» de la vista previa", "products", k.frappe, "4.2")
		wantCost(t, w.owner, "el extra de leche también", "modifier_options", k.extraLeche, "2.2")
		again := callTool(t, router, tok, "ingredient_expected_waste_set", args)
		if again.data["replayed"] != true {
			t.Fatalf("la repetición no se reconoció: %s", again.text)
		}
		if n := scalar(t, w, `select count(*) from audit_events where action = 'ingredient.expected_waste' and entity_id = $1`, k.leche); n != 1 {
			t.Fatalf("renglones de bitácora = %d; quiere 1", n)
		}
		if _, after := lastAudit(t, w.owner, "ingredient.expected_waste", k.leche); !strings.Contains(after, "rota poco") {
			t.Fatalf("la bitácora no lleva el motivo: %s", after)
		}
		if d := auditDevice(t, w, "ingredient.expected_waste", k.leche); !strings.Contains(d, `"agent": "mcp"`) {
			t.Fatalf("la bitácora no dice que vino del agente: %s", d)
		}
	})

	t.Run("clear deja sin captura y 0 deja capturado en cero", func(t *testing.T) {
		applied(t, callTool(t, router, tok, "ingredient_expected_waste_set",
			applyArgs(map[string]any{"ingredient_id": k.leche, "clear": true, "reason": "sin dato"}, "merma-leche-0002")))
		if n := scalar(t, w, `select count(*) from ingredients where id = $1 and expected_waste_pct is null`, k.leche); n != 1 {
			t.Fatal("clear no dejó el insumo sin captura")
		}
		applied(t, callTool(t, router, tok, "ingredient_expected_waste_set",
			applyArgs(map[string]any{"ingredient_id": k.leche, "pct": "0", "reason": "no se pierde"}, "merma-leche-0003")))
		if n := scalar(t, w, `select count(*) from ingredients where id = $1 and expected_waste_pct = 0`, k.leche); n != 1 {
			t.Fatal("0 no quedó capturado en cero")
		}
	})

	t.Run("sin menu.configure no se ofrece", func(t *testing.T) {
		sin := tokenForRole(t, w, "almacen-sin-menu", []string{"inventory.view", "inventory.configure"}, "read", "write:catalog")
		if contains(listTools(t, router, sin), "ingredient_expected_waste_set") {
			t.Fatal("se ofrece sin el permiso de la pantalla")
		}
	})
}

func TestMCPCategories(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	router := mcpRouter(w.app, w.jm)
	tok := w.token(t, w.company, w.admin, "read", "write:catalog")
	ctx := context.Background()
	var vacia, conProductos, viejaInactiva int64
	for _, c := range []struct {
		dst    *int64
		name   string
		active bool
	}{{&vacia, "Desayunos MCP", true}, {&conProductos, "Bebidas MCP", true}, {&viejaInactiva, "Tés viejos MCP", false}} {
		if err := w.owner.Pool.QueryRow(ctx, `insert into categories (company_id, name, is_active) values ($1, $2, $3) returning id`,
			w.company, c.name, c.active).Scan(c.dst); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.owner.Pool.Exec(ctx, `insert into products (company_id, name, category_id, price, is_active) values
		($1, 'Molletes MCP', $2, 10, false), ($1, 'Limonada MCP', $3, 10, true)`, w.company, vacia, conProductos); err != nil {
		t.Fatal(err)
	}

	t.Run("apagar la que ya no tiene productos activos", func(t *testing.T) {
		res := callTool(t, router, tok, "category_update", map[string]any{"category_id": vacia, "active": false})
		previewed(t, res)
		if strings.Contains(res.text, "attention") || !strings.Contains(res.text, "1 productos inactivos") {
			t.Fatalf("vista previa = %s; quiere sin avisos y que conserva 1 inactivo", res.text)
		}
		applied(t, callTool(t, router, tok, "category_update", applyArgs(map[string]any{"category_id": vacia, "active": false}, "apagar-desayunos-1")))
		if contains2(callTool(t, router, tok, "categories_list", map[string]any{}).text, "Desayunos MCP") {
			t.Error("la apagada sigue en la lista de activas")
		}
		if !contains2(callTool(t, router, tok, "categories_list", map[string]any{"include_inactive": true}).text, "Desayunos MCP") {
			t.Error("con include_inactive no sale la apagada")
		}
		if d := auditDevice(t, w, "category.updated", vacia); !strings.Contains(d, `"agent": "mcp"`) {
			t.Fatalf("la bitácora no dice que vino del agente: %s", d)
		}
	})

	t.Run("con productos activos avisa en la vista previa y la ruta lo rechaza con su pista", func(t *testing.T) {
		res := callTool(t, router, tok, "category_update", map[string]any{"category_id": conProductos, "active": false})
		previewed(t, res)
		if !strings.Contains(res.text, "1 productos activos") {
			t.Fatalf("la vista previa no avisa: %s", res.text)
		}
		refused(t, callTool(t, router, tok, "category_update", applyArgs(map[string]any{"category_id": conProductos, "active": false}, "apagar-bebidas-1")),
			"product_update")
	})

	t.Run("alta con parecidas primero, repetición y nombre repetido", func(t *testing.T) {
		res := callTool(t, router, tok, "category_create", map[string]any{"name": "Tés"})
		previewed(t, res)
		if !strings.Contains(res.text, "Tés viejos MCP") {
			t.Fatalf("la vista previa no pone la parecida inactiva: %s", res.text)
		}
		args := applyArgs(map[string]any{"name": "Smoothies MCP", "parent_id": conProductos}, "alta-smoothies-1")
		created := applied(t, callTool(t, router, tok, "category_create", args))
		if created["parentId"] != float64(conProductos) {
			t.Fatalf("alta = %v", created)
		}
		if again := callTool(t, router, tok, "category_create", args); again.data["replayed"] != true {
			t.Fatalf("la repetición creó otra: %s", again.text)
		}
		refused(t, callTool(t, router, tok, "category_create", applyArgs(map[string]any{"name": "smoothies mcp", "parent_id": conProductos}, "alta-smoothies-2")),
			"reactívala")
	})

	t.Run("un padre de otra empresa no existe", func(t *testing.T) {
		var ajena int64
		if err := w.owner.Pool.QueryRow(ctx, `insert into categories (company_id, name) values ($1, 'Ajena MCP') returning id`, w.other).Scan(&ajena); err != nil {
			t.Fatal(err)
		}
		if res := callTool(t, router, tok, "category_create", map[string]any{"name": "Hija", "parent_id": ajena}); !res.isError {
			t.Fatalf("la vista previa aceptó un padre ajeno: %s", res.text)
		}
	})
}

func contains2(haystack, needle string) bool { return strings.Contains(haystack, needle) }
