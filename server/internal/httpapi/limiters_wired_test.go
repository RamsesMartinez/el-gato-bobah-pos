package httpapi

import (
	"reflect"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
)

// TestEveryRateLimiterIsBuilt vigila que todo limitador declarado en Handlers salga construido de
// NewHandlers. Uno que solo se arma en las pruebas pasa la suite y en la API real es nil: el primer
// request lo desreferencia y responde 500. Pasó con «Enviarme por correo» de la lista de compra
// (spec 084), que solo lo encontró el e2e contra la API de verdad.
func TestEveryRateLimiterIsBuilt(t *testing.T) {
	h := reflect.ValueOf(NewHandlers(Deps{Cfg: config.Config{}})).Elem()
	limiter := reflect.TypeFor[*rateLimiter]()
	for i := range h.NumField() {
		f := h.Type().Field(i)
		if f.Type == limiter && h.Field(i).IsNil() {
			t.Errorf("Handlers.%s no se construye en NewHandlers: su endpoint responde 500", f.Name)
		}
	}
}
