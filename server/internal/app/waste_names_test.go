package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Nada de lo que se echó a perder depende de un negocio en particular (spec 039, FR-022): los
// motivos sugeridos son de cualquier negocio de comida y la empresa los cambia. Revisa el código, las
// consultas, la migración y las pantallas de la feature, no las pruebas.
func TestWasteCodeNamesNoBusiness(t *testing.T) {
	files := []string{
		"waste.go", "waste_reports.go", "../domain/waste.go", "../httpapi/handlers_waste.go",
		"../../queries/waste.sql", "../../migrations/0094_waste.sql",
		"../../../web/src/features/waste/WastePage.tsx",
		"../../../web/src/features/waste/PendingWasteBar.tsx",
		"../../../web/src/features/backoffice/WasteTab.tsx",
		"../../../web/src/features/backoffice/CountLossReport.tsx",
		"../../../web/src/features/admin/WasteSettings.tsx",
		"../../../web/src/api/waste.ts",
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
