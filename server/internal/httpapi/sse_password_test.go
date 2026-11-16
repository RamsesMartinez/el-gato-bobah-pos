package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// Un stream de `/events` abierto antes de que el admin restablezca la contraseña seguía recibiendo
// los eventos de la empresa sin fin: el control corre al conectar y el stream no termina. Se vuelve
// a revisar en cada ping y se corta (#311).
func TestTheEventStreamStopsWhenThePasswordChangeBecomesPending(t *testing.T) {
	prev := ssePingEvery
	ssePingEvery = 10 * time.Millisecond
	t.Cleanup(func() { ssePingEvery = prev })

	var pending atomic.Bool
	h := NewHandlers(Deps{Broker: realtime.NewBroker(), PasswordCheck: func(context.Context, AuthUser) (bool, error) {
		return pending.Load(), nil
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).
		WithContext(context.WithValue(ctx, userCtxKey, AuthUser{ID: 7, CompanyID: 1}))
	done := make(chan struct{})
	go func() {
		h.Events(httptest.NewRecorder(), req)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	pending.Store(true)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("el stream siguió abierto con el cambio de contraseña pendiente")
	}
}
