package domain

import (
	"crypto/sha1" // #nosec G505 -- solo acorta una llave de Redis; no protege nada
	"encoding/hex"
	"fmt"
	"html"
	"path"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// ServerAlert es lo que se sabe de una respuesta 500 al terminar la petición (spec 070). Nada de
// esto sale del cuerpo, los encabezados ni la query: el correo va a un buzón externo.
type ServerAlert struct {
	At          time.Time
	Environment string // host público de la app; ver config.AlertEnvironment
	Method      string
	Route       string // patrón de chi, nunca la URL real: la de los avisos en vivo trae ?token=
	RequestID   string
	UserID      int64
	UserName    string
	CompanyID   int64
	CompanyName string // lo llena el notifier; vacío si no se pudo leer
	Kind        string
	Message     string // ya pasó por SanitizeAlertText
	Panic       bool
	Frames      []string
}

// AlertSummary es lo que un correo agrega al aviso. Repeats > 0 lo vuelve el resumen de una ventana.
type AlertSummary struct {
	Repeats  int64 // repeticiones dentro de la ventana que cierra
	Orphaned int64 // repeticiones de una ventana cuyo proceso murió sin reportarlas
	Dropped  int64 // avisos que no cupieron en la cola desde el último correo
}

// AlertWindow es cada cuánto se avisa del mismo error: pidió el dueño uno cada 15 minutos.
const AlertWindow = 15 * time.Minute

const (
	maxAlertText   = 1000
	maxAlertFrames = 12
	projectModule  = "github.com/ramthedev/el-gato-bobah-pos/server/internal/"
)

// AlertKey identifies "the same error": same method, route pattern and error kind. El texto no
// entra: trae ids y montos, y agrupar por él mandaría un correo por pedido.
func AlertKey(a ServerAlert) string {
	sum := sha1.Sum([]byte(a.Method + " " + a.Route + " " + a.Kind)) // #nosec G401 -- ver import
	return hex.EncodeToString(sum[:])
}

var alertSecretPatterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	// Primero el JWT completo: si el par clave-valor lo corta antes, quedarían dos tercios visibles.
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`), "[jwt]"},
	{regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`), "Bearer ***"},
	// La clave puede llevar prefijo (client_secret, X-Api-Key); el separador antes evita que «pin»
	// salte dentro de otra palabra.
	{regexp.MustCompile(`(?i)((?:^|[^A-Za-z0-9])["']?(?:[a-z]+[_-])?(?:password|passwd|pin|token|access_?token|refresh_?token|secret|authorization|api[_-]?key|card|tarjeta|cvv|cookie|otp)["']?\s*[:=]\s*)("[^"]*"|[^\s,;&}]+)`), "${1}***"},
	// Postgres repite en el error el valor que mandó el cliente (22P02, 22007): puede ser lo que se
	// tecleó en cualquier campo.
	{regexp.MustCompile(`(?i)(invalid input (?:syntax|value) for [^:"]+: )"[^"]*"`), `${1}"***"`},
	// 13 a 19 dígitos, con o sin separadores: el largo de un número de tarjeta. Un id de este
	// sistema no llega a 13 dígitos.
	{regexp.MustCompile(`\b(?:\d[ -]?){12,18}\d\b`), "[número]"},
}

// opaqueRun es la forma de un token sin nombre (refresh, llave de API): 24 o más caracteres de
// base64url. Solo se enmascara si mezcla letras y dígitos, para no esconder el nombre de una
// restricción como order_line_modifiers_order_line_id_fkey.
var opaqueRun = regexp.MustCompile(`[A-Za-z0-9_-]{24,}`)

func maskOpaque(s string) string {
	return opaqueRun.ReplaceAllStringFunc(s, func(run string) string {
		if strings.ContainsAny(run, "0123456789") && strings.IndexFunc(run, func(r rune) bool {
			return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		}) >= 0 {
			return "[token]"
		}
		return run
	})
}

// SanitizeAlertText masks what must never reach a mailbox (tokens, passwords, card numbers) and
// truncates. Se aplica al mensaje del error y al valor de un panic, que pueden traer cualquier cosa.
func SanitizeAlertText(s string) string {
	for _, p := range alertSecretPatterns {
		s = p.re.ReplaceAllString(s, p.repl)
	}
	s = maskOpaque(s)
	if r := []rune(s); len(r) > maxAlertText {
		s = string(r[:maxAlertText]) + "…"
	}
	return s
}

// alertFrameSkip son los marcos que solo dicen «aquí se registró el error», no dónde ocurrió.
var alertFrameSkip = []string{
	"httpapi.Error",
	"httpapi.reportServerErrors",
	"httpapi.(*serverErrorWriter)",
	"httpapi.recordServerError",
}

// ProjectFrames keeps up to 12 frames of this project's code, innermost first, as
// "pkg.Func file:line". Lo de Go, chi y net/http no dice nada que no se sepa.
func ProjectFrames(frames []runtime.Frame) []string {
	var out []string
	for _, f := range frames {
		fn, ok := strings.CutPrefix(f.Function, projectModule)
		if !ok || skipAlertFrame(fn) {
			continue
		}
		out = append(out, fmt.Sprintf("%s %s:%d", fn, path.Base(f.File), f.Line))
		if len(out) == maxAlertFrames {
			break
		}
	}
	return out
}

func skipAlertFrame(fn string) bool {
	for _, s := range alertFrameSkip {
		if strings.HasPrefix(fn, s) {
			return true
		}
	}
	return false
}

// AlertEmail renders the subject and HTML body of an alert. Todo valor va escapado y el asunto sin
// saltos de línea: la ruta y el mensaje no los controla quien programa.
func AlertEmail(a ServerAlert, s AlertSummary) (subject, body string) {
	what := "Error 500"
	if a.Panic {
		what = "Panic"
	}
	if s.Repeats > 0 {
		what += fmt.Sprintf(" repetido %d veces", s.Repeats)
	}
	subject = oneLine(fmt.Sprintf("[POS %s] %s: %s %s", a.Environment, what, a.Method, a.Route))

	loc := LoadBusinessLocation(DefaultTimezone)
	rows := [][2]string{
		{"Hora (CDMX)", a.At.In(loc).Format("2006-01-02 15:04:05 MST")},
		{"Petición", a.Method + " " + a.Route},
		{"Usuario", who(a.UserID, a.UserName)},
		{"Empresa", who(a.CompanyID, a.CompanyName)},
		{"Id de request", a.RequestID},
		{"Tipo", a.Kind},
		{"Mensaje", a.Message},
	}
	if s.Repeats > 0 {
		r := fmt.Sprintf("%d en los últimos %d min", s.Repeats, int(AlertWindow.Minutes()))
		rows = append(rows, [2]string{"Repeticiones", r})
	}
	if s.Orphaned > 0 {
		rows = append(rows, [2]string{"Sin reportar", fmt.Sprintf("%d anteriores sin reportar", s.Orphaned)})
	}
	if s.Dropped > 0 {
		rows = append(rows, [2]string{"Descartados", fmt.Sprintf("%d avisos descartados por ráfaga desde el último correo", s.Dropped)})
	}

	var b strings.Builder
	b.WriteString(`<table cellpadding="4" style="font-family:sans-serif;font-size:14px;border-collapse:collapse">`)
	for _, r := range rows {
		fmt.Fprintf(&b, `<tr><th align="left" valign="top">%s</th><td>%s</td></tr>`, html.EscapeString(r[0]), html.EscapeString(r[1]))
	}
	b.WriteString(`</table><h4 style="font-family:sans-serif">Traza</h4><pre>`)
	if len(a.Frames) == 0 {
		b.WriteString("(sin marcos del proyecto)")
	}
	for _, f := range a.Frames {
		b.WriteString(html.EscapeString(f))
		b.WriteByte('\n')
	}
	b.WriteString("</pre>")
	return subject, b.String()
}

func who(id int64, name string) string {
	switch {
	case id == 0:
		return "sin sesión"
	case name == "":
		return fmt.Sprintf("#%d", id)
	default:
		return fmt.Sprintf("#%d %s", id, name)
	}
}

func oneLine(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}
