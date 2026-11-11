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
	return int32DeFrontera("limit", n)
}

// paginaDeQuery traduce `page`/`pageSize` a limit y offset; def es el tamaño cuando `pageSize` no vino.
//
// El tope de página se DERIVA del tamaño en vez de escribirse a mano, para que el offset quepa
// siempre en el int32 de la consulta. Se calculaba como `int32(n) * f.Limit` antes de validar, y
// con `page=214748365` sobre páginas de 20 el producto envuelve a 4: el servidor contestaba la
// quinta página con un 200 limpio y nada en la respuesta lo delataba.
func paginaDeQuery(q url.Values, def int32) (limit, offset int32, err error) {
	tam, vino, err := enteroDeQuery(q, "pageSize", 1, int64(domain.MaxSalesPageSize))
	if err != nil {
		return 0, 0, err
	}
	limit = def
	if vino {
		if limit, err = int32DeFrontera("pageSize", tam); err != nil {
			return 0, 0, err
		}
	}

	maxPagina := int64(math.MaxInt32) / int64(limit)
	pag, _, err := enteroDeQuery(q, "page", 0, maxPagina)
	if err != nil {
		return 0, 0, err
	}
	offset, err = int32DeFrontera("page", pag*int64(limit))
	return limit, offset, err
}

// MaxAdminListLimit acota las listas del catálogo, donde `limit=0` pide la lista completa (el POS
// carga así sus productos y opciones).
const MaxAdminListLimit = 100

// limiteYOffsetDeQuery lee `limit` y `offset` de las listas del catálogo, que paginan con offset en
// vez de con página.
//
// El offset se leía sin tope y se convertía a int32: con offset=4294967296 envolvía a 0 y la API
// contestaba la primera página con un 200. Un negativo o un texto se aplanaban a 0 y un limit de
// 500 se recortaba a 100, las dos cosas en silencio; ahora son 400, porque el front siempre manda
// números dentro del rango y lo demás no lo pidió nadie.
func limiteYOffsetDeQuery(q url.Values, def int32) (limit, offset int32, err error) {
	lim, vino, err := enteroDeQuery(q, "limit", 0, MaxAdminListLimit)
	if err != nil {
		return 0, 0, err
	}
	limit = def
	if vino {
		if limit, err = int32DeFrontera("limit", lim); err != nil {
			return 0, 0, err
		}
	}
	off, _, err := enteroDeQuery(q, "offset", 0, math.MaxInt32)
	if err != nil {
		return 0, 0, err
	}
	offset, err = int32DeFrontera("offset", off)
	return limit, offset, err
}

// int32DeFrontera es el único lugar de la frontera que convierte a int32. Repite la cota a
// propósito: si alguien le pasa a enteroDeQuery un tope mayor que int32, aquí sale un 400 en vez de
// envolver en silencio, que es justo el defecto que estos helpers vinieron a cerrar.
func int32DeFrontera(nombre string, n int64) (int32, error) {
	if n < math.MinInt32 || n > math.MaxInt32 {
		return 0, fmt.Errorf("%w: %s fuera de rango (%d)", domain.ErrValidation, nombre, n)
	}
	return int32(n), nil
}
