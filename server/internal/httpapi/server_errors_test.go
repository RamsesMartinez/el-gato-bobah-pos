package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/alerts"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

type alertMailbox struct {
	mu    sync.Mutex
	mails []string // asunto + cuerpo
}

func (b *alertMailbox) send(_, subject, body string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.mails = append(b.mails, subject+"\n"+body)
	return nil
}

func (b *alertMailbox) all() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.mails...)
}

func (b *alertMailbox) waitOne(t *testing.T) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for len(b.all()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no llegó el correo de aviso")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return b.all()[0]
}

func (b *alertMailbox) expectNone(t *testing.T) {
	t.Helper()
	time.Sleep(100 * time.Millisecond)
	if got := b.all(); len(got) != 0 {
		t.Fatalf("salió un aviso que no debía:\n%s", got[0])
	}
}

const alertTestSecret = "secreto-de-prueba-de-al-menos-32-caracteres!!"

// alertTestRouter monta la MISMA pila de middlewares que el Router (baseMiddlewares) con rutas que
// fuerzan cada caso. El Router real no tiene una ruta que truene a propósito.
func alertTestRouter(t *testing.T) (http.Handler, *alertMailbox, *auth.Manager) {
	t.Helper()
	b := &alertMailbox{}
	n := alerts.New(alerts.Options{To: "dueno@example.com", Send: b.send, Window: time.Hour})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = n.Close(ctx)
	})
	jm := auth.NewManager(alertTestSecret, nil)
	r := chi.NewRouter()
	baseMiddlewares(r, config.Config{Env: "development", AppBaseURL: "https://app.elgatobobah.com"}, n)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/things/{id}/fail", func(w http.ResponseWriter, _ *http.Request) {
			Error(w, fmt.Errorf("guardar cosa: %w", errors.New("se rompió")))
		})
		r.Get("/panic", func(http.ResponseWriter, *http.Request) {
			panic("explotó con " + sampleAlertJWT)
		})
		r.Get("/abort", func(http.ResponseWriter, *http.Request) {
			panic(http.ErrAbortHandler)
		})
		r.Get("/status/{code}", func(w http.ResponseWriter, req *http.Request) {
			errs := map[string]error{
				"400": domain.ErrValidation, "404": domain.ErrNotFound, "409": domain.ErrConflict,
				"501": app.ErrDocExtractDisabled, "503": domain.ErrShuttingDown,
			}
			Error(w, errs[chi.URLParam(req, "code")])
		})
		r.Group(func(r chi.Router) {
			r.Use(RequireAuth(jm))
			r.Post("/orders/{id}/pay", func(w http.ResponseWriter, req *http.Request) {
				_, _ = io.ReadAll(req.Body)
				Error(w, fmt.Errorf("cobrar con %s password=hunter2: %w", sampleAlertJWT, errors.New("terminal")))
			})
		})
	})
	return r, b, jm
}

const sampleAlertJWT = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiI5OSJ9.ZmlybWFkZWxhdG9rZW4"

func TestServerErrorMailsTheAlert(t *testing.T) {
	h, b, _ := alertTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/things/42/fail", nil)
	req.Header.Set(RequestIDHeader, "req-abc")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("estado %d", rec.Code)
	}
	m := b.waitOne(t)
	for _, want := range []string{
		"[POS app.elgatobobah.com] Error 500: GET /api/v1/things/{id}/fail",
		"req-abc", "*errors.errorString", "guardar cosa: se rompió", "sin sesión",
		"server_errors_test.go", // la traza llega hasta el handler que llamó Error
	} {
		if !strings.Contains(m, want) {
			t.Errorf("el aviso no trae %q:\n%s", want, m)
		}
	}
	if strings.Contains(m, "/things/42/") {
		t.Error("el aviso trae la URL real en vez del patrón de la ruta")
	}
}

func captureHTTPLogs(t *testing.T) func() string {
	t.Helper()
	var buf bytes.Buffer
	var mu sync.Mutex
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return buf.Write(p)
	}), nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return func() string { mu.Lock(); defer mu.Unlock(); return buf.String() }
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func TestPanicAnswersInternalAndMailsWithoutTheSecret(t *testing.T) {
	logs := captureHTTPLogs(t)
	h, b, _ := alertTestRouter(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/panic", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("un panic respondió %d", rec.Code)
	}
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Error.Code != "INTERNAL" {
		t.Fatalf("un panic no respondió el mismo cuerpo INTERNAL que un error: %q", rec.Body.String())
	}
	m := b.waitOne(t)
	if !strings.Contains(m, "Panic: GET /api/v1/panic") || !strings.Contains(m, "panic:string") {
		t.Fatalf("el aviso no se marcó como panic:\n%s", m)
	}
	if !strings.Contains(m, "server_errors_test.go") {
		t.Errorf("la traza del panic no llega al handler:\n%s", m)
	}
	if strings.Contains(m, sampleAlertJWT) {
		t.Fatal("el aviso del panic trae el JWT")
	}
	if !strings.Contains(logs(), "panic") {
		t.Fatal("el panic no quedó en la bitácora")
	}
	if strings.Contains(logs(), sampleAlertJWT) {
		t.Fatal("la bitácora del panic trae el JWT")
	}
}

func TestExpectedStatusesDoNotAlert(t *testing.T) {
	for _, code := range []string{"400", "404", "409", "501", "503"} {
		t.Run(code, func(t *testing.T) {
			h, b, _ := alertTestRouter(t)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status/"+code, nil))
			if fmt.Sprint(rec.Code) != code {
				t.Fatalf("estado %d, quería %s", rec.Code, code)
			}
			b.expectNone(t)
		})
	}
}

// http.ErrAbortHandler es como net/http corta una respuesta a propósito: se repropaga y no avisa.
func TestAbortHandlerIsNotAnAlert(t *testing.T) {
	h, b, _ := alertTestRouter(t)
	func() {
		defer func() {
			if rec := recover(); rec != http.ErrAbortHandler {
				t.Fatalf("ErrAbortHandler no se repropagó; recover = %v", rec)
			}
		}()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/abort", nil))
	}()
	b.expectNone(t)
}

// El correo sale a un buzón externo: nada de lo que mandó la tableta puede viajar en él.
func TestAlertEmailCarriesNoSecrets(t *testing.T) {
	h, b, jm := alertTestRouter(t)
	token, err := jm.Issue(domain.User{ID: 12, CompanyID: 2, Name: "Ana", Role: domain.Role("admin")})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"password":"hunter2","pin":"4821","customerName":"Cliente Secreto","card":"4111111111111111"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/9/pay?token="+sampleAlertJWT, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), req)

	m := b.waitOne(t)
	for _, secret := range []string{
		token, sampleAlertJWT, "hunter2", "4821", "Cliente Secreto", "4111111111111111", "token=", "/orders/9/",
	} {
		if strings.Contains(m, secret) {
			t.Errorf("el correo de aviso trae %q", secret)
		}
	}
	for _, want := range []string{"#12 Ana", "<td>#2</td>", "POST /api/v1/orders/{id}/pay"} {
		if !strings.Contains(m, want) {
			t.Errorf("el aviso no trae %q:\n%s", want, m)
		}
	}
}

// Un 500 lento de responder no se vuelve más lento por avisar, aunque el correo tarde.
func TestAlertingDoesNotDelayTheResponse(t *testing.T) {
	slow := func(_, _, _ string) error { time.Sleep(2 * time.Second); return nil }
	n := alerts.New(alerts.Options{To: "x@y.z", Send: slow, Window: time.Hour})
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		_ = n.Close(ctx)
	}()
	r := chi.NewRouter()
	baseMiddlewares(r, config.Config{Env: "development"}, n)
	r.Get("/x", func(w http.ResponseWriter, _ *http.Request) { Error(w, errors.New("x")) })
	start := time.Now()
	for range 5 {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("5 respuestas 500 con un SMTP de 2 s tardaron %v", d)
	}
}

// Un handler que manda encabezados con Flush antes de escribir ya respondió 200. Si después entra
// en panic, el middleware no puede pintar un 500 encima de una respuesta ya enviada.
func TestPanicAfterFlushDoesNotRewriteTheResponse(t *testing.T) {
	n := alerts.New(alerts.Options{To: "x@y.z", Send: (&alertMailbox{}).send, Window: time.Hour})
	defer func() { _ = n.Close(context.Background()) }()
	r := chi.NewRouter()
	baseMiddlewares(r, config.Config{Env: "development"}, n)
	r.Get("/stream", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		panic("se cayó a media transmisión")
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stream", nil))
	if strings.Contains(rec.Body.String(), "INTERNAL") {
		t.Fatalf("pintó un 500 dentro de una respuesta ya enviada: %q", rec.Body.String())
	}
}

// El X-Request-Id lo manda el cliente y viaja al correo: solo se acepta con forma de id.
func TestHostileRequestIDIsReplaced(t *testing.T) {
	for _, hostile := range []string{
		"Soporte urgente: llama al 55-1234-5678 y dicta el código",
		strings.Repeat("a", 10_000),
		"<b>hola</b>",
	} {
		h, b, _ := alertTestRouter(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/things/1/fail", nil)
		req.Header.Set(RequestIDHeader, hostile)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get(RequestIDHeader); got == hostile || len(got) > 64 {
			t.Errorf("un id hostil de %d bytes se aceptó", len(got))
		}
		if m := b.waitOne(t); strings.Contains(m, hostile) {
			t.Errorf("el correo trae el X-Request-Id hostil")
		}
	}
	// El que manda el front (un UUID) se respeta: es el que une la tableta con la bitácora.
	h, _, _ := alertTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(RequestIDHeader, "5f0c2a1e-9b7d-4c3e-8a21-0d6f4b9e7c11")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get(RequestIDHeader); got != "5f0c2a1e-9b7d-4c3e-8a21-0d6f4b9e7c11" {
		t.Fatalf("un UUID válido se reemplazó por %q", got)
	}
}
