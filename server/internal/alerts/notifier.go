package alerts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Options configures a Notifier. Window y QueueSize existen para los tests; en producción van los
// valores por omisión.
type Options struct {
	To          string
	Send        func(to, subject, htmlBody string) error
	CompanyName func(ctx context.Context, companyID int64) (string, error)
	RedisURL    string        // vacío = agrupamiento solo en memoria
	Window      time.Duration // 0 = domain.AlertWindow
	QueueSize   int           // 0 = 64
}

// Notifier recibe avisos de 500 sin bloquear y los manda desde UN goroutine, que es el único que
// toca Redis, la base y el SMTP. Así ningún request espera nada de eso, y el estado de las
// ventanas (owned) no necesita mutex.
//
// Un Notifier nil es válido: los avisos están apagados.
type Notifier struct {
	o       Options
	rdb     *redis.Client
	procID  string
	queue   chan domain.ServerAlert
	flushes chan string
	stop    chan struct{}
	done    chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc
	closed  atomic.Bool
	dropped atomic.Int64
	once    sync.Once

	owned map[string]*window // solo lo toca run()
}

// window es una ventana que ESTE proceso ganó: él manda el aviso inmediato y el resumen.
type window struct {
	first    domain.ServerAlert
	viaRedis bool  // si false, el conteo vive solo aquí (Redis caído o no configurado)
	local    int64 // repeticiones que no se pudieron contar en Redis
	timer    *time.Timer
}

// New starts the notifier goroutine. Close lo termina.
func New(o Options) *Notifier {
	if o.Window <= 0 {
		o.Window = domain.AlertWindow
	}
	if o.QueueSize <= 0 {
		o.QueueSize = 64
	}
	ctx, cancel := context.WithCancel(context.Background())
	n := &Notifier{
		o: o, rdb: newRedis(o.RedisURL), procID: randomID(),
		queue: make(chan domain.ServerAlert, o.QueueSize), flushes: make(chan string),
		stop: make(chan struct{}), done: make(chan struct{}),
		ctx: ctx, cancel: cancel, owned: map[string]*window{},
	}
	go n.run()
	return n
}

// Report queues an alert without ever blocking: si la cola está llena se descarta y se cuenta, y el
// siguiente correo lo dice. Esperar aquí sería atrasar la respuesta de la tableta.
func (n *Notifier) Report(a domain.ServerAlert) {
	if n == nil || n.closed.Load() {
		return
	}
	select {
	case n.queue <- a:
	default:
		n.dropped.Add(1)
	}
}

// Close sends what is pending (alertas en cola y resúmenes de las ventanas propias) and stops,
// waiting at most until ctx expires. La cola nunca se cierra: un Report tardío no entra en pánico.
func (n *Notifier) Close(ctx context.Context) error {
	if n == nil {
		return nil
	}
	n.once.Do(func() {
		n.closed.Store(true)
		close(n.stop)
	})
	select {
	case <-n.done:
		return nil
	case <-ctx.Done():
		n.cancel() // corta lo que espera a Redis o a la base; el SMTP termina por su propio plazo
		slog.Warn("server_alert_shutdown_incomplete", "error", ctx.Err())
		return ctx.Err()
	}
}

func (n *Notifier) run() {
	defer close(n.done)
	defer n.cancel()
	defer func() {
		if n.rdb != nil {
			_ = n.rdb.Close()
		}
	}()
	for {
		select {
		case a := <-n.queue:
			n.handle(a)
		case key := <-n.flushes:
			n.flush(key, true)
		case <-n.stop:
			n.drain()
			return
		}
	}
}

// drain atiende lo que quedó en la cola y manda los resúmenes pendientes. En una subida azul/verde
// el proceso viejo se apaga con ventanas abiertas: sin esto, sus repeticiones se perderían (o
// esperarían a que el error vuelva a pasar para salir como «anteriores sin reportar»).
func (n *Notifier) drain() {
	for len(n.queue) > 0 {
		n.handle(<-n.queue)
	}
	for key, w := range n.owned {
		w.timer.Stop()
		n.flush(key, false)
	}
}

func (n *Notifier) handle(a domain.ServerAlert) {
	key := domain.AlertKey(a)
	if w, ok := n.owned[key]; ok {
		n.repeat(key, w)
		return
	}
	won, orphaned, err := n.claim(key)
	if err != nil {
		if !errors.Is(err, errNoRedis) {
			slog.Warn("server_alert_redis_failed", "op", "claim", "error", err)
		}
		// Sin Redis no hay quien diga si otro proceso ya avisó: se avisa. Duplicar es mejor que callar.
		won, orphaned = true, 0
	}
	if !won {
		n.countForOwner(key)
		return
	}
	n.fillCompany(&a)
	w := &window{first: a, viaRedis: err == nil}
	n.owned[key] = w
	n.schedule(key, w)
	n.send(a, domain.AlertSummary{Orphaned: orphaned})
}

// repeat cuenta una repetición de una ventana propia, en Redis si la ventana vive ahí (así el
// resumen suma también las de otros procesos) y aquí si no.
func (n *Notifier) repeat(key string, w *window) {
	if w.viaRedis && n.incr(key) == nil {
		return
	}
	w.local++
}

func (n *Notifier) countForOwner(key string) {
	if err := n.incr(key); err != nil {
		slog.Warn("server_alert_redis_failed", "op", "incr", "error", err)
	}
}

// flush cierra la ventana: si hubo repeticiones manda el resumen y la renueva, porque el error
// sigue pasando; si no, la suelta. Al apagarse (renew=false) solo manda lo pendiente.
func (n *Notifier) flush(key string, renew bool) {
	w, ok := n.owned[key]
	if !ok {
		return
	}
	repeats := w.local
	w.local = 0
	if w.viaRedis {
		c, err := n.takeCount(key)
		if err != nil {
			slog.Warn("server_alert_redis_failed", "op", "getdel", "error", err)
		}
		repeats += c
	}
	if repeats > 0 {
		n.send(w.first, domain.AlertSummary{Repeats: repeats})
	}
	if repeats > 0 && renew && n.renew(key, w) {
		n.schedule(key, w)
		return
	}
	delete(n.owned, key)
	if w.viaRedis {
		n.release(key)
	}
}

func (n *Notifier) renew(key string, w *window) bool {
	if !w.viaRedis {
		return true
	}
	ok, err := n.extend(key)
	if err != nil {
		slog.Warn("server_alert_redis_failed", "op", "renew", "error", err)
	}
	return ok
}

func (n *Notifier) schedule(key string, w *window) {
	w.timer = time.AfterFunc(n.o.Window, func() {
		select {
		case n.flushes <- key:
		case <-n.stop:
		}
	})
}

func (n *Notifier) fillCompany(a *domain.ServerAlert) {
	if a.CompanyID == 0 || n.o.CompanyName == nil {
		return
	}
	// Si el 500 es porque la base no responde, esto también falla: el correo sale con el id.
	ctx, cancel := context.WithTimeout(n.ctx, 2*time.Second)
	defer cancel()
	if name, err := n.o.CompanyName(ctx, a.CompanyID); err == nil {
		a.CompanyName = name
	}
}

func (n *Notifier) send(a domain.ServerAlert, s domain.AlertSummary) {
	s.Dropped = n.dropped.Swap(0)
	subject, body := domain.AlertEmail(a, s)
	if err := n.o.Send(n.o.To, subject, body); err != nil {
		// Sin reintento: un SMTP caído no se arregla insistiendo, y la bitácora guarda 30 días.
		// Solo la causa y la llave del error; el contenido del correo no va a la bitácora.
		slog.Error("server_alert_send_failed", "error", err, "kind", a.Kind, "request_id", a.RequestID)
	}
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
