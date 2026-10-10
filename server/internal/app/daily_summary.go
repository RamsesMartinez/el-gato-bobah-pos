package app

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// SendMail manda un correo. Es una función y no una interfaz: hay un solo cartero (internal/mailer)
// y las pruebas pasan uno de mentiras.
type SendMail func(to []string, subject, htmlBody string) error

// DailySummaryService manda al dueño el resumen del cierre de cada día (spec 032, punto 7).
type DailySummaryService struct {
	store *store.Store
	now   func() time.Time
	send  SendMail
}

// NewDailySummaryService construye el servicio.
func NewDailySummaryService(s *store.Store, now func() time.Time, send SendMail) *DailySummaryService {
	if now == nil {
		now = time.Now
	}
	return &DailySummaryService{store: s, now: now, send: send}
}

// Emails devuelve los correos del resumen.
func (s *DailySummaryService) Emails(ctx context.Context) ([]string, error) {
	e, err := s.store.QC(ctx).GetSummaryEmails(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return []string{}, nil
	}
	if e == nil {
		e = []string{}
	}
	return e, err
}

// SetEmails guarda los correos, validados (decisión del dueño del 2026-10-09).
func (s *DailySummaryService) SetEmails(ctx context.Context, emails []string) error {
	clean, err := domain.NormalizeSummaryEmails(emails)
	if err != nil {
		return err
	}
	_, err = s.store.QC(ctx).SetSummaryEmails(ctx, clean)
	return err
}

// summaryLookback: hasta cuántos días atrás se busca un cierre sin resumen. Cubre un servidor que
// estuvo apagado un fin de semana sin mandar resúmenes de hace meses.
const summaryLookback = 7

// SendPending manda el resumen de cada día ya cerrado de UNA empresa que no lo tenga. Un fallo del
// correo se registra y se reintenta en la siguiente pasada; nunca afecta al cierre.
func (s *DailySummaryService) SendPending(ctx context.Context, companyID int64) error {
	var emails []string
	var dias []pgtype.Date
	since := s.now().AddDate(0, 0, -summaryLookback)
	if err := s.store.WithTenant(ctx, companyID, func(q *db.Queries) error {
		e, err := q.GetSummaryEmails(ctx)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		emails = e
		if len(emails) == 0 {
			return nil
		}
		dias, err = q.DaysReadyForSummary(ctx, pgtype.Date{Time: since, Valid: true})
		return err
	}); err != nil {
		return err
	}
	if len(emails) == 0 {
		if len(dias) > 0 {
			slog.InfoContext(ctx, "daily_summary_no_recipients", "company_id", companyID)
		}
		return nil
	}
	for _, d := range dias {
		if err := s.sendDay(ctx, companyID, emails, d); err != nil {
			slog.WarnContext(ctx, "daily_summary_failed", "company_id", companyID, "business_date", d.Time.Format("2006-01-02"), "error", err.Error())
		}
	}
	return nil
}

func (s *DailySummaryService) sendDay(ctx context.Context, companyID int64, emails []string, day pgtype.Date) error {
	var id int64
	var money db.DaySummaryMoneyRow
	var salidas []db.DaySummaryCashOutsRow
	err := s.store.WithTenant(ctx, companyID, func(q *db.Queries) error {
		var err error
		if id, err = q.ClaimSummarySend(ctx, day); err != nil {
			return err
		}
		if money, err = q.DaySummaryMoney(ctx, day); err != nil {
			return err
		}
		salidas, err = q.DaySummaryCashOuts(ctx, day)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // otra pasada ya lo tomó o ya se mandó
	}
	if err != nil {
		return err
	}
	subject := "Resumen del cierre del " + day.Time.Format("02/01/2006")
	sendErr := s.send(emails, subject, summaryHTML(day.Time, money, salidas))
	// La marca va en su propia transacción y se confirma aunque el envío haya fallado: devolver el
	// error del correo desde aquí desharía la marca y el día quedaría «pendiente» sin reintento.
	if err := s.store.WithTenant(ctx, companyID, func(q *db.Queries) error {
		if sendErr != nil {
			return q.MarkSummaryFailed(ctx, db.MarkSummaryFailedParams{ID: id, Left: sendErr.Error()})
		}
		return q.MarkSummarySent(ctx, id)
	}); err != nil {
		return err
	}
	return sendErr
}

// summaryHTML arma el cuerpo. Sin datos de clientes: solo cifras del día y conceptos de salida.
func summaryHTML(day time.Time, m db.DaySummaryMoneyRow, salidas []db.DaySummaryCashOutsRow) string {
	var b strings.Builder
	row := func(label, value string) {
		fmt.Fprintf(&b, "<tr><td>%s</td><td style=\"text-align:right\">%s</td></tr>", html.EscapeString(label), html.EscapeString(value))
	}
	fmt.Fprintf(&b, "<h2>Cierre del %s</h2><table>", day.Format("02/01/2006"))
	row("Turnos cerrados", fmt.Sprint(m.Shifts))
	row("Ventas", "$"+m.Sales.StringFixed(2))
	row("Propinas cobradas", "$"+m.Tips.StringFixed(2))
	row("Propinas entregadas", "$"+m.TipsPaidOut.StringFixed(2))
	row("Propinas por entregar", "$"+m.TipsPending.StringFixed(2))
	row("Diferencia del cajón", "$"+m.DrawerDifference.StringFixed(2))
	row("Diferencia de terminales", "$"+m.TerminalDifference.StringFixed(2))
	row("Salidas sin concepto", fmt.Sprint(m.CashOutsWithoutConcept))
	b.WriteString("</table><h3>Salidas por concepto</h3><table>")
	for _, s := range salidas {
		row(s.Concept, "$"+s.Total.StringFixed(2))
	}
	if len(salidas) == 0 {
		row("Sin salidas", "")
	}
	b.WriteString("</table><p>Las propinas no son ventas ni gastos: se muestran aparte.</p>")
	return b.String()
}

// SendSummariesPeriodically corre SendPending para cada empresa cada `every`. La lista de empresas
// la da quien llama (la conexión de plataforma, que es la única que las ve todas).
func SendSummariesPeriodically(ctx context.Context, every time.Duration, svc *DailySummaryService, companies func(context.Context) ([]int64, error)) {
	pass := func() {
		ids, err := companies(ctx)
		if err != nil {
			slog.WarnContext(ctx, "daily_summary_companies_failed", "error", err.Error())
			return
		}
		for _, id := range ids {
			if err := svc.SendPending(ctx, id); err != nil {
				slog.WarnContext(ctx, "daily_summary_failed", "company_id", id, "error", err.Error())
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
