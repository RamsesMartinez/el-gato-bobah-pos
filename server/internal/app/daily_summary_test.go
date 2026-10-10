package app

import (
	"strings"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// El concepto lo teclea quien cobra y llega al correo del dueño: tiene que salir escapado, o un
// concepto «<a href=...>Pagar</a>» se vuelve un enlace dentro del resumen.
func TestSummaryHTMLEscapesTypedConcepts(t *testing.T) {
	body := summaryHTML(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC), db.DaySummaryMoneyRow{},
		[]db.DaySummaryCashOutsRow{{Concept: `<a href="//x">Pagar</a><script>`, Total: mustDec("10")}})
	if strings.Contains(body, "<a href") || strings.Contains(body, "<script>") {
		t.Fatalf("el concepto llegó sin escapar al correo:\n%s", body)
	}
}
