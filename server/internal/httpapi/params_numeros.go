package httpapi

import (
	"fmt"
	"math"
	"net/url"
	"strconv"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Los números de la frontera, con la misma regla que las fechas: el default es para el parámetro
// AUSENTE, nunca para el presente y malformado.
//
// Se escribió porque los enteros se habían quedado fuera de esa regla mientras las fechas ya la
// cumplían. `?limit=abc` contestaba las 50 filas del default, `?limit=0` también, y `?limit=3e9`
// truncaba a un int32 NEGATIVO que Postgres rechaza — o sea un 500 por una petición que nunca fue
// válida, cuando la constitución pide 400.

// MaxListLimit acota cualquier lista sin paginar de la frontera. Serializar cien mil filas no es
// una consulta lenta, es un proceso muerto en el gigabyte de RAM del VPS.
const MaxListLimit = 200

// enteroDeQuery lee un entero acotado. Devuelve (valor, vino, error): quien llama decide el default
// del caso ausente, y aquí no se adivina ninguno.
func enteroDeQuery(q url.Values, nombre string, min, max int64) (int64, bool, error) {
	if !q.Has(nombre) {
		return 0, false, nil
	}
	v := q.Get(nombre)
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, true, fmt.Errorf("%w: %s no es un número entero (%q)", domain.ErrValidation, nombre, v)
	}
	if n < min || n > max {
		return 0, true, fmt.Errorf("%w: %s va de %d a %d, no %d", domain.ErrValidation, nombre, min, max, n)
	}
	return n, true, nil
}

// limiteDeQuery lee `limit` con su tope. El default aplica solo si no vino.
func limiteDeQuery(q url.Values, def int32) (int32, error) {
	n, vino, err := enteroDeQuery(q, "limit", 1, MaxListLimit)
	if err != nil {
		return 0, err
	}
	if !vino {
		return def, nil
	}
	return int32(n), nil
}

// paginaDeQuery traduce `page`/`pageSize` a limit y offset.
//
// El tope de página se DERIVA del tamaño en vez de escribirse a mano, para que el offset quepa
// siempre en el int32 de la consulta. Se calculaba como `int32(n) * f.Limit` antes de validar, y
// con `page=214748365` sobre páginas de 20 el producto envuelve a 4: el servidor contestaba la
// quinta página con un 200 limpio y nada en la respuesta lo delataba.
func paginaDeQuery(q url.Values) (limit, offset int32, err error) {
	tam, vino, err := enteroDeQuery(q, "pageSize", 1, int64(domain.MaxSalesPageSize))
	if err != nil {
		return 0, 0, err
	}
	limit = 20
	if vino {
		limit = int32(tam)
	}

	maxPagina := int64(math.MaxInt32) / int64(limit)
	pag, _, err := enteroDeQuery(q, "page", 0, maxPagina)
	if err != nil {
		return 0, 0, err
	}
	return limit, int32(pag * int64(limit)), nil
}
