package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/mailtpl"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// El correo de avisos de inventario (spec 084, historia 6). Un correo por bloque fijo de 5 minutos con
// todo lo nuevo, ninguno si no hay nada, enfriamiento de 12 h por artículo, y el resumen de la mañana.
// Sale DESPUÉS de confirmar (lo lee de la tabla, no del trigger): un aviso que se abrió y se resolvió
// antes de salir no se manda. Cada envío se reclama en su fila antes de mandar (dos APIs vivas o un
// reinicio no mandan dos veces), se manda FUERA de la transacción y se marca en otra, como el resumen
// del cierre. Una falla del correo deja los avisos pendientes y se reintenta en la siguiente pasada;
// nunca toca una venta.

// NotificationMailer arma y manda los correos.
type NotificationMailer struct {
	store   *store.Store
	now     func() time.Time
	send    SendMail
	baseURL string
}

// NewNotificationMailer arma el despachador. baseURL es la dirección del POS (para el logo y el botón).
func NewNotificationMailer(st *store.Store, now func() time.Time, send SendMail, baseURL string) *NotificationMailer {
	if now == nil {
		now = time.Now
	}
	return &NotificationMailer{store: st, now: now, send: send, baseURL: strings.TrimRight(baseURL, "/")}
}

// envelope es un correo listo para mandar a cada destinatario, con lo que se marca al confirmar.
type envelope struct {
	sendID  int64
	ids     []int64
	subject string
	bodies  map[string]string // destinatario → HTML
}

var (
	diasES  = []string{"Domingo", "Lunes", "Martes", "Miércoles", "Jueves", "Viernes", "Sábado"}
	mesesES = []string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}
)

func fechaES(t time.Time) string {
	return fmt.Sprintf("%s %d %s", diasES[t.Weekday()], t.Day(), mesesES[t.Month()-1])
}

// SendWindows manda el correo de cada bloque de 5 minutos ya cerrado que tenga algo nuevo.
func (m *NotificationMailer) SendWindows(ctx context.Context, company int64) error {
	now := m.now()
	var jobs []envelope
	err := m.store.WithTenant(ctx, company, func(q *db.Queries) error {
		pending, err := q.PendingEmailNotifications(ctx, domain.EmailWindowStart(now))
		if err != nil {
			return err
		}
		windows := map[time.Time][]db.PendingEmailNotificationsRow{}
		var order []time.Time
		for _, n := range pending {
			w := domain.EmailWindowStart(n.OpenedAt)
			if !domain.EmailWindowClosed(w, now) {
				continue
			}
			if _, ok := windows[w]; !ok {
				order = append(order, w)
			}
			windows[w] = append(windows[w], n)
		}
		for _, w := range order {
			job, err := m.windowJob(ctx, q, w, windows[w])
			if err != nil {
				return err
			}
			if job != nil {
				jobs = append(jobs, *job)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return m.deliver(ctx, company, jobs)
}

func (m *NotificationMailer) windowJob(ctx context.Context, q *db.Queries, w time.Time, rows []db.PendingEmailNotificationsRow) (*envelope, error) {
	claimed, err := q.ClaimNotificationSend(ctx, db.ClaimNotificationSendParams{Kind: "window", PeriodKey: domain.EmailWindowKey(w)})
	if err != nil {
		return nil, err
	}
	if len(claimed) == 0 {
		return nil, nil // otra API lo tomó, o ya salió
	}
	sendID := claimed[0]
	loc := businessLocation(ctx, q)
	var skip, keep []int64
	var items []mailItem
	for _, n := range rows {
		if n.Status != "open" {
			skip = append(skip, n.ID)
			continue
		}
		if n.Kind != string(domain.KindNotFound) {
			last, err := q.LastEmailedAt(ctx, n.ItemKey)
			if err != nil {
				return nil, err
			}
			if !last.IsZero() && last.Year() > 1970 && domain.InEmailCooldown(&last, n.OpenedAt) {
				skip = append(skip, n.ID)
				continue
			}
		}
		keep = append(keep, n.ID)
		items = append(items, mailItem{kind: n.Kind, name: n.ItemName, unit: n.UnitCode, query: n.QueryText, at: n.OpenedAt.In(loc),
			by: n.OpenedByName, onHand: n.OnHandSeen.String(), min: n.MinSeen.String(), needsCount: n.NeedsCount, key: keyOf(n.ItemKey)})
	}
	if len(skip) > 0 {
		if err := q.MarkNotificationsEmailed(ctx, db.MarkNotificationsEmailedParams{State: "skipped", Ids: skip}); err != nil {
			return nil, err
		}
	}
	if len(items) == 0 {
		return nil, q.FinishNotificationSend(ctx, db.FinishNotificationSendParams{ID: sendID, Status: "empty"})
	}
	period := fmt.Sprintf("%s · %s a %s", fechaES(w.In(loc)), w.In(loc).Format("15:04"), w.Add(5*time.Minute).In(loc).Format("15:04"))
	bodies, err := m.bodies(ctx, q, items, period, false)
	if err != nil {
		return nil, err
	}
	if len(bodies) == 0 {
		// Nadie lo recibe: no cuenta como enviado, o el enfriamiento callaría la próxima vez que se
		// acabe, cuando ya haya a quién avisarle.
		if err := q.MarkNotificationsEmailed(ctx, db.MarkNotificationsEmailedParams{State: "skipped", Ids: keep}); err != nil {
			return nil, err
		}
		return nil, q.FinishNotificationSend(ctx, db.FinishNotificationSendParams{ID: sendID, Status: "empty"})
	}
	subject := fmt.Sprintf("Avisos de inventario: %d %s", len(items), plural(len(items), "nuevo", "nuevos"))
	return &envelope{sendID: sendID, ids: keep, subject: subject, bodies: bodies}, nil
}

// SendMorning manda el resumen de la mañana una vez por día (fecha del negocio) a partir de su hora.
// Antes vuelve a evaluar lo que tiene alerta: si el trigger perdió un aviso, aquí se recupera.
func (m *NotificationMailer) SendMorning(ctx context.Context, company int64) error {
	now := m.now()
	var jobs []envelope
	err := m.store.WithTenant(ctx, company, func(q *db.Queries) error {
		at, err := q.NotificationsMorningAt(ctx)
		if err != nil {
			return err
		}
		loc := businessLocation(ctx, q)
		date, due, err := domain.MorningSummaryDue(now, loc, at)
		if err != nil || !due {
			return err
		}
		if done, err := q.MorningSendExists(ctx, date); err != nil || done {
			return err
		}
		enabled, err := q.EnabledLowStockItems(ctx)
		if err != nil {
			return err
		}
		for _, it := range enabled {
			ing, prod := itemIDs(it.ItemType, it.ID)
			if err := evaluateEverywhere(ctx, q, it.ItemType, ing, prod); err != nil {
				return err
			}
		}
		claimed, err := q.ClaimNotificationSend(ctx, db.ClaimNotificationSendParams{Kind: "morning", PeriodKey: date})
		if err != nil || len(claimed) == 0 {
			return err
		}
		open, err := q.OpenNotificationsForMorning(ctx)
		if err != nil {
			return err
		}
		items := make([]mailItem, 0, len(open))
		for _, n := range open {
			items = append(items, mailItem{kind: n.Kind, name: n.ItemName, unit: n.UnitCode, query: n.QueryText, at: n.OpenedAt.In(loc),
				by: n.OpenedByName, onHand: n.OnHandSeen.String(), min: n.MinSeen.String(), needsCount: n.NeedsCount, key: n.ItemKey})
		}
		if len(items) == 0 {
			return q.FinishNotificationSend(ctx, db.FinishNotificationSendParams{ID: claimed[0], Status: "empty"})
		}
		bodies, err := m.bodies(ctx, q, items, fechaES(now.In(loc))+" · resumen de la mañana", true)
		if err != nil {
			return err
		}
		if len(bodies) == 0 {
			return q.FinishNotificationSend(ctx, db.FinishNotificationSendParams{ID: claimed[0], Status: "empty"})
		}
		jobs = append(jobs, envelope{sendID: claimed[0], subject: fmt.Sprintf("Resumen de la mañana: %d %s", len(items),
			plural(len(items), "aviso abierto", "avisos abiertos")), bodies: bodies})
		return nil
	})
	if err != nil {
		return err
	}
	return m.deliver(ctx, company, jobs)
}

// deliver manda cada correo fuera de la transacción y marca el resultado en otra.
func (m *NotificationMailer) deliver(ctx context.Context, company int64, jobs []envelope) error {
	var errs []error
	for _, j := range jobs {
		// Un destinatario que rebota no les quita el correo a los demás: se sigue con el resto y el
		// bloque cuenta como enviado si llegó a alguien. Reintentar todo le repetiría el correo a
		// quien ya lo recibió.
		var failed error
		sent := 0
		for to, html := range j.bodies {
			if err := m.send([]string{to}, j.subject, html); err != nil {
				failed = err
				continue
			}
			sent++
		}
		if sent > 0 {
			failed = nil
		}
		err := m.store.WithTenant(ctx, company, func(q *db.Queries) error {
			if failed != nil {
				// Sin el texto del error: puede traer el correo al que no se pudo entregar.
				msg := fmt.Sprintf("%T", failed)
				return q.FinishNotificationSend(ctx, db.FinishNotificationSendParams{ID: j.sendID, Status: "failed", Recipients: int16(sent), LastError: &msg})
			}
			if len(j.ids) > 0 {
				if err := q.MarkNotificationsEmailed(ctx, db.MarkNotificationsEmailedParams{State: "sent", SendID: &j.sendID, Ids: j.ids}); err != nil {
					return err
				}
			}
			return q.FinishNotificationSend(ctx, db.FinishNotificationSendParams{ID: j.sendID, Status: "sent", Recipients: int16(sent)})
		})
		if failed != nil {
			// Solo el tipo: el texto del rechazo trae la dirección ajena y esto va a la bitácora.
			errs = append(errs, fmt.Errorf("correo de avisos: falló el envío (%T)", failed))
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// mailItem es un aviso como entra al correo.
type mailItem struct {
	kind, name, unit, query, by, onHand, min string
	key                                      int64 // id del insumo, o el del producto en negativo (notifications.item_key)
	at                                       time.Time
	needsCount                               bool
}

// bodies arma el HTML de cada destinatario con los tipos de aviso que recibe; quien no recibe nada de
// lo que hay, no recibe correo. La lista de compra viene de la misma consulta que la pantalla.
func (m *NotificationMailer) bodies(ctx context.Context, q *db.Queries, items []mailItem, period string, morning bool) (map[string]string, error) {
	lines, err := shoppingListIn(ctx, q)
	if err != nil {
		return nil, err
	}
	buy := buyIndex(lines)
	name, err := q.BusinessNameForEmail(ctx)
	if err != nil {
		return nil, err
	}
	stock, err := recipientsFor(ctx, q, "stock")
	if err != nil {
		return nil, err
	}
	notFound, err := recipientsFor(ctx, q, "not_found")
	if err != nil {
		return nil, err
	}
	group := func(kinds ...string) []mailtpl.Group {
		titles := map[string]string{"stock_out": "Se acabó", "stock_low": "Bajo el mínimo", "not_found": "No lo encuentran"}
		var out []mailtpl.Group
		for _, k := range kinds {
			g := mailtpl.Group{Title: titles[k]}
			for _, it := range items {
				if it.kind != k {
					continue
				}
				g.Items = append(g.Items, m.item(it, buy))
			}
			if len(g.Items) > 0 {
				out = append(out, g)
			}
		}
		return out
	}
	out := map[string]string{}
	render := func(to string, groups []mailtpl.Group) error {
		if len(groups) == 0 {
			return nil
		}
		n := 0
		for _, g := range groups {
			n += len(g.Items)
		}
		title := fmt.Sprintf("%d %s", n, plural(n, "aviso nuevo", "avisos nuevos"))
		if morning {
			title = fmt.Sprintf("%d %s", n, plural(n, "aviso abierto", "avisos abiertos"))
		}
		html, err := mailtpl.RenderStock(mailtpl.StockEmail{BusinessName: name, Period: period, Title: title,
			Subtitle: "Lo que se acabó o está por acabarse, con cuánto conviene comprar", LogoURL: m.baseURL + "/logo192.png",
			OpenURL: m.baseURL + "/almacen?tab=compra", Groups: groups,
			Footer: "Te llega porque recibes los avisos de inventario; quién recibe se cambia en Almacén › Avisos. " +
				"Este correo junta lo nuevo de 5 minutos y el mismo artículo no se repite en 12 horas."})
		if err != nil {
			return err
		}
		out[to] = html
		return nil
	}
	targets := map[string][]string{}
	for _, e := range stock {
		targets[e] = append(targets[e], "stock_out", "stock_low")
	}
	for _, e := range notFound {
		targets[e] = append(targets[e], "not_found")
	}
	for to, kinds := range targets {
		if err := render(to, group(kinds...)); err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		slog.InfoContext(ctx, "notification_mail_no_recipients")
	}
	return out, nil
}

// buyIndex indexa la lista de compra por artículo, con la misma llave que notifications.item_key:
// por nombre, un insumo y un producto que se llaman igual se pisaban.
func buyIndex(lines []ShoppingLine) map[int64]ShoppingLine {
	out := make(map[int64]ShoppingLine, len(lines))
	for _, l := range lines {
		k := l.ID
		if l.ItemType == "producto" {
			k = -l.ID
		}
		out[k] = l
	}
	return out
}

func (m *NotificationMailer) item(it mailItem, buy map[int64]ShoppingLine) mailtpl.Item {
	out := mailtpl.Item{Kind: it.kind, Name: it.name}
	switch it.kind {
	case string(domain.KindStockOut):
		if it.by != "" {
			out.Detail = fmt.Sprintf("Lo reportó %s a las %s", it.by, it.at.Format("15:04"))
		} else {
			out.Detail = fmt.Sprintf("Se acabó a las %s", it.at.Format("15:04"))
		}
	case string(domain.KindStockLow):
		out.Detail = fmt.Sprintf("Hay %s · mínimo %s", humanText(it.onHand, it.unit), humanText(it.min, it.unit))
		if it.needsCount {
			out.Detail += " · conviene contarlo"
		}
	case string(domain.KindNotFound):
		out.Name = "«" + it.query + "»"
		out.Detail = fmt.Sprintf("Lo buscó %s a las %s y no está en el sistema. Dalo de alta o márcalo como otro nombre de un insumo.", it.by, it.at.Format("15:04"))
		return out
	}
	if l, ok := buy[it.key]; ok {
		out.Supplier, out.Why = l.Supplier, l.Why
		if l.Suggested != nil {
			if l.Suggested.Packs > 0 {
				out.Buy = fmt.Sprintf("%d × %s", l.Suggested.Packs, l.Suggested.PackName)
			} else {
				out.Buy = domain.HumanQty(l.Suggested.Qty, l.Unit)
			}
		}
	}
	return out
}

func humanText(qty, unit string) string {
	d, err := decimalFrom(qty)
	if err != nil {
		return qty
	}
	return domain.HumanQty(d, unit)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// SendNotificationsPeriodically corre los bloques y el resumen de cada empresa cada `every`.
func SendNotificationsPeriodically(ctx context.Context, every time.Duration, m *NotificationMailer, companies func(context.Context) ([]int64, error)) {
	// Como el resumen del cierre: una pasada a medio envío termina aunque se apague el ciclo.
	passCtx := context.WithoutCancel(ctx)
	pass := func() {
		ids, err := companies(passCtx)
		if err != nil {
			slog.WarnContext(passCtx, "notification_mail_companies_failed", "error", err.Error())
			return
		}
		for _, id := range ids {
			if err := m.SendWindows(passCtx, id); err != nil {
				slog.WarnContext(passCtx, "notification_mail_failed", "company_id", id, "error", err.Error())
			}
			if err := m.SendMorning(passCtx, id); err != nil {
				slog.WarnContext(passCtx, "notification_morning_failed", "company_id", id, "error", err.Error())
			}
		}
	}
	pass()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pass()
		}
	}
}

// ErrNoEmail: quien pidió la lista no tiene correo capturado.
var ErrNoEmail = fmt.Errorf("%w: tu usuario no tiene correo; pídelo a quien administra el personal", domain.ErrValidation)

// ErrMailDisabled: el servidor no tiene correo configurado.
var ErrMailDisabled = errors.New("el correo no está configurado en este servidor")

// MailSender adapta el envío de un correo a la vez al de una lista. Con el correo apagado responde
// ErrMailDisabled, que la API sabe decir; cualquier otro error era un 500.
func MailSender(enabled func() bool, send func(to, subject, body string) error) func([]string, string, string) error {
	return func(to []string, subject, body string) error {
		if !enabled() {
			return ErrMailDisabled
		}
		var errs []error
		for _, dest := range to {
			errs = append(errs, send(dest, subject, body))
		}
		return errors.Join(errs...)
	}
}

// SendShoppingListTo manda la lista de compra de hoy al correo de quien la pide («Enviarme por
// correo»). Devuelve a dónde la mandó.
func (m *NotificationMailer) SendShoppingListTo(ctx context.Context, company, user int64) (string, error) {
	var to, html string
	err := m.store.WithTenant(ctx, company, func(q *db.Queries) error {
		u, err := q.GetUserByID(ctx, user)
		if err != nil {
			return notFound(err)
		}
		if u.RecoveryEmail == nil || *u.RecoveryEmail == "" {
			return ErrNoEmail
		}
		to = *u.RecoveryEmail
		lines, err := shoppingListIn(ctx, q)
		if err != nil {
			return err
		}
		name, err := q.BusinessNameForEmail(ctx)
		if err != nil {
			return err
		}
		groups := map[string]*mailtpl.Group{}
		var order []string
		for _, l := range lines {
			key := l.Supplier
			if key == "" {
				key = "Sin lugar de compra"
			}
			g, ok := groups[key]
			if !ok {
				g = &mailtpl.Group{Title: key}
				groups[key] = g
				order = append(order, key)
			}
			it := mailtpl.Item{Kind: map[string]string{"out": "stock_out", "low": "stock_low"}[l.State], Name: l.Name, Why: l.Why,
				Detail: "Hay " + domain.HumanQty(l.OnHand, l.Unit)}
			if l.Suggested != nil {
				if l.Suggested.Packs > 0 {
					it.Buy = fmt.Sprintf("%d × %s", l.Suggested.Packs, l.Suggested.PackName)
				} else {
					it.Buy = domain.HumanQty(l.Suggested.Qty, l.Unit)
				}
			}
			g.Items = append(g.Items, it)
		}
		var gs []mailtpl.Group
		for _, k := range order {
			gs = append(gs, *groups[k])
		}
		loc := businessLocation(ctx, q)
		html, err = mailtpl.RenderStock(mailtpl.StockEmail{BusinessName: name, Period: fechaES(m.now().In(loc)) + " · lista de compra",
			Title:    fmt.Sprintf("%d %s por comprar", len(lines), plural(len(lines), "cosa", "cosas")),
			Subtitle: "Agrupada por dónde se compra, con cuánto conviene comprar y por qué", LogoURL: m.baseURL + "/logo192.png",
			OpenURL: m.baseURL + "/almacen?tab=compra", Groups: gs, Footer: "La pediste desde Almacén › Lista de compra."})
		return err
	})
	if err != nil {
		return "", err
	}
	if err := m.send([]string{to}, "Lista de compra", html); err != nil {
		return "", err
	}
	return to, nil
}
