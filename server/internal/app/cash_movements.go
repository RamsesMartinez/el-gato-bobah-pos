package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
	"github.com/shopspring/decimal"
)

// CashMovementService registra lo que entra y sale del cajón durante el turno: entradas, salidas,
// su corrección y los traspasos entre cajas (spec 082, fase 2).
type CashMovementService struct{ cashDeps }

func (s *CashMovementService) RecordCashMovement(ctx context.Context, registerID int64, cmd CashMovementCmd) (*SessionView, error) {
	if !domain.ValidCashKind(cmd.Kind) {
		return nil, domain.ErrValidation
	}
	amt := domain.Round2(cmd.Amount)
	if !domain.ValidMoney(amt, false) {
		return nil, domain.ErrValidation
	}
	reg, err := s.store.QC(ctx).GetCashRegister(ctx, registerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	sess, err := s.store.QC(ctx).GetOpenSessionByRegister(ctx, registerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound // sin caja abierta no hay dónde registrar
		}
		return nil, err
	}
	// Con el turno bloqueado en compartido, como un cobro: un cierre que corre a la vez termina antes
	// —y aquí se ve cerrado— o espera a que este movimiento confirme y lo cuenta (spec 031, D8).
	if err := s.store.WithTx(ctx, func(q *db.Queries) error {
		concept, err := cashMovementConcept(ctx, q, cmd)
		if err != nil {
			return err
		}
		if _, err := q.LockOpenSessionForShare(ctx, sess.ID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if cmd.Kind == domain.CashSalida {
			_, err = q.InsertCashOut(ctx, db.InsertCashOutParams{SessionID: sess.ID, Amount: amt, Concept: concept, UserID: cmd.UserID, ConceptID: cmd.ConceptID})
		} else {
			_, err = q.InsertCashMovement(ctx, db.InsertCashMovementParams{
				SessionID: sess.ID, Kind: cmd.Kind, Amount: amt, Concept: concept, UserID: cmd.UserID,
			})
		}
		if err != nil {
			return err
		}
		return auditCash(ctx, q, "cash.movement", sess.ID, &cmd.UserID, nil,
			map[string]any{"kind": cmd.Kind, "amount": amt, "reason": concept})
	}); err != nil {
		return nil, err
	}
	return s.vistaDelTurnoAbierto(ctx, sess, reg)
}

// CorrectCashOut corrige una salida sin editarla (spec 032, punto 4): crea su reverso y una salida
// nueva bien capturada, los dos en el turno ABIERTO de la caja de la original —un corte ya cerrado
// no se reescribe—. Todo o nada.
func (s *CashMovementService) CorrectCashOut(ctx context.Context, movementID int64, cmd CashMovementCmd) (*SessionView, error) {
	amt := domain.Round2(cmd.Amount)
	if !domain.ValidMoney(amt, false) {
		return nil, domain.ErrValidation
	}
	cmd.Kind = domain.CashSalida
	var sess db.RegisterSession
	var registerID int64
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		m, err := q.GetMovementForCorrection(ctx, movementID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := domain.CanReverse(domain.ReversibleMovement{Kind: m.Kind, IsExpense: m.IsExpense, IsTransfer: m.IsTransfer,
			IsRefund: m.IsRefund, AlreadyReversed: m.AlreadyReversed}); err != nil {
			return err
		}
		registerID = m.RegisterID
		concept, err := cashMovementConcept(ctx, q, cmd)
		if err != nil {
			return err
		}
		sess, err = q.GetOpenSessionByRegister(ctx, m.RegisterID)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: abre la caja para corregir", domain.ErrConflict)
		}
		if err != nil {
			return err
		}
		if _, err := q.LockOpenSessionForShare(ctx, sess.ID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrConflict
			}
			return err
		}
		if _, err := q.InsertCashReversal(ctx, db.InsertCashReversalParams{SessionID: sess.ID, Amount: m.Amount,
			Concept: "Corrección: " + m.Concept, UserID: cmd.UserID, ReversesID: &m.ID}); err != nil {
			if isUniqueViolation(err) {
				return domain.ErrAlreadyReversed
			}
			return err
		}
		if _, err = q.InsertCashOut(ctx, db.InsertCashOutParams{SessionID: sess.ID, Amount: amt, Concept: concept, UserID: cmd.UserID, ConceptID: cmd.ConceptID}); err != nil {
			return err
		}
		return auditCash(ctx, q, "cash.movement_corrected", sess.ID, &cmd.UserID,
			map[string]any{"amount": m.Amount, "reason": m.Concept}, map[string]any{"amount": amt, "reason": concept})
	})
	if err != nil {
		return nil, err
	}
	reg, err := s.store.QC(ctx).GetCashRegister(ctx, registerID)
	if err != nil {
		return nil, err
	}
	return s.vistaDelTurnoAbierto(ctx, sess, reg)
}

// Transfer mueve efectivo de una caja abierta a otra: registra el traspaso y genera, en la MISMA
// tx, la salida en origen + la entrada en destino, ambas ligadas al traspaso → las dos cajas lo
// reflejan de forma atómica ("lo detectan"). Exige ambas cajas abiertas y misma moneda (un
// traspaso no convierte divisa). Devuelve el id del traspaso.
func (s *CashMovementService) Transfer(ctx context.Context, fromRegisterID, toRegisterID int64, amount decimal.Decimal, note string, userID int64) (int64, error) {
	amt := domain.Round2(amount)
	if !domain.ValidTransfer(fromRegisterID, toRegisterID, amt) {
		return 0, domain.ErrValidation
	}
	from, err := s.store.QC(ctx).GetCashRegister(ctx, fromRegisterID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrNotFound
		}
		return 0, err
	}
	to, err := s.store.QC(ctx).GetCashRegister(ctx, toRegisterID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrNotFound
		}
		return 0, err
	}
	fromSess, err := s.openSessionOrConflict(ctx, fromRegisterID)
	if err != nil {
		return 0, err
	}
	toSess, err := s.openSessionOrConflict(ctx, toRegisterID)
	if err != nil {
		return 0, err
	}
	if fromSess.Currency != toSess.Currency {
		return 0, domain.ErrValidation
	}
	var transferID int64
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		// Las dos cajas bloqueadas en compartido, en orden de id para que dos traspasos cruzados no
		// se esperen entre sí: una pierna escrita en un turno que se cerró mientras tanto queda fuera
		// del esperado firmado (spec 031, D8).
		ids := []int64{fromSess.ID, toSess.ID}
		if ids[1] < ids[0] {
			ids[0], ids[1] = ids[1], ids[0]
		}
		for _, id := range ids {
			if _, err := q.LockOpenSessionForShare(ctx, id); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return domain.ErrConflict
				}
				return err
			}
		}
		var e error
		transferID, e = q.CreateCashTransfer(ctx, db.CreateCashTransferParams{
			FromSessionID: fromSess.ID, ToSessionID: toSess.ID, Amount: amt, Note: strPtr(note), CreatedBy: userID,
		})
		if e != nil {
			return e
		}
		// Concepto con el nombre de la contraparte al momento del traspaso (snapshot de auditoría:
		// si luego renombran la caja, el histórico conserva cómo se llamaba).
		if e = q.InsertTransferMovement(ctx, db.InsertTransferMovementParams{
			SessionID: fromSess.ID, Kind: domain.CashSalida, Amount: amt,
			Concept: "Traspaso a " + to.Name, UserID: userID, TransferID: &transferID,
		}); e != nil {
			return e
		}
		if e = q.InsertTransferMovement(ctx, db.InsertTransferMovementParams{
			SessionID: toSess.ID, Kind: domain.CashEntrada, Amount: amt,
			Concept: "Traspaso desde " + from.Name, UserID: userID, TransferID: &transferID,
		}); e != nil {
			return e
		}
		return auditCash(ctx, q, "cash.movement", fromSess.ID, &userID, nil,
			map[string]any{"kind": "traspaso", "amount": amt, "reason": "Traspaso a " + to.Name})
	})
	if err != nil {
		return 0, err
	}
	return transferID, nil
}
