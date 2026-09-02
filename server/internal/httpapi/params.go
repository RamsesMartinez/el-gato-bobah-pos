package httpapi

import (
	"fmt"
	"net/http"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// queryBool lee una bandera de la query string.
//
// El parámetro AUSENTE cae al default; el PRESENTE y mal escrito se rechaza. Un `?porCobrar=si`
// leído como `false` devuelve una lista distinta de la que se pidió, con cara de correcta y sin
// nada que la delate — peor que un error, porque nadie la audita. Por eso se distingue "no vino"
// de "vino vacío" con Has y no con el string vacío, que los confunde.
func queryBool(r *http.Request, nombre string, def bool) (bool, error) {
	q := r.URL.Query()
	if !q.Has(nombre) {
		return def, nil
	}
	switch v := q.Get(nombre); v {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("%w: %s solo acepta true o false, no %q", domain.ErrValidation, nombre, v)
	}
}
