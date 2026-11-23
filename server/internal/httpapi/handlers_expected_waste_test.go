package httpapi

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// El cuerpo de los tres PUT de merma (spec 096) siempre dice qué se quiere: `{"pct": null}` quita el
// %, `{"pct": 0}` lo captura en cero, y un cuerpo sin la llave es un error — nunca «sin captura» por
// omisión (constitución V: un parámetro presente y malformado no cae a un default).
func TestDecodeExpectedWastePct(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string // "" = nil
		ok   bool
	}{
		{"null quita el %", `{"pct": null}`, "", true},
		{"cero es capturado en cero", `{"pct": 0}`, "0", true},
		{"con decimales", `{"pct": 2.5}`, "2.5", true},
		{"sin la llave no es «sin captura»", `{}`, "", false},
		{"texto", `{"pct": "cinco"}`, "", false},
		{"campo desconocido", `{"pct": 5, "porcentaje": 5}`, "", false},
		{"cuerpo roto", `{"pct":`, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("PUT", "/", strings.NewReader(c.body))
			got, err := decodeExpectedWastePct(r)
			if c.ok != (err == nil) {
				t.Fatalf("err = %v; quiere ok=%v", err, c.ok)
			}
			if err != nil {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("el error %v no es ErrValidation (400)", err)
				}
				return
			}
			if (c.want == "") != (got == nil) || (got != nil && got.String() != c.want) {
				t.Fatalf("pct = %v; quiere %q", got, c.want)
			}
		})
	}
}
