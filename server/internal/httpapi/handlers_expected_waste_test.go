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

// Solo la merma del artículo acepta motivo (spec 099: el MCP lo manda); la general y la de producto
// lo siguen rechazando como campo desconocido, para que nadie crea que quedó guardado.
func TestDecodeArticleExpectedWaste(t *testing.T) {
	cases := []struct {
		name, body, wantPct, wantReason string
		ok                              bool
	}{
		{"sin motivo, como la pantalla", `{"pct": 8}`, "8", "", true},
		{"con motivo", `{"pct": 8, "reason": "rota poco"}`, "8", "rota poco", true},
		{"quitar la captura con motivo", `{"pct": null, "reason": "x"}`, "", "x", true},
		{"motivo que no es texto", `{"pct": 8, "reason": 5}`, "", "", false},
		{"sin pct, aunque traiga motivo", `{"reason": "x"}`, "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("PUT", "/", strings.NewReader(c.body))
			pct, reason, err := decodeArticleExpectedWaste(r)
			if c.ok != (err == nil) {
				t.Fatalf("err = %v; quiere ok=%v", err, c.ok)
			}
			if err != nil {
				return
			}
			if (c.wantPct == "") != (pct == nil) || (pct != nil && pct.String() != c.wantPct) || reason != c.wantReason {
				t.Fatalf("= %v, %q; quiere %q, %q", pct, reason, c.wantPct, c.wantReason)
			}
		})
	}
	r := httptest.NewRequest("PUT", "/", strings.NewReader(`{"pct": 5, "reason": "x"}`))
	if _, err := decodeExpectedWastePct(r); err == nil {
		t.Fatal("la merma general o de producto aceptó un motivo que no guarda")
	}
}

// El % de la vista previa va en la URL: `none` es «sin captura»; ausente o malformado es 400, nunca
// «sin captura» por omisión.
func TestParsePreviewPct(t *testing.T) {
	cases := []struct {
		name, query, want string
		ok                bool
	}{
		{"número", "pct=8", "8", true},
		{"con decimales", "pct=7.5", "7.5", true},
		{"cero", "pct=0", "0", true},
		{"sin captura", "pct=none", "", true},
		{"ausente", "", "", false},
		{"vacío", "pct=", "", false},
		{"texto", "pct=abc", "", false},
		{"null no es la palabra acordada", "pct=null", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/?"+c.query, nil)
			got, err := parsePreviewPct(r)
			if c.ok != (err == nil) {
				t.Fatalf("err = %v; quiere ok=%v", err, c.ok)
			}
			if err != nil {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("%v no es ErrValidation", err)
				}
				return
			}
			if (c.want == "") != (got == nil) || (got != nil && got.String() != c.want) {
				t.Fatalf("= %v; quiere %q", got, c.want)
			}
		})
	}
}
