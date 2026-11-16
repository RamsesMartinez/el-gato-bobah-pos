package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// GET /events — stream SSE del tablero de pedidos.
// ssePingEvery mantiene viva la conexión detrás de proxies que cortan lo que no habla. Es variable
// solo para que la prueba no espere 25 segundos.
var ssePingEvery = 25 * time.Second

func (h *Handlers) Events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		Error(w, fmt.Errorf("streaming no soportado"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Suscripción scopeada a la empresa del usuario: nunca recibe eventos de otro tenant.
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, fmt.Errorf("streaming requiere sesión"))
		return
	}
	ch, unsub := h.broker.Subscribe(u.CompanyID)
	defer unsub()
	// Un canal recién creado solo llega cerrado si el broker ya se cerró. Un 200 vacío haría que
	// la tableta reconectara en ciclo contra la copia que se va.
	select {
	case _, ok := <-ch:
		if !ok {
			Error(w, domain.ErrShuttingDown)
			return
		}
	default:
	}

	// ping inicial para abrir el stream
	_, _ = fmt.Fprint(w, ": ok\n\n")
	flusher.Flush()

	ping := time.NewTicker(ssePingEvery)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			// El control de la ruta corre al conectar; un restablecimiento de contraseña posterior
			// (#311) cortaría el stream hasta el siguiente ping, no nunca.
			if h.passwordCheck != nil {
				if pending, err := h.passwordCheck(r.Context(), u); err != nil || pending {
					return
				}
			}
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				// El broker se cerró: la API se está apagando. La tableta reconecta sola y cae en
				// la copia nueva.
				return
			}
			data, _ := json.Marshal(ev)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
			flusher.Flush()
		}
	}
}
