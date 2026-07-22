package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// GET /events — stream SSE del tablero de pedidos.
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

	// ping inicial para abrir el stream
	_, _ = fmt.Fprint(w, ": ok\n\n")
	flusher.Flush()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev := <-ch:
			data, _ := json.Marshal(ev)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
			flusher.Flush()
		}
	}
}
