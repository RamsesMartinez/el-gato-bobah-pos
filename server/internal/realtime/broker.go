package realtime

import "sync"

// Event es un mensaje que se envía a los tableros conectados por SSE.
type Event struct {
	Type string `json:"type"` // order.created | order.updated | menu.updated
	Data any    `json:"data,omitempty"`
}

// Broker es un pub/sub in-process SCOPEADO POR EMPRESA: los eventos (que pueden llevar datos de
// pedidos) solo llegan a los suscriptores del MISMO tenant, nunca a otra empresa. Suficiente para
// 1 réplica; la interfaz permite migrar a Redis pub/sub (con el company_id en el canal) si crece.
type Broker struct {
	mu     sync.RWMutex
	subs   map[int64]map[chan Event]struct{} // companyID → suscriptores
	closed bool
}

func NewBroker() *Broker {
	return &Broker{subs: map[int64]map[chan Event]struct{}{}}
}

// Subscribe registra un suscriptor para una empresa y devuelve su canal + función de baja.
func (b *Broker) Subscribe(companyID int64) (<-chan Event, func()) {
	ch := make(chan Event, 16)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	if b.subs[companyID] == nil {
		b.subs[companyID] = map[chan Event]struct{}{}
	}
	b.subs[companyID][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		// Si ya no está, Close lo cerró: cerrarlo otra vez entra en pánico.
		m := b.subs[companyID]
		if _, ok := m[ch]; !ok {
			return
		}
		delete(m, ch)
		if len(m) == 0 {
			delete(b.subs, companyID)
		}
		close(ch)
	}
}

// Publish envía un evento SOLO a los suscriptores de esa empresa (no bloqueante: si un canal
// está lleno, se omite ese evento para ese suscriptor).
func (b *Broker) Publish(companyID int64, ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs[companyID] {
		select {
		case ch <- ev:
		default:
		}
	}
}

// Close cierra el canal de todos los suscriptores, y los que se suscriban después nacen cerrados.
//
// Es lo que deja apagar sin esperar: `http.Server.Shutdown` no cancela el contexto de las
// peticiones, y un stream SSE solo termina cuando su canal se cierra. Sin esto cada subida
// esperaba el plazo completo del apagado y dejaba «context deadline exceeded» en el log (spec 041).
func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for _, m := range b.subs {
		for ch := range m {
			close(ch)
		}
	}
	b.subs = map[int64]map[chan Event]struct{}{}
}
