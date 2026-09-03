package httpapi

import (
	"errors"
	"math"
	"net/url"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// UN NÚMERO DE LA FRONTERA QUE NO SE PUEDE ATENDER SE RECHAZA, NO CAE AL DEFAULT.
//
// Es la misma regla que `parseDate` ya aplica a las fechas, y que a los números se les había
// olvidado: `?limit=abc` contestaba las 50 filas de siempre y `?limit=0` también. Una pantalla que
// se ve correcta y responde algo que nadie pidió es peor que un error, porque nadie la audita.
func TestUnEnteroDeLaFronteraNoCaeAlDefault(t *testing.T) {
	casos := []struct {
		nombre   string
		valor    string
		presente bool
		quiere   int64
		malo     bool
	}{
		{"ausente usa el default", "", false, 0, false},
		{"un valor normal pasa", "25", true, 25, false},
		{"el mínimo pasa", "1", true, 1, false},
		{"el máximo pasa", "100", true, 100, false},
		{"texto se rechaza", "abc", true, 0, true},
		{"vacío presente se rechaza", "", true, 0, true},
		{"cero se rechaza", "0", true, 0, true},
		{"negativo se rechaza", "-5", true, 0, true},
		{"por encima del tope se rechaza", "101", true, 0, true},
		// Lo que desborda int32 NO se puede truncar: 3,000,000,000 truncado a int32 es negativo, y
		// un LIMIT negativo es un error de Postgres, o sea un 500 por una petición que nunca fue
		// válida. "Rechaza entradas absurdas como 400, no 500".
		{"lo que desborda int32 se rechaza", "3000000000", true, 0, true},
		{"lo que desborda int64 se rechaza", "99999999999999999999999", true, 0, true},
		{"con espacios se rechaza", " 25 ", true, 0, true},
		{"decimal se rechaza", "25.5", true, 0, true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			q := url.Values{}
			if c.presente {
				q.Set("limit", c.valor)
			}
			v, hubo, err := enteroDeQuery(q, "limit", 1, 100)
			if c.malo {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("limit=%q: err = %v, quiere ErrValidation", c.valor, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("limit=%q: %v", c.valor, err)
			}
			if hubo != c.presente {
				t.Fatalf("limit=%q: presente = %v, quiere %v", c.valor, hubo, c.presente)
			}
			if hubo && v != c.quiere {
				t.Fatalf("limit=%q = %d, quiere %d", c.valor, v, c.quiere)
			}
		})
	}
}

// UNA PÁGINA ABSURDA NO PUEDE ENVOLVERSE Y CONTESTAR OTRA.
//
// `f.Offset = int32(n) * f.Limit` se calculaba ANTES de validar, con un `nolint` que decía "ambos ya
// acotados" y en ese punto no lo estaban. Con `page=214748365` y páginas de 20, el producto es
// 4,294,967,300: envuelve a 4 en int32 y el servidor contesta la QUINTA página con un 200 limpio.
// Nadie pidió esa página y nada en la respuesta lo delata.
func TestUnaPaginaQueDesbordaNoContestaOtra(t *testing.T) {
	malas := []url.Values{
		{"page": {"214748365"}},
		{"page": {"999999999999"}},
		{"page": {"-1"}},
		{"page": {"abc"}},
		{"pageSize": {"4294967297"}},
		{"pageSize": {"0"}},
		{"pageSize": {"101"}},
		{"pageSize": {"-20"}},
	}
	for _, q := range malas {
		t.Run(q.Encode(), func(t *testing.T) {
			if _, _, err := paginaDeQuery(q); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("%s: err = %v, quiere ErrValidation", q.Encode(), err)
			}
		})
	}
}

func TestLaPaginaNormalSeTraduceAOffset(t *testing.T) {
	casos := []struct {
		q             url.Values
		limit, offset int32
	}{
		{url.Values{}, 20, 0},
		{url.Values{"page": {"0"}}, 20, 0},
		{url.Values{"page": {"3"}}, 20, 60},
		{url.Values{"pageSize": {"50"}, "page": {"2"}}, 50, 100},
		{url.Values{"pageSize": {"1"}, "page": {"7"}}, 1, 7},
	}
	for _, c := range casos {
		t.Run(c.q.Encode(), func(t *testing.T) {
			limit, offset, err := paginaDeQuery(c.q)
			if err != nil {
				t.Fatalf("%s: %v", c.q.Encode(), err)
			}
			if limit != c.limit || offset != c.offset {
				t.Fatalf("%s = limit %d offset %d, quiere %d y %d",
					c.q.Encode(), limit, offset, c.limit, c.offset)
			}
		})
	}
}

// El offset tiene que caber en el int32 de la consulta. Con el tope de página derivado del tamaño
// —y no un número escrito a mano— la cota se ajusta sola si el tamaño de página cambia.
func TestElOffsetNuncaDesbordaInt32(t *testing.T) {
	q := url.Values{"pageSize": {"100"}, "page": {itoaDePrueba(math.MaxInt32/100 + 1)}}
	if _, _, err := paginaDeQuery(q); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("una página que desborda el offset debe rechazarse, fue %v", err)
	}
	// Y la última que sí cabe, pasa.
	q = url.Values{"pageSize": {"100"}, "page": {itoaDePrueba(math.MaxInt32 / 100)}}
	if _, _, err := paginaDeQuery(q); err != nil {
		t.Fatalf("la última página que cabe debe pasar, fue %v", err)
	}
}

func itoaDePrueba(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
