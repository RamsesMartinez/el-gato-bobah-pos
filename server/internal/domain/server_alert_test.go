package domain

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

const sampleJWT = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMiJ9.c2lnbmF0dXJh"

func TestSanitizeAlertTextMasksSecrets(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		absent []string
	}{
		{"jwt suelto", "token inválido " + sampleJWT + " al parsear", []string{sampleJWT, "eyJzdWIiOiIxMiJ9"}},
		{"bearer", "header Authorization: Bearer abc.def-ghi rechazado", []string{"abc.def-ghi"}},
		{"password=", "login falló password=hunter2 user=ana", []string{"hunter2"}},
		{"json token", `cuerpo {"token":"s3cr3t-value","x":1}`, []string{"s3cr3t-value"}},
		{"pin con dos puntos", "pin: 4821 no coincide", []string{"4821"}},
		{"api_key", "uber api_key=AKIA123456 vencida", []string{"AKIA123456"}},
		{"tarjeta corrida", "cobro con 4111111111111111 rechazado", []string{"4111111111111111"}},
		{"tarjeta con espacios", "cobro con 4111 1111 1111 1111 rechazado", []string{"4111 1111 1111 1111"}},
		{"tarjeta con guiones", "cobro con 4111-1111-1111-1111 rechazado", []string{"4111-1111-1111-1111"}},
		{"secret en mayúsculas", "SECRET=abcdef123 filtrado", []string{"abcdef123"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SanitizeAlertText(c.in)
			for _, a := range c.absent {
				if strings.Contains(got, a) {
					t.Fatalf("el texto enmascarado todavía trae %q: %q", a, got)
				}
			}
		})
	}
}

// Enmascarar de más esconde justo lo que se necesita para encontrar el error.
func TestSanitizeAlertTextKeepsOrdinaryText(t *testing.T) {
	in := "no se pudo cobrar el pedido 1234 (id 77): ERROR: duplicate key value violates unique constraint \"orders_pkey\" (SQLSTATE 23505)"
	if got := SanitizeAlertText(in); got != in {
		t.Fatalf("cambió un texto sin secretos:\n got %q\nwant %q", got, in)
	}
}

func TestSanitizeAlertTextTruncates(t *testing.T) {
	got := SanitizeAlertText(strings.Repeat("á", 5000))
	if n := len([]rune(got)); n > maxAlertText+1 {
		t.Fatalf("el mensaje mide %d caracteres; el tope es %d", n, maxAlertText)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatal("un mensaje recortado no avisa que se recortó")
	}
}

func frame(fn, file string, line int) runtime.Frame {
	return runtime.Frame{Function: fn, File: file, Line: line}
}

func TestProjectFramesKeepsOnlyProjectCode(t *testing.T) {
	const mod = "github.com/ramthedev/el-gato-bobah-pos/server/internal/"
	in := []runtime.Frame{
		frame("runtime.gopanic", "/usr/local/go/src/runtime/panic.go", 770),
		frame(mod+"httpapi.Error", "/src/internal/httpapi/respond.go", 40),
		frame(mod+"app.(*OrdersService).Pay", "/src/internal/app/orders.go", 210),
		frame(mod+"httpapi.(*Handlers).PayOrder", "/src/internal/httpapi/handlers_orders.go", 88),
		frame("net/http.HandlerFunc.ServeHTTP", "/usr/local/go/src/net/http/server.go", 2220),
		frame("github.com/go-chi/chi/v5.(*Mux).routeHTTP", "/go/pkg/mod/chi/mux.go", 459),
	}
	got := ProjectFrames(in)
	want := []string{
		"app.(*OrdersService).Pay orders.go:210",
		"httpapi.(*Handlers).PayOrder handlers_orders.go:88",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("marcos:\n got %q\nwant %q", got, want)
	}
}

func TestProjectFramesCapsAtTwelve(t *testing.T) {
	var in []runtime.Frame
	for i := range 30 {
		in = append(in, frame("github.com/ramthedev/el-gato-bobah-pos/server/internal/app.f", "/src/a.go", i))
	}
	if got := ProjectFrames(in); len(got) != maxAlertFrames {
		t.Fatalf("salieron %d marcos; el tope es %d", len(got), maxAlertFrames)
	}
}

func TestAlertKeySeparatesRouteMethodAndKind(t *testing.T) {
	base := ServerAlert{Method: "POST", Route: "/api/v1/orders/{id}/pay", Kind: "pg:23505"}
	same := base
	same.Message, same.RequestID, same.UserID = "otro texto", "r2", 9
	if AlertKey(base) != AlertKey(same) {
		t.Fatal("el mismo error con otro mensaje o usuario cayó en otra ventana")
	}
	for _, other := range []ServerAlert{
		{Method: "GET", Route: base.Route, Kind: base.Kind},
		{Method: base.Method, Route: "/api/v1/orders", Kind: base.Kind},
		{Method: base.Method, Route: base.Route, Kind: "*errors.errorString"},
	} {
		if AlertKey(other) == AlertKey(base) {
			t.Fatalf("%+v cayó en la misma ventana que %+v", other, base)
		}
	}
}

func sampleAlert() ServerAlert {
	return ServerAlert{
		At:          time.Date(2026, 11, 8, 20, 3, 12, 0, time.UTC),
		Environment: "app.elgatobobah.com",
		Method:      "POST", Route: "/api/v1/orders/{id}/pay", RequestID: "req-123",
		UserID: 12, UserName: "Ana", CompanyID: 2, CompanyName: "El Gato Bobah",
		Kind: "pg:23505", Message: "duplicate key",
		Frames: []string{"app.(*OrdersService).Pay orders.go:210"},
	}
}

func TestAlertEmailImmediate(t *testing.T) {
	subject, body := AlertEmail(sampleAlert(), AlertSummary{})
	if subject != "[POS app.elgatobobah.com] Error 500: POST /api/v1/orders/{id}/pay" {
		t.Fatalf("asunto: %q", subject)
	}
	for _, want := range []string{
		"2026-11-08 14:03:12 CST", // 20:03 UTC = 14:03 en CDMX (sin horario de verano)
		"POST /api/v1/orders/{id}/pay", "#12 Ana", "#2 El Gato Bobah", "req-123", "pg:23505",
		"duplicate key", "app.(*OrdersService).Pay orders.go:210",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("el cuerpo no trae %q", want)
		}
	}
	for _, absent := range []string{"Repeticiones", "descartados"} {
		if strings.Contains(body, absent) {
			t.Errorf("el aviso inmediato sin repeticiones ni descartes trae %q", absent)
		}
	}
}

func TestAlertEmailPanicAndSummarySubjects(t *testing.T) {
	a := sampleAlert()
	a.Panic = true
	if s, _ := AlertEmail(a, AlertSummary{}); s != "[POS app.elgatobobah.com] Panic: POST /api/v1/orders/{id}/pay" {
		t.Fatalf("asunto de panic: %q", s)
	}
	s, body := AlertEmail(sampleAlert(), AlertSummary{Repeats: 99, Orphaned: 3, Dropped: 5})
	if s != "[POS app.elgatobobah.com] Error 500 repetido 99 veces: POST /api/v1/orders/{id}/pay" {
		t.Fatalf("asunto del resumen: %q", s)
	}
	for _, want := range []string{"99 en los últimos 15 min", "3 anteriores sin reportar", "5 avisos descartados"} {
		if !strings.Contains(body, want) {
			t.Errorf("el resumen no trae %q", want)
		}
	}
}

func TestAlertEmailWithoutSessionOrCompanyName(t *testing.T) {
	a := sampleAlert()
	a.UserID, a.UserName, a.CompanyName = 0, "", ""
	_, body := AlertEmail(a, AlertSummary{})
	if !strings.Contains(body, "sin sesión") {
		t.Error("una petición sin usuario no dice «sin sesión»")
	}
	if !strings.Contains(body, "#2<") {
		t.Error("una empresa sin nombre no se muestra solo con su id")
	}
	a.CompanyID = 0
	if _, body := AlertEmail(a, AlertSummary{}); strings.Count(body, "sin sesión") != 2 {
		t.Error("sin empresa, la empresa también dice «sin sesión»")
	}
}

func TestAlertEmailEscapesHTML(t *testing.T) {
	a := sampleAlert()
	a.Message = `<script>alert(1)</script>`
	a.Route = "/x/<b>"
	subject, body := AlertEmail(a, AlertSummary{})
	if strings.Contains(body, "<script>") || strings.Contains(body, "<b>") {
		t.Fatalf("el cuerpo trae HTML sin escapar: %s", body)
	}
	if strings.ContainsAny(subject, "\r\n") {
		t.Fatal("el asunto trae saltos de línea")
	}
}

// Un salto de línea en el asunto inyectaría encabezados en el correo.
func TestAlertEmailSubjectHasNoHeaderInjection(t *testing.T) {
	a := sampleAlert()
	a.Route = "/x\r\nBcc: otro@ejemplo.com"
	if s, _ := AlertEmail(a, AlertSummary{}); strings.ContainsAny(s, "\r\n") {
		t.Fatalf("asunto con salto de línea: %q", s)
	}
}
