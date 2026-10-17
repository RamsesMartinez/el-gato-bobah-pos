package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Nada de la salud del inventario depende de un negocio en particular (FR-018, SC-006): el producto
// se vende a otras empresas. Revisa el código y las consultas de la spec 037, no las pruebas, que sí
// usan datos de ejemplo.
func TestInventoryHealthCodeNamesNoBusiness(t *testing.T) {
	files := []string{
		"inventory_health.go", "inventory_health_csv.go",
		"../domain/inventory_health.go", "../domain/consumption_mode.go", "../domain/mode_change.go",
		"../httpapi/handlers_inventory_health.go",
		"../../queries/inventory_health.sql",
		"../../../web/src/features/backoffice/InventoryHealthTab.tsx",
		"../../../web/src/shared/createIngredient.ts",
	}
	for _, f := range files {
		b, err := os.ReadFile(filepath.Clean(f))
		if err != nil {
			t.Fatalf("leer %s: %v", f, err)
		}
		text := strings.ToLower(string(b))
		for _, name := range []string{"gatobobah", "gato bobah", "el gato"} {
			if strings.Contains(text, name) {
				t.Errorf("%s nombra al negocio («%s»): lo que distingue a un negocio es dato, no código", f, name)
			}
		}
	}
}
