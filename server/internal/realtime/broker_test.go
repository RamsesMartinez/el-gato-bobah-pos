package realtime

import (
	"testing"
	"time"
)

// TestCloseEndsEverySubscriber cubre el apagado (spec 041): un `Shutdown` de net/http no cancela
// el contexto de las peticiones, así que un stream SSE solo termina si su canal se cierra. Sin
// esto cada subida esperaba el plazo completo y dejaba «context deadline exceeded» en el log.
func TestCloseEndsEverySubscriber(t *testing.T) {
	b := NewBroker()
	a, unsubA := b.Subscribe(1)
	c, unsubC := b.Subscribe(2)

	b.Close()

	for name, ch := range map[string]<-chan Event{"empresa 1": a, "empresa 2": c} {
		select {
		case _, ok := <-ch:
			if ok {
				t.Fatalf("%s: el canal recibió un evento en vez de cerrarse", name)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s: el canal sigue abierto tras Close; su stream SSE haría esperar el apagado", name)
		}
	}

	// La baja que corre el handler al regresar no puede cerrar dos veces el mismo canal.
	unsubA()
	unsubC()
}

func TestSubscribeAfterCloseReturnsAClosedChannel(t *testing.T) {
	b := NewBroker()
	b.Close()
	ch, unsub := b.Subscribe(1)
	defer unsub()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("recibió un evento")
		}
	case <-time.After(time.Second):
		t.Fatal("un stream que se abre durante el apagado se quedaría abierto hasta el plazo")
	}
}

func TestPublishAfterCloseDoesNotPanic(t *testing.T) {
	b := NewBroker()
	_, unsub := b.Subscribe(1)
	b.Close()
	b.Publish(1, Event{Type: "order.created"}) // un handler que termina su cobro durante el apagado
	unsub()
}
