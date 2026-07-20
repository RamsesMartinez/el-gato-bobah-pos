package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const RequestIDHeader = "X-Request-Id"
const maxLogBody = 2048 // bytes que se registran del cuerpo (redactado); acota tokens

type reqIDKey struct{}

// traceInfo lo llena RequireAuth (downstream) y lo lee RequestLogger (al terminar),
// vía un puntero compartido en el contexto → sabemos QUIÉN hizo cada request.
type traceInfo struct {
	userID int64
	role   string
}
type traceKey struct{}

func traceFrom(ctx context.Context) *traceInfo {
	if t, ok := ctx.Value(traceKey{}).(*traceInfo); ok {
		return t
	}
	return nil
}

// RequestID toma el X-Request-Id del front o genera uno, lo pone en la respuesta y
// en el contexto para trazar el request de inicio a fin.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get(RequestIDHeader))
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set(RequestIDHeader, id)
		ctx := context.WithValue(r.Context(), reqIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(reqIDKey{}).(string); ok {
		return v
	}
	return ""
}

// RequestLogger registra cada request y su respuesta (método, ruta, estado, duración,
// request_id, y cuerpos redactados). Sirve en todos los ambientes.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		var reqBody []byte
		if r.Body != nil {
			reqBody, _ = io.ReadAll(io.LimitReader(r.Body, maxLogBody+1))
			r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(reqBody), r.Body))
		}

		// holder que RequireAuth rellenará con el usuario autenticado
		ti := &traceInfo{}
		r = r.WithContext(context.WithValue(r.Context(), traceKey{}, ti))

		cw := &captureWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(cw, r)

		attrs := []any{
			"request_id", requestIDFrom(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", cw.status,
			"dur_ms", time.Since(start).Milliseconds(),
			"ip", clientIP(r),
		}
		if ti.userID != 0 {
			attrs = append(attrs, "user_id", ti.userID, "role", ti.role)
		}
		// A09: los cuerpos traen PII (customerName, notas). En operación normal no se
		// registran; solo cuando el operador pidió verbosidad (debug) o hubo un 5xx que
		// hay que triagear. Los secretos siguen redactados por si aparecen aquí.
		if cw.status >= 500 || slog.Default().Enabled(r.Context(), slog.LevelDebug) {
			attrs = append(attrs,
				"req_body", redactBody(r.Header.Get("Content-Type"), reqBody),
				"resp_body", redactBody(cw.Header().Get("Content-Type"), cw.body.Bytes()),
			)
		}
		slog.Info("http", attrs...)
	})
}

func clientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		return strings.TrimSpace(strings.Split(xf, ",")[0])
	}
	return r.RemoteAddr
}

// captureWriter guarda el estado y hasta maxLogBody bytes del cuerpo de respuesta.
// Implementa Flusher para no romper SSE.
type captureWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
	wrote  bool
	stream bool
}

func (c *captureWriter) WriteHeader(code int) {
	c.status = code
	if strings.Contains(c.Header().Get("Content-Type"), "event-stream") {
		c.stream = true
	}
	c.wrote = true
	c.ResponseWriter.WriteHeader(code)
}

func (c *captureWriter) Write(b []byte) (int, error) {
	if !c.wrote {
		c.WriteHeader(http.StatusOK)
	}
	if !c.stream && c.body.Len() < maxLogBody {
		c.body.Write(b[:min(len(b), maxLogBody-c.body.Len())])
	}
	return c.ResponseWriter.Write(b)
}

func (c *captureWriter) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// --- redacción de datos sensibles ---

var sensitiveKeys = map[string]bool{
	"password": true, "pin": true, "token": true, "accesstoken": true,
	"refreshtoken": true, "secret": true, "authorization": true, "jwt": true,
	"pin_hash": true, "password_hash": true,
}

func redactBody(contentType string, body []byte) string {
	if len(body) == 0 {
		return ""
	}
	if strings.Contains(contentType, "event-stream") {
		return "[stream]"
	}
	if strings.Contains(contentType, "application/json") {
		var v any
		if json.Unmarshal(body, &v) == nil {
			redact(v)
			if b, err := json.Marshal(v); err == nil {
				return truncate(string(b))
			}
		}
	}
	// cuerpos no-JSON: no se registra el contenido crudo (puede traer binario/secretos)
	return fmt.Sprintf("[%d bytes]", len(body))
}

func redact(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if sensitiveKeys[strings.ToLower(k)] {
				t[k] = "***"
				continue
			}
			redact(val)
		}
	case []any:
		for _, item := range t {
			redact(item)
		}
	}
}

func truncate(s string) string {
	if len(s) > maxLogBody {
		return s[:maxLogBody] + "…(truncado)"
	}
	return s
}
