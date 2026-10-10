package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// TipsService entrega propinas desde la caja (spec 032, puntos 1 y 2 de las decisiones del dueño).
type TipsService struct {
	store *store.Store
	now   func() time.Time
}

// NewTipsService construye el servicio.
func NewTipsService(s *store.Store, now func() time.Time) *TipsService {
	return &TipsService{store: s, now: now}
}

// TipsPendingView es lo que falta por entregar en el turno abierto de una caja.
type TipsPendingView struct {
	Total     decimal.Decimal `json:"total"`
	Cash      decimal.Decimal `json:"cash"`
	NonCash   decimal.Decimal `json:"nonCash"`
	Inherited decimal.Decimal `json:"inherited"`
	// People: a quién se le puede entregar. Solo id y nombre: quien cobra no lee la lista de usuarios.
	People []TipPerson `json:"people"`
}

// TipPerson es una persona activa del negocio que puede recibir propina.
type TipPerson struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// TipRecipient es una persona del reparto. Amount solo en modo ajustado.
type TipRecipient struct {
	UserID int64            `json:"userId"`
	Amount *decimal.Decimal `json:"amount"`
}

// TipPayoutCmd: repartir la propina pendiente entre una o varias personas.
type TipPayoutCmd struct {
	Mode       string         `json:"mode"` // "parejo" | "ajustado"
	Recipients []TipRecipient `json:"recipients"`
	ActorID    int64          `json:"-"`
}

// TipPayoutView es un movimiento de entrega, uno por persona.
type TipPayoutView struct {
	MovementID    int64           `json:"movementId"`
	RecipientID   int64           `json:"recipientId"`
	RecipientName string          `json:"recipientName"`
	Amount        decimal.Decimal `json:"amount"`
}

// tipSources arma las fuentes del pendiente del turno. Lo devuelto de un pedido se descuenta de sus
// cobros en orden; lo que exceda lo que queda (porque ya se entregó) no deja la fuente en negativo.
func tipSources(ctx context.Context, q *db.Queries, sessionID int64) ([]domain.TipSource, error) {
	pays, err := q.TipPaymentsForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	refunds, err := q.RefundedTipsByOrderForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	refunded := map[int64]decimal.Decimal{}
	for _, r := range refunds {
		refunded[r.OrderID] = r.RefundedTips
	}
	inh, err := q.InheritedTipsForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := []domain.TipSource{{Inherited: true, Available: decimal.Max(decimal.Zero, inh.CarriedIn.Sub(inh.Used))}}
	for _, p := range pays {
		avail := p.TipAmount.Sub(p.PaidOut)
		if r := refunded[p.OrderID]; r.IsPositive() {
			take := decimal.Min(r, p.TipAmount)
			refunded[p.OrderID] = r.Sub(take)
			avail = avail.Sub(take)
		}
		out = append(out, domain.TipSource{PaymentID: p.ID, Cash: p.IsCash, Available: decimal.Max(decimal.Zero, avail)})
	}
	return out, nil
}

func pendingView(src []domain.TipSource) TipsPendingView {
	v := TipsPendingView{People: []TipPerson{}}
	for _, s := range src {
		switch {
		case s.Inherited:
			v.Inherited = v.Inherited.Add(s.Available)
		case s.Cash:
			v.Cash = v.Cash.Add(s.Available)
		default:
			v.NonCash = v.NonCash.Add(s.Available)
		}
	}
	v.Inherited, v.Cash, v.NonCash = domain.Round2(v.Inherited), domain.Round2(v.Cash), domain.Round2(v.NonCash)
	v.Total = domain.PendingTips(src)
	return v
}

func (s *TipsService) openSession(ctx context.Context, q *db.Queries, registerID int64) (db.RegisterSession, error) {
	sess, err := q.GetOpenSessionByRegister(ctx, registerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return sess, fmt.Errorf("%w: la caja no tiene turno abierto", domain.ErrConflict)
	}
	return sess, err
}

// Pending devuelve la propina por entregar del turno abierto de la caja.
func (s *TipsService) Pending(ctx context.Context, registerID int64) (TipsPendingView, error) {
	q := s.store.QC(ctx)
	sess, err := s.openSession(ctx, q, registerID)
	if err != nil {
		return TipsPendingView{}, err
	}
	src, err := tipSources(ctx, q, sess.ID)
	if err != nil {
		return TipsPendingView{}, err
	}
	v := pendingView(src)
	users, err := q.ListActiveUsers(ctx)
	if err != nil {
		return TipsPendingView{}, err
	}
	v.People = make([]TipPerson, 0, len(users))
	for _, u := range users {
		v.People = append(v.People, TipPerson{ID: u.ID, Name: u.Name})
	}
	return v, nil
}

// Payout reparte la propina pendiente: un movimiento de caja tipo propina por persona, ligado a los
// cobros de los que sale. Todo o nada.
func (s *TipsService) Payout(ctx context.Context, registerID int64, cmd TipPayoutCmd) ([]TipPayoutView, error) {
	if cmd.Mode != "parejo" && cmd.Mode != "ajustado" {
		return nil, fmt.Errorf("%w: modo de reparto desconocido", domain.ErrValidation)
	}
	if len(cmd.Recipients) > domain.MaxTipRecipients {
		return nil, domain.ErrTooManyRecipients
	}
	out := []TipPayoutView{}
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		sess, err := s.openSession(ctx, q, registerID)
		if err != nil {
			return err
		}
		if _, err := q.LockSessionForTips(ctx, sess.ID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("%w: la caja se cerró mientras se repartía", domain.ErrConflict)
			}
			return err
		}
		src, err := tipSources(ctx, q, sess.ID)
		if err != nil {
			return err
		}
		pending := domain.PendingTips(src)

		lines := make([]domain.PayoutLine, 0, len(cmd.Recipients))
		if cmd.Mode == "parejo" {
			seen := map[int64]bool{}
			for _, r := range cmd.Recipients {
				if seen[r.UserID] {
					return domain.ErrDuplicateRecipient
				}
				seen[r.UserID] = true
			}
			each, _, err := domain.SplitEven(pending, len(cmd.Recipients))
			if err != nil {
				return err
			}
			for _, r := range cmd.Recipients {
				lines = append(lines, domain.PayoutLine{UserID: r.UserID, Amount: each})
			}
		} else {
			for _, r := range cmd.Recipients {
				if r.Amount == nil {
					return fmt.Errorf("%w: falta el monto de una persona", domain.ErrValidation)
				}
				lines = append(lines, domain.PayoutLine{UserID: r.UserID, Amount: *r.Amount})
			}
		}
		if err := domain.ValidatePayout(pending, lines); err != nil {
			return err
		}

		actor, err := q.GetUserByID(ctx, cmd.ActorID)
		if err != nil {
			return err
		}
		for _, l := range lines {
			u, err := q.GetUserByID(ctx, l.UserID)
			// Misma empresa que quien entrega: la FK compuesta lo haría fallar como 500, y el owner
			// (sin RLS) sí vería al usuario ajeno.
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!u.IsActive || u.CompanyID != actor.CompanyID)) {
				return fmt.Errorf("%w: esa persona no está activa en el negocio", domain.ErrValidation)
			}
			if err != nil {
				return err
			}
			alloc, err := domain.AllocateTipSources(src, l.Amount)
			if err != nil {
				return err
			}
			name := u.Name
			mid, err := q.InsertTipMovement(ctx, db.InsertTipMovementParams{
				SessionID: sess.ID, Amount: l.Amount, Concept: "Propina a " + name, UserID: cmd.ActorID,
				RecipientUserID: &u.ID, RecipientName: &name,
			})
			if err != nil {
				return err
			}
			for _, a := range alloc {
				var pid *int64
				if a.PaymentID != 0 {
					pid = new(a.PaymentID)
				}
				if err := q.InsertTipSource(ctx, db.InsertTipSourceParams{MovementID: mid, Amount: a.Amount, OrderPaymentID: pid}); err != nil {
					return err
				}
				// La fuente usada ya no está disponible para la siguiente persona del reparto.
				for i := range src {
					if src[i].PaymentID == a.PaymentID {
						src[i].Available = src[i].Available.Sub(a.Amount)
					}
				}
			}
			out = append(out, TipPayoutView{MovementID: mid, RecipientID: u.ID, RecipientName: name, Amount: l.Amount})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
