package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// dbRoles son los roles de la base: llevan el nombre del sistema, no el de un cliente.
var dbRoles = regexp.MustCompile(`gatobobah_(app|platform|webhook)`)

// Nada de compras, empaques ni conteo depende de un negocio en particular (spec 038, FR-018,
// SC-006): el producto se vende a otras empresas. Revisa el código, las consultas y las
// migraciones de la spec, no las pruebas, que sí usan datos de ejemplo.
func TestPurchasesCodeNamesNoBusiness(t *testing.T) {
	files := []string{
		"purchases.go", "purchases_receive.go", "presentations.go", "stock_counts.go", "costing.go",
		"../domain/purchases.go", "../domain/stock_count.go",
		"../httpapi/handlers_purchases.go", "../httpapi/handlers_presentations.go", "../httpapi/handlers_stock_counts.go",
		"../../queries/purchases.sql", "../../queries/stock_counts.sql",
		"../../migrations/0091_unit_kind_length.sql", "../../migrations/0092_purchases_into_stock.sql",
		"../../../web/src/features/backoffice/ExpenseDialog.tsx",
		"../../../web/src/features/backoffice/ExpenseDetailDialog.tsx",
		"../../../web/src/features/backoffice/PendingReceiptsTab.tsx",
		"../../../web/src/features/backoffice/ArrivedItemSheet.tsx",
		"../../../web/src/features/backoffice/StockCountTab.tsx",
		"../../../web/src/features/backoffice/PresentationsSheet.tsx",
		"../../../web/src/features/admin/InventorySettingsSection.tsx",
	}
	for _, f := range files {
		b, err := os.ReadFile(filepath.Clean(f))
		if err != nil {
			t.Fatalf("leer %s: %v", f, err)
		}
		text := dbRoles.ReplaceAllString(strings.ToLower(string(b)), "")
		for _, name := range []string{"gatobobah", "gato bobah", "el gato"} {
			if strings.Contains(text, name) {
				t.Errorf("%s nombra al negocio («%s»): lo que distingue a un negocio es dato, no código", f, name)
			}
		}
	}
}
