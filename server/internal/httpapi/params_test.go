package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// UNA BANDERA MAL ESCRITA SE RECHAZA; NUNCA CAE AL DEFAULT EN SILENCIO.
//
// `?porCobrar=si` leído como `false` devuelve la lista COMPLETA con cara de ser la filtrada: el
// operador ve treinta renglones donde pidió dieciséis y no tiene forma de saber que su parámetro se
// perdió. Es el principio V — el default es para el parámetro AUSENTE, no para el presente y
// malformado.
func TestUnaBanderaMalEscritaNoSeLeeComoFalse(t *testing.T) {
	casos := []struct {
		query  string
		def    bool
		quiere bool
		malo   bool
	}{
		{"", false, false, false},
		{"", true, true, false},
		{"porCobrar=true", false, true, false},
		{"porCobrar=1", false, true, false},
		{"porCobrar=false", true, false, false},
		{"porCobrar=0", true, false, false},
		// Las tres formas en que alguien la escribe a mano y que ANTES habrían pasado por "false".
		{"porCobrar=si", false, false, true},
		{"porCobrar=TRUE", false, false, true},
		{"porCobrar=", false, false, true},
	}
	for _, c := range casos {
		t.Run(c.query, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/orders/open?"+c.query, nil)
			got, err := queryBool(r, "porCobrar", c.def)
			if c.malo {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("err = %v, quiere ErrValidation: un valor que no se entiende no puede pasar por default", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != c.quiere {
				t.Errorf("got = %v, quiere %v", got, c.quiere)
			}
		})
	}
}
