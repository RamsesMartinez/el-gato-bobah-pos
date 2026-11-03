package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// TerminalsService administra las terminales de tarjeta por sucursal (spec 032, puntos 8 y 9).
type TerminalsService struct{ store *store.Store }

// NewTerminalsService construye el servicio.
func NewTerminalsService(s *store.Store) *TerminalsService { return &TerminalsService{store: s} }

// TerminalView es una terminal en Configuración y al cobrar.
type TerminalView struct {
	ID         int64  `json:"id"`
	BranchID   int64  `json:"branchId"`
	BranchName string `json:"branchName"`
	Name       string `json:"name"`
	Archived   bool   `json:"archived"`
}

func terminalName(s string) (string, error) {
	n := strings.Join(strings.Fields(s), " ")
	if n == "" || len([]rune(n)) > 40 {
		return "", fmt.Errorf("%w: escribe el nombre de la terminal (hasta 40 letras)", domain.ErrValidation)
	}
	return n, nil
}

// List devuelve las terminales de la empresa, activas primero.
func (s *TerminalsService) List(ctx context.Context) ([]TerminalView, error) {
	rows, err := s.store.QC(ctx).ListCardTerminals(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TerminalView, 0, len(rows))
	for _, r := range rows {
		out = append(out, TerminalView{ID: r.ID, BranchID: r.BranchID, BranchName: r.BranchName, Name: r.Name, Archived: r.Archived})
	}
	return out, nil
}

// Create agrega una terminal a una sucursal.
func (s *TerminalsService) Create(ctx context.Context, branchID int64, name string) (TerminalView, error) {
	n, err := terminalName(name)
	if err != nil {
		return TerminalView{}, err
	}
	t, err := s.store.QC(ctx).InsertCardTerminal(ctx, db.InsertCardTerminalParams{BranchID: branchID, Name: n})
	switch {
	case isUniqueViolation(err):
		return TerminalView{}, fmt.Errorf("%w: ya hay una terminal con ese nombre en la sucursal", domain.ErrConflict)
	case isFKViolation(err):
		return TerminalView{}, fmt.Errorf("%w: esa sucursal no existe", domain.ErrValidation)
	case err != nil:
		return TerminalView{}, err
	}
	return TerminalView{ID: t.ID, BranchID: t.BranchID, Name: t.Name}, nil
}

// Rename cambia el nombre; los cobros pasados conservan el que tenían.
func (s *TerminalsService) Rename(ctx context.Context, id int64, name string) (TerminalView, error) {
	n, err := terminalName(name)
	if err != nil {
		return TerminalView{}, err
	}
	t, err := s.store.QC(ctx).RenameCardTerminal(ctx, db.RenameCardTerminalParams{ID: id, Name: n})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return TerminalView{}, domain.ErrNotFound
	case isUniqueViolation(err):
		return TerminalView{}, fmt.Errorf("%w: ya hay una terminal con ese nombre en la sucursal", domain.ErrConflict)
	case err != nil:
		return TerminalView{}, err
	}
	return TerminalView{ID: t.ID, BranchID: t.BranchID, Name: t.Name}, nil
}

// Archive deja de ofrecer la terminal; sus cobros la conservan.
func (s *TerminalsService) Archive(ctx context.Context, id int64) error {
	n, err := s.store.QC(ctx).ArchiveCardTerminal(ctx, id)
	if err == nil && n == 0 {
		return domain.ErrNotFound
	}
	return err
}

// SetCardCountMode fija cómo se arquea la tarjeta en una sucursal (punto 9). Aplica desde el
// siguiente turno que se abra.
func (s *TerminalsService) SetCardCountMode(ctx context.Context, branchID int64, mode string) error {
	m, err := domain.ParseCardCountMode(mode)
	if err != nil {
		return err
	}
	n, err := s.store.QC(ctx).SetBranchCardCountMode(ctx, db.SetBranchCardCountModeParams{ID: branchID, CardCountMode: string(m)})
	if err == nil && n == 0 {
		return domain.ErrNotFound
	}
	return err
}

// BranchModeView es el modo de arqueo de una sucursal.
type BranchModeView struct {
	BranchID int64  `json:"branchId"`
	Name     string `json:"name"`
	Mode     string `json:"mode"`
}

// CardCountModes lista el modo de arqueo de cada sucursal activa.
func (s *TerminalsService) CardCountModes(ctx context.Context) ([]BranchModeView, error) {
	rows, err := s.store.QC(ctx).ListBranchCardCountModes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]BranchModeView, 0, len(rows))
	for _, r := range rows {
		out = append(out, BranchModeView{BranchID: r.ID, Name: r.Name, Mode: r.CardCountMode})
	}
	return out, nil
}

// resolveTerminal decide la terminal de un cobro con tarjeta: la elegida, si es activa y de la
// sucursal de la caja; si no se eligió, la única activa de la sucursal. Con varias, se pide siempre:
// recordar la del usuario dejó un débito en Terminal A cuando se cobró en Terminal B, y la
// terminal es un hecho físico del momento, no algo que se pueda decidir al configurar (spec 036).
func resolveTerminal(ctx context.Context, q *db.Queries, registerID int64, chosen *int64) (db.ActiveTerminalsOfRegisterBranchRow, error) {
	activas, err := q.ActiveTerminalsOfRegisterBranch(ctx, registerID)
	if err != nil {
		return db.ActiveTerminalsOfRegisterBranchRow{}, err
	}
	find := func(id int64) (db.ActiveTerminalsOfRegisterBranchRow, bool) {
		for _, t := range activas {
			if t.ID == id {
				return t, true
			}
		}
		return db.ActiveTerminalsOfRegisterBranchRow{}, false
	}
	if chosen != nil {
		if t, ok := find(*chosen); ok {
			return t, nil
		}
		return db.ActiveTerminalsOfRegisterBranchRow{}, fmt.Errorf("%w: esa terminal no está disponible en esta sucursal", domain.ErrValidation)
	}
	if len(activas) == 1 {
		return activas[0], nil
	}
	return db.ActiveTerminalsOfRegisterBranchRow{}, domain.ErrCardTerminalRequired
}

// ChangeTerminalCmd corrige en qué terminal se cobró un pago con tarjeta (spec 036).
// FromTerminalID es la terminal que vio la pantalla: nil si el pago no tenía.
type ChangeTerminalCmd struct {
	OrderID, PaymentID, ActorID int64
	FromTerminalID              *int64
	TerminalID                  int64
	Reason                      string
}

// ChangePaymentTerminal mueve un pago con tarjeta a otra terminal activa de la sucursal donde se
// cobró y deja el evento en la bitácora en la misma transacción: sin evento no hay cambio.
//
// No toca el arqueo guardado de un turno cerrado: lo que se firmó se queda como se firmó, y el corte
// compara contra lo esperado de hoy al pintarse (ver terminalCounts).
func (s *OrdersService) ChangePaymentTerminal(ctx context.Context, cmd ChangeTerminalCmd) (*OrderView, error) {
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		// El candado del pedido antes de leer el pago, el mismo de cobrar y devolver: dos cambios a
		// la vez se forman y el segundo ve la terminal que dejó el primero.
		o, err := q.GetOrderForUpdate(ctx, cmd.OrderID)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: Ese pedido no existe", domain.ErrNotFound)
		}
		if err != nil {
			return err
		}
		p, err := q.GetPaymentForTerminalChange(ctx, db.GetPaymentForTerminalChangeParams{ID: cmd.PaymentID, OrderID: cmd.OrderID})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: Ese pago no es de este pedido", domain.ErrNotFound)
		}
		if err != nil {
			return err
		}
		active := []domain.TerminalOption{}
		if p.RegisterID != nil {
			rows, err := q.ActiveTerminalsOfRegisterBranch(ctx, *p.RegisterID)
			if err != nil {
				return err
			}
			for _, r := range rows {
				active = append(active, domain.TerminalOption{ID: r.ID, Name: r.Name})
			}
		}
		to, reason, err := domain.ValidateTerminalChange(domain.TerminalChange{
			MethodKind: p.MethodKind, OrderStatus: string(o.Status), Current: p.CardTerminalID,
			From: cmd.FromTerminalID, To: cmd.TerminalID, Active: active, Reason: cmd.Reason,
		})
		if err != nil {
			return err
		}
		if err := q.SetPaymentTerminal(ctx, db.SetPaymentTerminalParams{ID: p.ID, CardTerminalID: &to.ID, CardTerminalName: &to.Name}); err != nil {
			return err
		}
		side := func(id *int64, name *string) map[string]any {
			return map[string]any{"terminalId": id, "terminalName": name, "amount": p.Amount,
				"sessionId": p.RegisterSessionID, "branchId": p.BranchID}
		}
		after := side(&to.ID, &to.Name)
		after["reason"] = reason
		branch := p.BranchID
		if branch == nil {
			b, err := q.GetOrderBranch(ctx, cmd.OrderID)
			if err != nil {
				return err
			}
			branch = &b
		}
		ev := domain.AuditEvent{Action: "payment.terminal_changed", EntityType: "order_payment", EntityID: p.ID,
			OrderID: &cmd.OrderID, BranchID: branch, Before: side(p.CardTerminalID, p.CardTerminalName), After: after}
		if cmd.ActorID > 0 {
			ev.ActorKind, ev.ActorUserID = domain.ActorUser, &cmd.ActorID
		}
		return recordAudit(ctx, q, ev)
	})
	if err != nil {
		return nil, err
	}
	return s.Detail(ctx, cmd.OrderID)
}
