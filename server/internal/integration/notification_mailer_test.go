//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// EL CORREO DE AVISOS (spec 084, historia 6): un correo por bloque de 5 minutos con todo lo nuevo,
// ninguno si no hay nada, enfriamiento de 12 h por artículo y el resumen de la mañana una vez por día
// aunque el proceso se reinicie. El reloj de la API es falso; las horas de los avisos se fijan en la
// base para que los dos relojes hablen del mismo momento.

type mailbox struct {
	mu      sync.Mutex
	sent    []struct{ to, subject, html string }
	fail    error
	failFor string // un destinatario que el servidor de correo rechaza
}

func (b *mailbox) send(to []string, subject, html string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fail != nil {
		return b.fail
	}
	for _, t := range to {
		if t == b.failFor {
			return errors.New("550 <" + t + ">: Recipient address rejected")
		}
	}
	for _, t := range to {
		b.sent = append(b.sent, struct{ to, subject, html string }{t, subject, html})
	}
	return nil
}

func (b *mailbox) count() int { b.mu.Lock(); defer b.mu.Unlock(); return len(b.sent) }

type mailerKit struct {
	st    *store.Store
	app   *store.Store
	box   *mailbox
	user  int64
	block time.Time // inicio de un bloque de 5 minutos
}

func newMailerKit(t *testing.T) *mailerKit {
	t.Helper()
	st := newTestStore(t)
	k := &mailerKit{st: st, app: appRoleStore(t), box: &mailbox{}}
	k.user = makeUser(t, st, "gerente_084m", "gerente")
	setEmail(t, st, k.user, "gerente084m@ejemplo.mx")
	k.block = time.Date(2026, 11, 14, 2, 35, 0, 0, time.UTC) // 20:35 en el centro de México
	return k
}

func (k *mailerKit) mailer(now time.Time) *app.NotificationMailer {
	return app.NewNotificationMailer(k.app, func() time.Time { return now }, k.box.send, "https://app.ejemplo.mx")
}

// openAt abre un «se acabó» de un insumo nuevo y fija su hora en la base.
func (k *mailerKit) openAt(t *testing.T, name string, at time.Time) (ing, notification int64) {
	t.Helper()
	ing = pieceIngredient(t, k.st, name)
	adjust(t, k.st, "ingrediente", ing, "1")
	alertOn(t, k.st, ing, "", false, true)
	adjust(t, k.st, "ingrediente", ing, "-1")
	if err := k.st.Pool.QueryRow(context.Background(),
		`update notifications set opened_at = $2 where ingredient_id = $1 and status = 'open' returning id`, ing, at).Scan(&notification); err != nil {
		t.Fatalf("aviso de %s: %v", name, err)
	}
	return ing, notification
}

func TestOneEmailPerWindow(t *testing.T) {
	t.Parallel()
	k := newMailerKit(t)
	k.openAt(t, "Bolsas 084m", k.block.Add(1*time.Minute))
	k.openAt(t, "Platos 084m", k.block.Add(2*time.Minute))
	k.openAt(t, "Queso 084m", k.block.Add(3*time.Minute))
	ctx := context.Background()

	if err := k.mailer(k.block.Add(4*time.Minute)).SendWindows(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if k.box.count() != 0 {
		t.Fatal("salió correo antes de que cerrara el bloque")
	}
	if err := k.mailer(k.block.Add(5*time.Minute+time.Second)).SendWindows(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if k.box.count() != 1 {
		t.Fatalf("salieron %d correos; uno por bloque", k.box.count())
	}
	for _, name := range []string{"Bolsas 084m", "Platos 084m", "Queso 084m"} {
		if !strings.Contains(k.box.sent[0].html, name) {
			t.Fatalf("el correo no trae %s", name)
		}
	}
	if err := k.mailer(k.block.Add(11*time.Minute)).SendWindows(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if k.box.count() != 1 {
		t.Fatalf("un bloque sin nada nuevo mandó correo: %d", k.box.count())
	}
}

func TestResolvedBeforeTheWindowIsNotEmailed(t *testing.T) {
	t.Parallel()
	k := newMailerKit(t)
	ing, _ := k.openAt(t, "Se resolvió 084m", k.block.Add(time.Minute))
	adjust(t, k.st, "ingrediente", ing, "5") // vuelve a haber: se resuelve antes de que salga el correo
	if err := k.mailer(k.block.Add(6*time.Minute)).SendWindows(context.Background(), defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if k.box.count() != 0 {
		t.Fatal("un aviso ya resuelto salió por correo")
	}
	var state string
	if err := k.st.Pool.QueryRow(context.Background(), `select email_state from notifications where ingredient_id = $1`, ing).Scan(&state); err != nil || state != "skipped" {
		t.Fatalf("estado %s %v", state, err)
	}
}

func TestCooldownPerItem(t *testing.T) {
	t.Parallel()
	k := newMailerKit(t)
	ctx := context.Background()
	ing, _ := k.openAt(t, "Oscila 084m", k.block.Add(time.Minute))
	if err := k.mailer(k.block.Add(6*time.Minute)).SendWindows(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if k.box.count() != 1 {
		t.Fatalf("primer correo: %d", k.box.count())
	}
	// Vuelve a haber y se vuelve a acabar dos horas después del primer correo.
	if _, err := k.st.Pool.Exec(ctx, `update notification_email_sends set sent_at = $1`, k.block.Add(6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	adjust(t, k.st, "ingrediente", ing, "3")
	adjust(t, k.st, "ingrediente", ing, "-3")
	later := k.block.Add(2 * time.Hour)
	if _, err := k.st.Pool.Exec(ctx, `update notifications set opened_at = $2 where ingredient_id = $1 and status = 'open'`, ing, later.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := k.mailer(later.Add(6*time.Minute)).SendWindows(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if k.box.count() != 1 {
		t.Fatalf("el mismo artículo mandó otro correo antes de 12 h: %d", k.box.count())
	}
	var open int
	if err := k.st.Pool.QueryRow(ctx, `select count(*) from notifications where ingredient_id = $1 and status = 'open'`, ing).Scan(&open); err != nil || open != 1 {
		t.Fatalf("el aviso tiene que seguir en la campana: %d %v", open, err)
	}
}

func TestTwoProcessesClaimingTheSameWindowSendOne(t *testing.T) {
	t.Parallel()
	k := newMailerKit(t)
	k.openAt(t, "Carrera 084m", k.block.Add(time.Minute))
	now := k.block.Add(6 * time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = k.mailer(now).SendWindows(context.Background(), defaultCompanyID) }()
	}
	wg.Wait()
	if k.box.count() != 1 {
		t.Fatalf("dos procesos mandaron %d correos del mismo bloque", k.box.count())
	}
}

func TestAMailFailureRetriesAndNeverBlocksASale(t *testing.T) {
	t.Parallel()
	k := newMailerKit(t)
	ctx := context.Background()
	ing, _ := k.openAt(t, "Falla 084m", k.block.Add(time.Minute))
	k.box.fail = errors.New("smtp caído")
	now := k.block.Add(6 * time.Minute)
	if err := k.mailer(now).SendWindows(ctx, defaultCompanyID); err == nil {
		t.Fatal("la falla del correo no se reportó")
	}
	// Mientras el correo falla, las ventas siguen.
	adjust(t, k.st, "ingrediente", ing, "-1")
	k.box.fail = nil
	if err := k.mailer(now.Add(time.Minute)).SendWindows(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if k.box.count() != 1 {
		t.Fatalf("tras la falla se mandaron %d", k.box.count())
	}
}

func TestMorningSummaryOncePerDayAcrossRestarts(t *testing.T) {
	t.Parallel()
	k := newMailerKit(t)
	ctx := context.Background()
	k.openAt(t, "Mañana 084m", k.block.Add(-time.Hour))
	// 08:05 del 15 de noviembre en el centro de México = 14:05 UTC.
	at := time.Date(2026, 11, 15, 14, 5, 0, 0, time.UTC)
	before := time.Date(2026, 11, 15, 13, 55, 0, 0, time.UTC)
	if err := k.mailer(before).SendMorning(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if k.box.count() != 0 {
		t.Fatal("el resumen salió antes de su hora")
	}
	if err := k.mailer(at).SendMorning(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if err := k.mailer(at.Add(10*time.Minute)).SendMorning(ctx, defaultCompanyID); err != nil { // «reinicio»
		t.Fatal(err)
	}
	if k.box.count() != 1 {
		t.Fatalf("resúmenes de la mañana: %d", k.box.count())
	}
	if !strings.Contains(k.box.sent[0].html, "Mañana 084m") || !strings.Contains(k.box.sent[0].subject, "mañana") {
		t.Fatalf("resumen: %s", k.box.sent[0].subject)
	}
}

func (b *mailbox) to(addr string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, m := range b.sent {
		if m.to == addr {
			n++
		}
	}
	return n
}

// Sin nadie que reciba, el bloque se daba por enviado: el aviso quedaba «enviado» sin que nadie lo
// leyera y el enfriamiento de 12 h callaba la siguiente vez que se acabara, ya con destinatario.
func TestWithoutRecipientsNothingCountsAsEmailed(t *testing.T) {
	t.Parallel()
	k := newMailerKit(t)
	ctx := context.Background()
	if _, err := k.st.Pool.Exec(ctx, `update users set recovery_email = null where recovery_email is not null`); err != nil {
		t.Fatal(err)
	}
	ing, _ := k.openAt(t, "Sin destinatario 084m", k.block.Add(time.Minute))
	if err := k.mailer(k.block.Add(6*time.Minute)).SendWindows(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := k.st.Pool.QueryRow(ctx, `select email_state from notifications where ingredient_id = $1`, ing).Scan(&state); err != nil || state == "sent" {
		t.Fatalf("sin destinatarios el aviso quedó %q (%v)", state, err)
	}
	// Llega una gerente con correo y el mismo insumo se vuelve a acabar dos horas después.
	setEmail(t, k.st, k.user, "gerente084m@ejemplo.mx")
	adjust(t, k.st, "ingrediente", ing, "3")
	adjust(t, k.st, "ingrediente", ing, "-3")
	later := k.block.Add(2 * time.Hour)
	if _, err := k.st.Pool.Exec(ctx, `update notifications set opened_at = $2 where ingredient_id = $1 and status = 'open'`, ing, later.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := k.mailer(later.Add(6*time.Minute)).SendWindows(ctx, defaultCompanyID); err != nil {
		t.Fatal(err)
	}
	if k.box.count() != 1 {
		t.Fatalf("el enfriamiento contó un correo que no salió: %d enviados", k.box.count())
	}
}

// Un destinatario que el servidor rechaza cortaba el bloque para todos los demás y, al reintentar,
// los que ya lo habían recibido lo recibían otra vez. Además, el rechazo traía la dirección ajena y
// terminaba en la bitácora de la API cada minuto.
func TestOneBadRecipientDoesNotStopTheOthers(t *testing.T) {
	t.Parallel()
	k := newMailerKit(t)
	ctx := context.Background()
	svc, _, nctx := asNotifications(t, defaultCompanyID)
	v, err := svc.Recipients(nctx)
	if err != nil {
		t.Fatal(err)
	}
	v.Extras = append(v.Extras, app.RecipientExtra{Email: "rebota084@tercero.mx", Stock: true}, app.RecipientExtra{Email: "compras084@ejemplo.mx", Stock: true})
	if err := svc.SetRecipients(nctx, v, k.user); err != nil {
		t.Fatal(err)
	}
	k.box.failFor = "rebota084@tercero.mx"
	k.openAt(t, "Rebote 084m", k.block.Add(time.Minute))
	err = k.mailer(k.block.Add(6*time.Minute)).SendWindows(ctx, defaultCompanyID)
	if err != nil && strings.Contains(err.Error(), "tercero.mx") {
		t.Fatalf("el error lleva la dirección ajena a la bitácora: %v", err)
	}
	if k.box.to("gerente084m@ejemplo.mx") != 1 || k.box.to("compras084@ejemplo.mx") != 1 {
		t.Fatalf("un rebote dejó sin correo a los demás: gerente %d, compras %d", k.box.to("gerente084m@ejemplo.mx"), k.box.to("compras084@ejemplo.mx"))
	}
	if err := k.mailer(k.block.Add(7*time.Minute)).SendWindows(ctx, defaultCompanyID); err != nil && strings.Contains(err.Error(), "tercero.mx") {
		t.Fatal(err)
	}
	if k.box.to("gerente084m@ejemplo.mx") != 1 {
		t.Fatalf("la gerente recibió el mismo bloque %d veces", k.box.to("gerente084m@ejemplo.mx"))
	}
}
