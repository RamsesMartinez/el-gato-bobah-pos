package mailer

import (
	"encoding/base64"
	"strings"
	"testing"
)

// RFC 5322 limita cada línea a 998 octetos; un aviso de error con un mensaje largo la rebasaba y un
// servidor estricto rechaza el correo entero (auditoría de la spec 070).
func TestBuildMessageKeepsLinesShortAndBodyIntact(t *testing.T) {
	body := "<table>" + strings.Repeat("ñandú ", 2000) + "</table>"
	msg := string(buildMessage("a@b.c", "d@e.f", "Asunto", body))
	for i, line := range strings.Split(msg, "\r\n") {
		if len(line) > 998 {
			t.Fatalf("la línea %d mide %d octetos", i, len(line))
		}
	}
	head, encoded, ok := strings.Cut(msg, "\r\n\r\n")
	if !ok || !strings.Contains(head, "Content-Transfer-Encoding: base64") {
		t.Fatalf("encabezados: %q", head)
	}
	got, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(encoded, "\r\n", ""))
	if err != nil || string(got) != body {
		t.Fatalf("el cuerpo no se recupera igual (err %v)", err)
	}
}
