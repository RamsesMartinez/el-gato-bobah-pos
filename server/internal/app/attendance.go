package app

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// AttendanceService registra entradas y salidas del personal (spec 033).
type AttendanceService struct {
	store *store.Store
	now   func() time.Time
}

func NewAttendanceService(s *store.Store, now func() time.Time) *AttendanceService {
	if now == nil {
		now = time.Now
	}
	return &AttendanceService{store: s, now: now}
}

// RosterPerson es una persona en la pantalla de checar. Solo nombre y si está dentro: la pantalla
// se ve desde el mostrador.
type RosterPerson struct {
	UserID    int64      `json:"userId"`
	Name      string     `json:"name"`
	OpenSince *time.Time `json:"openSince,omitempty"`
	HasPin    bool       `json:"hasPin"`
}

// Roster dice si la asistencia está activa y quiénes pueden checar. Una entrada abierta vieja no
// cuenta como «dentro»: checar le abrirá una nueva (domain.NextClock), así que la pantalla tiene
// que ofrecer «Entrada».
func (s *AttendanceService) Roster(ctx context.Context) (enabled bool, people []RosterPerson, err error) {
	q := s.store.QC(ctx)
	cfg, err := q.GetAttendanceSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, []RosterPerson{}, nil
	}
	if err != nil {
		return false, nil, err
	}
	if !cfg.AttendanceEnabled {
		return false, []RosterPerson{}, nil
	}
	rows, err := q.ListAttendanceRoster(ctx)
	if err != nil {
		return false, nil, err
	}
	now := s.now()
	people = make([]RosterPerson, 0, len(rows))
	for _, r := range rows {
		p := RosterPerson{UserID: r.ID, Name: r.Name, HasPin: r.HasPin}
		if r.OpenSince.Valid && !domain.IsStaleOpen(r.OpenSince.Time, now) {
			t := r.OpenSince.Time
			p.OpenSince = &t
		}
		people = append(people, p)
	}
	return true, people, nil
}

// ClockResult es lo que la pantalla confirma: la hora del servidor y, en la salida, lo trabajado.
type ClockResult struct {
	Kind          domain.ClockKind `json:"kind"`
	At            time.Time        `json:"at"`
	WorkedMinutes *int             `json:"workedMinutes,omitempty"`
}

// Clock registra la entrada o la salida de `userID`, que se identifica con su PIN. No emite sesión:
// quien está cobrando en la tableta sigue siendo quien era. El autor del evento es quien checó.
func (s *AttendanceService) Clock(ctx context.Context, userID int64, pin string, expected domain.ClockKind) (*ClockResult, error) {
	var res *ClockResult
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		cfg, err := q.GetAttendanceSettings(ctx)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if !cfg.AttendanceEnabled {
			// Apagada se responde distinto de un PIN equivocado (409, no 401): no hay secreto que
			// proteger, la pantalla ya lo sabe por el roster. Sin bcrypt: no hay nada que igualar.
			return domain.ErrAttendanceDisabled
		}
		u, err := q.LockUserForAttendance(ctx, userID)
		if err != nil {
			auth.CheckDummySecret(pin)
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrInvalidCredentials
			}
			return err
		}
		if !u.IsActive || u.PinHash == nil {
			auth.CheckDummySecret(pin)
			return domain.ErrInvalidCredentials
		}
		if !auth.CheckSecret(*u.PinHash, pin) {
			return domain.ErrInvalidCredentials
		}
		if !u.TracksAttendance {
			return domain.ErrAttendanceNotTracked
		}

		now := s.now()
		var openSince *time.Time
		open, err := q.GetOpenAttendance(ctx, userID)
		switch {
		case err == nil:
			openSince = &open.ClockInAt
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		d, err := domain.NextClock(openSince, now, expected)
		if err != nil {
			return err
		}
		o := OriginFrom(ctx)
		actor := userID
		ev := domain.AuditEvent{ActorKind: domain.ActorUser, ActorUserID: &actor, SubjectUserID: &actor,
			EntityType: "attendance_entry"}

		if d.Kind == domain.ClockOut {
			if _, err := q.CloseAttendanceEntry(ctx, db.CloseAttendanceEntryParams{
				ID: open.ID, ClockOutAt: pgtype.Timestamptz{Time: now, Valid: true},
				OutIp: o.IP, OutDevice: o.Device, OutDeviceKey: o.DeviceKey,
			}); err != nil {
				return err
			}
			worked := int(now.Sub(open.ClockInAt).Minutes())
			res = &ClockResult{Kind: domain.ClockOut, At: now, WorkedMinutes: &worked}
			ev.Action, ev.EntityID = "attendance.clock_out", open.ID
			ev.After = map[string]any{"clockOutAt": now}
			return recordAudit(ctx, q, ev)
		}

		if d.CloseStale {
			if _, err := q.MarkAttendanceMissingOut(ctx, open.ID); err != nil {
				return err
			}
		}
		branch, err := singleBranch(ctx, q)
		if err != nil {
			return err
		}
		loc := domain.LoadBusinessLocation(cfg.Timezone)
		id, err := q.InsertAttendanceEntry(ctx, db.InsertAttendanceEntryParams{
			UserID: userID, BranchID: branch,
			BusinessDate: pgtype.Date{Time: domain.BusinessDateOf(now, loc), Valid: true},
			ClockInAt:    now, Status: "open", Source: "clock",
			InIp: o.IP, InDevice: o.Device, InDeviceKey: o.DeviceKey,
		})
		if err != nil {
			return err
		}
		res = &ClockResult{Kind: domain.ClockIn, At: now}
		ev.Action, ev.EntityID, ev.BranchID = "attendance.clock_in", id, branch
		ev.After = map[string]any{"clockInAt": now}
		if d.CloseStale {
			// La entrada vieja quedó «sin salida» en este mismo paso: el evento lo dice, o ese cambio
			// de estado no dejaría rastro.
			ev.After = map[string]any{"clockInAt": now, "closedAsMissingOut": open.ID}
		}
		return recordAudit(ctx, q, ev)
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// singleBranch devuelve la sucursal de la empresa si tiene una sola; con varias, nula. Adivinar la
// matriz mezclaría la asistencia de dos locales.
func singleBranch(ctx context.Context, q *db.Queries) (*int64, error) {
	ids, err := q.ListActiveBranchIDs(ctx)
	if err != nil {
		return nil, err
	}
	if len(ids) != 1 {
		return nil, nil
	}
	return &ids[0], nil
}
