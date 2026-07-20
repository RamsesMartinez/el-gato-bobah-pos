package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedactBody(t *testing.T) {
	// Secretos redactados incluso anidados y dentro de arreglos; lo no sensible permanece.
	in := []byte(`{"username":"kate","password":"hunter2","nested":{"token":"abc"},"list":[{"pin":"1234"}]}`)
	out := redactBody("application/json", in)
	for _, secret := range []string{"hunter2", "abc", "1234"} {
		if strings.Contains(out, secret) {
			t.Fatalf("secreto %q no redactado: %s", secret, out)
		}
	}
	if !strings.Contains(out, "kate") {
		t.Fatalf("dato no sensible debería permanecer: %s", out)
	}

	if got := redactBody("text/plain", []byte("hola")); got != "[4 bytes]" {
		t.Fatalf("no-JSON debe reportar bytes, got %q", got)
	}
	if got := redactBody("text/event-stream", []byte("data: x")); got != "[stream]" {
		t.Fatalf("SSE debe ser [stream], got %q", got)
	}
	if got := redactBody("application/json", nil); got != "" {
		t.Fatalf("cuerpo vacío → \"\", got %q", got)
	}
}

func TestTruncate(t *testing.T) {
	long := strings.Repeat("a", maxLogBody+100)
	got := truncate(long)
	if !strings.HasSuffix(got, "…(truncado)") {
		t.Fatalf("debe marcar el truncado")
	}
	if len(got) >= len(long) {
		t.Fatalf("truncado debe acortar (%d ≥ %d)", len(got), len(long))
	}
}

// A09: los cuerpos traen PII (customerName, notas). En operación normal (Info + 2xx) no
// deben registrarse; sí en debug o ante un 5xx (triage).
func TestRequestLogger_OmitsBodiesUnlessDebugOrError(t *testing.T) {
	logLine := func(level slog.Level, status int) map[string]any {
		var buf bytes.Buffer
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level})))
		defer slog.SetDefault(prev)

		h := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"customerName":"Ana"}`))
		}))
		req := httptest.NewRequest(http.MethodPost, "/orders",
			strings.NewReader(`{"customerName":"Ana","notes":"sin azúcar"}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(httptest.NewRecorder(), req)

		var m map[string]any
		for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
			if len(line) > 0 {
				_ = json.Unmarshal(line, &m)
			}
		}
		return m
	}

	if m := logLine(slog.LevelInfo, http.StatusOK); m["req_body"] != nil || m["resp_body"] != nil {
		t.Fatalf("Info/200 no debe volcar cuerpos (PII), got %v", m)
	}
	if m := logLine(slog.LevelInfo, http.StatusInternalServerError); m["req_body"] == nil {
		t.Fatal("Info/500 debe incluir req_body para triage")
	}
	if m := logLine(slog.LevelDebug, http.StatusOK); m["req_body"] == nil {
		t.Fatal("Debug/200 debe incluir cuerpos")
	}
}
