package httpapi

import (
	"net/http"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Headers con los que el POS dice desde qué aparato trabaja (spec 033). La clave la genera el
// navegador la primera vez; la descripción es un JSON corto. Ninguno autentica nada.
const (
	DeviceKeyHeader  = "X-Device-Key"
	DeviceInfoHeader = "X-Device-Info"
)

// withRequestOrigin pone en el contexto de dónde vino el request, para que la bitácora lo guarde.
// La IP sale de rateKeyIP y no de clientIP: detrás del proxy toma el último X-Forwarded-For (el que
// el cliente no puede falsear); clientIP confía en el header siempre y solo sirve para logs.
func withRequestOrigin(behindProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			o := domain.NormalizeOrigin(rateKeyIP(r, behindProxy),
				r.Header.Get(DeviceKeyHeader), r.Header.Get(DeviceInfoHeader))
			next.ServeHTTP(w, r.WithContext(app.WithOrigin(r.Context(), o)))
		})
	}
}
