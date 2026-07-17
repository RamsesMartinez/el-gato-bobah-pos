package realtime

import "sync"

// Event es un mensaje que se envía a los tableros conectados por SSE.
type Event struct {
	Type string `json:"type"` // order.created | order.updated | menu.updated
	Data any    `json:"data,omitempty"`
}

// Broker es un pub/sub in-process. Suficiente para 1 réplica de API; la interfaz
// permite cambiar a Redis pub/sub si algún día hay varias.
type Broker struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

func NewBroker() *Broker {
	return &Broker{subs: map[chan Event]struct{}{}}
}

// Subscribe registra un suscriptor y devuelve su canal + función para darse de baja.
func (b *Broker) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 16)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		close(ch)
		b.mu.Unlock()
	}
}

// Publish envía un evento a todos los suscriptores (no bloqueante: si un canal está
// lleno, se omite ese evento para ese suscriptor).
func (b *Broker) Publish(ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}
