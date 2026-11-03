package app

import "testing"

// Un texto que una hoja de cálculo leería como fórmula se protege. Además de = + - @, OWASP lista el
// tabulador y el retorno de carro al inicio: las hojas recortan ese blanco al importar y evalúan lo
// que sigue. Lo encontró la revisión de seguridad del 037.
func TestSafeCell(t *testing.T) {
	cases := map[string]string{
		"Leche":                 "Leche",
		"":                      "",
		"=HYPERLINK(\"x\")":     "'=HYPERLINK(\"x\")",
		"+1":                    "'+1",
		"-200":                  "'-200",
		"@SUM(A1)":              "'@SUM(A1)",
		"\t=HYPERLINK(\"x\")":   "'\t=HYPERLINK(\"x\")",
		"\r=1+1":                "'\r=1+1",
		"Café con leche = rico": "Café con leche = rico",
	}
	for in, want := range cases {
		if got := safeCell(in); got != want {
			t.Errorf("safeCell(%q) = %q, quiere %q", in, got, want)
		}
	}
}
