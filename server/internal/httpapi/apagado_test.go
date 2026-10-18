package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// suscrito avisa en el primer Flush, que el handler hace solo después de suscribirse: así el test
// cierra el broker con un suscriptor VIVO, no uno que nació cerrado.
type suscrito struct {
	*httptest.ResponseRecorder
	listo chan struct{}
	once  sync.Once
}

func (s *suscrito) Flush() {
	s.ResponseRecorder.Flush()
	s.once.Do(func() { close(s.listo) })
}

// TestEventsEndsWhenTheBrokerCloses: el contexto de la petición sigue vivo (net/http no lo
// cancela en Shutdown) y aun así el stream tiene que regresar en cuanto el broker se cierra.
func TestEventsEndsWhenTheBrokerCloses(t *testing.T) {
	b := realtime.NewBroker()
	h := &Handlers{broker: b}
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	req = req.WithContext(context.WithValue(context.Background(), userCtxKey, AuthUser{ID: 1, CompanyID: 1}))
	w := &suscrito{ResponseRecorder: httptest.NewRecorder(), listo: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		h.Events(w, req)
		close(done)
	}()
	select {
	case <-w.listo:
	case <-time.After(time.Second):
		t.Fatal("el stream nunca abrió")
	}
	b.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("el stream SSE siguió abierto con el broker cerrado: el apagado esperaría el plazo completo")
	}
}

// TestEventsDuringShutdownAnswers503: una tableta que todavía llega a la copia que se apaga no
// recibe un 200 vacío que la haga reconectar en ciclo contra la misma copia.
func TestEventsDuringShutdownAnswers503(t *testing.T) {
	b := realtime.NewBroker()
	b.Close()
	h := &Handlers{broker: b}
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	req = req.WithContext(context.WithValue(context.Background(), userCtxKey, AuthUser{ID: 1, CompanyID: 1}))
	rec := httptest.NewRecorder()
	h.Events(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/events durante el apagado = %d %q; quería 503", rec.Code, rec.Body.String())
	}
}

// TestReadyzSaysShuttingDownDuringShutdown: mientras la copia se apaga deja de anunciarse lista,
// pero sigue viva para terminar lo que trae.
func TestReadyzSaysShuttingDownDuringShutdown(t *testing.T) {
	h := &Handlers{}
	r := Router(config.Config{}, nil, h, nil)
	h.BeginShutdown()

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "shutting_down") {
		t.Fatalf("/readyz durante el apagado = %d %s; quería 503 shutting_down", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/healthz durante el apagado = %d; el proceso sigue vivo", rec.Code)
	}
}
