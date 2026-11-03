//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// CADA ACCIÓN DE LA BITÁCORA TIENE UNA PRUEBA QUE LA PRODUCE Y LA LEE.
//
// La lista blanca crece sola cuando alguien agrega una acción; la instrumentación no. Una acción en
// la lista que ningún camino escribe sale como filtro en pantalla y nunca trae nada, y nadie lo
// nota. Esta prueba busca, en las pruebas de integración, una consulta a `audit_events` que nombre
// cada acción, y falla diciendo cuál falta.
func TestEveryAuditActionIsProduced(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	var corpus strings.Builder
	for _, f := range files {
		if f == "audit_coverage_test.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "audit_events") || strings.Contains(string(b), "orderEvents") || strings.Contains(string(b), "actionsBy") {
			corpus.Write(b)
		}
	}
	text := corpus.String()
	var missing []string
	for _, a := range domain.AllAuditActions() {
		if !strings.Contains(text, `"`+string(a)+`"`) && !strings.Contains(text, `'`+string(a)+`'`) && !strings.Contains(text, " "+string(a)+" ") && !strings.Contains(text, " "+string(a)+`"`) && !strings.Contains(text, `"`+string(a)+" ") {
			missing = append(missing, string(a))
		}
	}
	if len(missing) > 0 {
		t.Fatalf("acciones sin una prueba que las produzca: %s", strings.Join(missing, ", "))
	}
}
