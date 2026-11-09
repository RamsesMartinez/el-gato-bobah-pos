package alerts

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

type mail struct{ to, subject, body string }

// inbox es el enviador falso: guarda lo que se habría mandado.
type inbox struct {
	mu    sync.Mutex
	mails []mail
	delay time.Duration
	err   error
}

func (b *inbox) send(to, subject, body string) error {
	if b.delay > 0 {
		time.Sleep(b.delay)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}
	b.mails = append(b.mails, mail{to, subject, body})
	return nil
}

func (b *inbox) all() []mail {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]mail(nil), b.mails...)
}

func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("no pasó a tiempo: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func alertFor(route, kind string) domain.ServerAlert {
	return domain.ServerAlert{
		At: time.Now(), Environment: "test", Method: "POST", Route: route, Kind: kind,
		RequestID: "r", Message: "boom", CompanyID: 2,
	}
}

func newTestNotifier(t *testing.T, b *inbox, o Options) *Notifier {
	t.Helper()
	o.To = "dueno@example.com"
	o.Send = b.send
	if o.Window == 0 {
		o.Window = 150 * time.Millisecond
	}
	if o.QueueSize == 0 {
		o.QueueSize = 512
	}
	n := New(o)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = n.Close(ctx)
	})
	return n
}

func TestFirstErrorMailsAtOnceAndRepeatsWaitForTheWindow(t *testing.T) {
	b := &inbox{}
	n := newTestNotifier(t, b, Options{Window: 300 * time.Millisecond})
	for range 100 {
		n.Report(alertFor("/api/v1/orders/{id}", "pg:23505"))
	}
	waitFor(t, "el aviso inmediato", time.Second, func() bool { return len(b.all()) >= 1 })
	time.Sleep(100 * time.Millisecond) // dentro de la ventana
	if got := len(b.all()); got != 1 {
		t.Fatalf("dentro de la ventana salieron %d correos; debía ser 1", got)
	}
	if b.all()[0].to != "dueno@example.com" {
		t.Fatalf("destinatario %q", b.all()[0].to)
	}
	waitFor(t, "el resumen al cerrar la ventana", 2*time.Second, func() bool { return len(b.all()) >= 2 })
	if s := b.all()[1].subject; !strings.Contains(s, "repetido 99 veces") {
		t.Fatalf("el resumen no dice 99 repeticiones: %q", s)
	}
}

func TestQuietWindowSendsNothingAtClose(t *testing.T) {
	b := &inbox{}
	n := newTestNotifier(t, b, Options{})
	n.Report(alertFor("/a", "k"))
	waitFor(t, "el aviso inmediato", time.Second, func() bool { return len(b.all()) == 1 })
	time.Sleep(400 * time.Millisecond) // más de dos ventanas
	if got := len(b.all()); got != 1 {
		t.Fatalf("una ventana sin repeticiones mandó %d correos; debía quedarse en 1", got)
	}
	// Ya cerrada, el mismo error vuelve a avisar al instante.
	n.Report(alertFor("/a", "k"))
	waitFor(t, "el aviso tras una ventana cerrada", time.Second, func() bool { return len(b.all()) == 2 })
}

func TestDifferentRouteOrKindMailsSeparately(t *testing.T) {
	b := &inbox{}
	n := newTestNotifier(t, b, Options{Window: time.Second})
	n.Report(alertFor("/a", "k1"))
	n.Report(alertFor("/b", "k1"))
	n.Report(alertFor("/a", "k2"))
	waitFor(t, "tres avisos", time.Second, func() bool { return len(b.all()) == 3 })
}

func TestRepeatsAfterASummaryRenewTheWindow(t *testing.T) {
	b := &inbox{}
	n := newTestNotifier(t, b, Options{Window: 200 * time.Millisecond})
	n.Report(alertFor("/a", "k"))
	n.Report(alertFor("/a", "k"))
	waitFor(t, "resumen", time.Second, func() bool { return len(b.all()) == 2 })
	// La ventana se renovó: una repetición nueva no avisa al instante, va al siguiente resumen.
	n.Report(alertFor("/a", "k"))
	time.Sleep(50 * time.Millisecond)
	if got := len(b.all()); got != 2 {
		t.Fatalf("una repetición en la ventana renovada mandó correo inmediato (%d correos)", got)
	}
	waitFor(t, "segundo resumen", time.Second, func() bool { return len(b.all()) == 3 })
	if s := b.all()[2].subject; !strings.Contains(s, "repetido 1 veces") {
		t.Fatalf("segundo resumen: %q", s)
	}
}

func TestReportNeverWaitsForASlowSender(t *testing.T) {
	b := &inbox{delay: 500 * time.Millisecond}
	n := newTestNotifier(t, b, Options{QueueSize: 4})
	start := time.Now()
	for i := range 50 {
		n.Report(alertFor("/a", string(rune('a'+i%26))))
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("50 Report con un enviador lento tardaron %v; no deben esperar", d)
	}
}

func TestFullQueueDropsAndTheNextMailCountsThem(t *testing.T) {
	b := &inbox{delay: 200 * time.Millisecond}
	n := newTestNotifier(t, b, Options{QueueSize: 1, Window: time.Hour})
	for i := range 20 {
		n.Report(alertFor("/r", string(rune('a'+i))))
	}
	waitFor(t, "algún correo con descartados", 3*time.Second, func() bool {
		for _, m := range b.all() {
			if strings.Contains(m.body, "descartados") {
				return true
			}
		}
		return false
	})
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	var mu sync.Mutex
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(lockedWriter{&buf, &mu}, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

type lockedWriter struct {
	b  *bytes.Buffer
	mu *sync.Mutex
}

func (w lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func TestSendFailureIsLoggedAndNotRetried(t *testing.T) {
	logs := captureLogs(t)
	calls := 0
	var mu sync.Mutex
	n := New(Options{To: "x@y.z", Window: time.Hour, Send: func(_, _, _ string) error {
		mu.Lock()
		calls++
		mu.Unlock()
		return errors.New("535 auth failed")
	}})
	n.Report(alertFor("/a", "k"))
	time.Sleep(200 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = n.Close(ctx)
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("un envío fallido se intentó %d veces; no se reintenta", calls)
	}
	if !strings.Contains(logs.String(), "server_alert_send_failed") {
		t.Fatalf("el fallo no quedó en la bitácora con su clave: %s", logs.String())
	}
	if strings.Contains(logs.String(), "boom") {
		t.Fatal("la bitácora del fallo trae el contenido del correo")
	}
}

// Redis caído no detiene los avisos: el agrupamiento sigue en memoria.
func TestUnreachableRedisFallsBackToMemory(t *testing.T) {
	b := &inbox{}
	n := newTestNotifier(t, b, Options{RedisURL: "redis://127.0.0.1:1/0", Window: 300 * time.Millisecond})
	for range 5 {
		n.Report(alertFor("/a", "k"))
	}
	waitFor(t, "aviso inmediato con Redis caído", 3*time.Second, func() bool { return len(b.all()) >= 1 })
	waitFor(t, "resumen con Redis caído", 3*time.Second, func() bool { return len(b.all()) >= 2 })
	if got := len(b.all()); got != 2 {
		t.Fatalf("con Redis caído salieron %d correos; debían ser 2", got)
	}
	if s := b.all()[1].subject; !strings.Contains(s, "repetido 4 veces") {
		t.Fatalf("resumen en memoria: %q", s)
	}
}

func TestCloseSendsPendingSummariesWithinTheDeadline(t *testing.T) {
	b := &inbox{}
	n := New(Options{To: "x@y.z", Send: b.send, Window: time.Hour})
	n.Report(alertFor("/a", "k"))
	n.Report(alertFor("/a", "k"))
	n.Report(alertFor("/a", "k"))
	waitFor(t, "inmediato", time.Second, func() bool { return len(b.all()) == 1 })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := n.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := b.all(); len(got) != 2 || !strings.Contains(got[1].subject, "repetido 2 veces") {
		t.Fatalf("al cerrar no salió el resumen pendiente: %+v", got)
	}
}

func TestCloseRespectsItsDeadlineWithAHungSender(t *testing.T) {
	b := &inbox{delay: 3 * time.Second}
	n := New(Options{To: "x@y.z", Send: b.send, Window: time.Hour})
	n.Report(alertFor("/a", "k"))
	time.Sleep(20 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_ = n.Close(ctx)
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("Close tardó %v con un plazo de 200ms", d)
	}
}

func TestReportAfterCloseAndOnNilIsHarmless(t *testing.T) {
	var nilN *Notifier
	nilN.Report(alertFor("/a", "k")) // avisos apagados: no truena
	if err := nilN.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	n := New(Options{To: "x@y.z", Send: (&inbox{}).send})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = n.Close(ctx)
	done := make(chan struct{})
	go func() { n.Report(alertFor("/a", "k")); close(done) }()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Report después de Close se bloqueó")
	}
}

func TestCompanyNameIsLookedUpAndFallsBackToTheID(t *testing.T) {
	b := &inbox{}
	n := newTestNotifier(t, b, Options{CompanyName: func(_ context.Context, id int64) (string, error) {
		if id == 2 {
			return "El Gato Bobah", nil
		}
		return "", errors.New("db down")
	}})
	n.Report(alertFor("/a", "k"))
	other := alertFor("/b", "k")
	other.CompanyID = 3
	n.Report(other)
	waitFor(t, "dos avisos", time.Second, func() bool { return len(b.all()) == 2 })
	if !strings.Contains(b.all()[0].body, "#2 El Gato Bobah") {
		t.Errorf("no puso el nombre de la empresa: %s", b.all()[0].body)
	}
	if !strings.Contains(b.all()[1].body, "<td>#3</td>") {
		t.Errorf("sin nombre no cayó al id: %s", b.all()[1].body)
	}
}
