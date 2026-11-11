package httpapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Roles por empresa (spec 068): ningún control pregunta por el nombre de un rol. Un RequireRole
// nuevo ignora los roles propios de cada empresa y lo que cada una cambió en los sugeridos.
func TestNoRouteIsGatedByRoleName(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "RequireRole(") {
			t.Errorf("%s pregunta por el nombre de un rol (RequireRole): usa RequirePermission", f)
		}
	}
}
