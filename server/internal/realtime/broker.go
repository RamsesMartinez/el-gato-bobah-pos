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
	mu   sync.RWMutex
	subs map[int64]map[chan Event]struct{} // companyID → suscriptores
}

func NewBroker() *Broker {
	return &Broker{subs: map[int64]map[chan Event]struct{}{}}
}

// Subscribe registra un suscriptor para una empresa y devuelve su canal + función de baja.
func (b *Broker) Subscribe(companyID int64) (<-chan Event, func()) {
	ch := make(chan Event, 16)
	b.mu.Lock()
	if b.subs[companyID] == nil {
		b.subs[companyID] = map[chan Event]struct{}{}
	}
	b.subs[companyID][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if m := b.subs[companyID]; m != nil {
			delete(m, ch)
			if len(m) == 0 {
				delete(b.subs, companyID)
			}
		}
		close(ch)
		b.mu.Unlock()
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
