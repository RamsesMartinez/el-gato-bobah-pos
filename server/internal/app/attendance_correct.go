package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// overlapWindow es cuánto alrededor de un registro se buscan otros con los que pueda encimarse.
const overlapWindow = 48 * time.Hour

// CorrectCmd corrige la entrada o la salida de un registro.
type CorrectCmd struct {
	EntryID int64
	Field   string // "clock_in_at" | "clock_out_at"
	At      time.Time
	Reason  string
}

// ManualEntryCmd captura a mano un turno que nadie checó.
type ManualEntryCmd struct {
	UserID  int64
	In, Out time.Time
	Reason  string
}

// Correct corrige una hora de un registro con motivo. El valor original queda en
// attendance_corrections y el registro se marca corregido.
func (s *AttendanceService) Correct(ctx context.Context, actor int64, perms []domain.Permission, cmd CorrectCmd) error {
	reason, err := domain.ValidateCorrectionReason(cmd.Reason)
	if err != nil {
		return err
	}
	if err := domain.CanCorrectAny(perms); err != nil {
		return err
	}
	if cmd.Field != "clock_in_at" && cmd.Field != "clock_out_at" {
		return fmt.Errorf("%w: solo se corrige la entrada o la salida", domain.ErrValidation)
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		e, err := q.GetAttendanceEntryForUpdate(ctx, cmd.EntryID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := domain.CanCorrect(actor, e.UserID, perms); err != nil {
			return err
		}
		if _, err := q.LockUserForAttendance(ctx, e.UserID); err != nil {
			return err
		}
		in, out := e.ClockInAt, nullableTime(e.ClockOutAt)
		var before *time.Time
		if cmd.Field == "clock_in_at" {
			before, in = &e.ClockInAt, cmd.At
		} else {
			before, out = out, &cmd.At
		}
		if out != nil && !out.After(in) {
			return fmt.Errorf("%w: la salida tiene que ser después de la entrada", domain.ErrValidation)
		}
		if err := s.checkOverlap(ctx, q, e.UserID, e.ID, in, out); err != nil {
			return err
		}
		status := e.Status
		if out != nil {
			status = "closed"
		}
		loc, err := s.location(ctx, q)
		if err != nil {
			return err
		}
		// La corrección va ANTES del cambio: la base solo deja mover una hora si en esta misma
		// transacción ya existe su fila con el valor original.
		if err := q.InsertAttendanceCorrection(ctx, db.InsertAttendanceCorrectionParams{EntryID: e.ID, Field: cmd.Field,
			Before: tsOf(before), After: cmd.At, Reason: reason, CorrectedBy: actor}); err != nil {
			return err
		}
		if err := q.CorrectAttendanceEntry(ctx, db.CorrectAttendanceEntryParams{ID: e.ID, ClockInAt: in,
			ClockOutAt: tsOf(out), Status: status,
			BusinessDate: pgtype.Date{Time: domain.BusinessDateOf(in, loc), Valid: true}}); err != nil {
			return err
		}
		subject := e.UserID
		return recordAudit(ctx, q, domain.AuditEvent{Action: "attendance.corrected", ActorKind: domain.ActorUser,
			ActorUserID: &actor, SubjectUserID: &subject, EntityType: "attendance_entry", EntityID: e.ID,
			Before: map[string]any{cmd.Field: before}, After: map[string]any{cmd.Field: cmd.At, "reason": reason}})
	})
}

// ManualEntry captura a mano un turno completo de alguien que no checó. Queda marcado «a mano».
func (s *AttendanceService) ManualEntry(ctx context.Context, actor int64, perms []domain.Permission, cmd ManualEntryCmd) (int64, error) {
	reason, err := domain.ValidateCorrectionReason(cmd.Reason)
	if err != nil {
		return 0, err
	}
	if !cmd.Out.After(cmd.In) {
		return 0, fmt.Errorf("%w: la salida tiene que ser después de la entrada", domain.ErrValidation)
	}
	if err := domain.CanCorrect(actor, cmd.UserID, perms); err != nil {
		return 0, err
	}
	var id int64
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		if _, err := q.LockUserForAttendance(ctx, cmd.UserID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if err := s.checkOverlap(ctx, q, cmd.UserID, 0, cmd.In, &cmd.Out); err != nil {
			return err
		}
		loc, err := s.location(ctx, q)
		if err != nil {
			return err
		}
		branch, err := singleBranch(ctx, q)
		if err != nil {
			return err
		}
		if id, err = q.InsertAttendanceEntry(ctx, db.InsertAttendanceEntryParams{UserID: cmd.UserID, BranchID: branch,
			BusinessDate: pgtype.Date{Time: domain.BusinessDateOf(cmd.In, loc), Valid: true},
			ClockInAt:    cmd.In, ClockOutAt: tsOf(&cmd.Out), Status: "closed", Source: "manual", CreatedBy: &actor}); err != nil {
			return err
		}
		if err := q.InsertAttendanceCorrection(ctx, db.InsertAttendanceCorrectionParams{EntryID: id, Field: "created",
			After: cmd.In, Reason: reason, CorrectedBy: actor}); err != nil {
			return err
		}
		return recordAudit(ctx, q, domain.AuditEvent{Action: "attendance.manual_entry", ActorKind: domain.ActorUser,
			ActorUserID: &actor, SubjectUserID: &cmd.UserID, EntityType: "attendance_entry", EntityID: id, BranchID: branch,
			After: map[string]any{"in": cmd.In, "out": cmd.Out, "reason": reason}})
	})
	return id, err
}

func (s *AttendanceService) checkOverlap(ctx context.Context, q *db.Queries, userID, skip int64, in time.Time, out *time.Time) error {
	until := in.Add(overlapWindow)
	if out != nil {
		until = out.Add(overlapWindow)
	}
	rows, err := q.ListUserAttendanceNear(ctx, db.ListUserAttendanceNearParams{UserID: userID,
		SinceAt: pgtype.Timestamptz{Time: in.Add(-overlapWindow), Valid: true}, UntilAt: until})
	if err != nil {
		return err
	}
	existing := make([]domain.Interval, 0, len(rows))
	for _, r := range rows {
		iv := domain.Interval{ID: r.ID, In: r.ClockInAt, Out: nullableTime(r.ClockOutAt)}
		// Un registro sin salida que ya quedó «sin salida» no cubre lo que sigue: su fin no se sabe,
		// y bloquear todo lo posterior impediría justo capturar la salida que le falta.
		if r.Status == "missing_out" {
			iv.Out = &iv.In
		}
		existing = append(existing, iv)
	}
	if domain.Overlaps(existing, skip, in, out) {
		return domain.ErrAttendanceOverlap
	}
	return nil
}

func (s *AttendanceService) location(ctx context.Context, q *db.Queries) (*time.Location, error) {
	cfg, err := q.GetAttendanceSettings(ctx)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return domain.LoadBusinessLocation(cfg.Timezone), nil
}

func nullableTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func tsOf(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
