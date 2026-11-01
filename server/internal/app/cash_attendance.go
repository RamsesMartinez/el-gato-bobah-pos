package app

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// cashPerson es la asistencia de una persona al abrir o cerrar caja (spec 046).
type cashPerson struct {
	Attendance domain.CashAttendance
	Name       string
	// EntryID: su entrada abierta; 0 si no tiene.
	EntryID int64
}

// cashAttendanceOf lee la asistencia de `userID` con su fila tomada `for share`. Todo con el MISMO
// `q`: una lectura por otra conexión no la protege el candado, y una entrada o salida que confirma
// en otra tableta se colaría entre la lectura y la decisión.
func cashAttendanceOf(ctx context.Context, q *db.Queries, userID int64) (cashPerson, error) {
	u, err := q.LockUserAttendanceForCash(ctx, userID)
	if err != nil {
		// Nunca «no aplica»: una persona que RLS no deja ver es de otra empresa, y tratarla como
		// alguien sin asistencia abriría la puerta que la regla cierra.
		if errors.Is(err, pgx.ErrNoRows) {
			return cashPerson{}, domain.ErrNotFound
		}
		return cashPerson{}, err
	}
	// Sin fila de ajustes la asistencia está apagada, igual que para checar (`Clock`) y para la
	// pantalla de Asistencia (`Roster`): la columna nace en `false` y una empresa sin fila nunca la
	// encendió.
	cfg, err := q.GetAttendanceSettings(ctx)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return cashPerson{}, err
	}
	p := cashPerson{Name: u.Name, Attendance: domain.CashAttendance{
		Enabled: cfg.AttendanceEnabled, Tracks: u.TracksAttendance, Active: u.IsActive,
	}}
	open, err := q.GetOpenAttendance(ctx, userID)
	switch {
	case err == nil:
		since := open.ClockInAt
		p.Attendance.OpenSince, p.EntryID = &since, open.ID
	case !errors.Is(err, pgx.ErrNoRows):
		return cashPerson{}, err
	}
	return p, nil
}

// CashAttendancePerson es la persona de la que depende la acción.
type CashAttendancePerson struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// CashAttendanceView es lo que la hoja de la caja necesita antes de contar. Sin IP, aparato ni
// historial: solo lo que decide qué ofrecer.
type CashAttendanceView struct {
	Stage        string               `json:"stage"`
	Applies      bool                 `json:"applies"`
	Ready        bool                 `json:"ready"`
	Person       CashAttendancePerson `json:"person"`
	OpenSince    *time.Time           `json:"openSince,omitempty"`
	Stale        bool                 `json:"stale"`
	IsSelf       bool                 `json:"isSelf"`
	CanAuthorize bool                 `json:"canAuthorize"`
}

// CashAttendanceState dice si la acción que toca en la caja —abrirla si está cerrada, cerrarla si
// está abierta— necesita algo de asistencia. Sale de las mismas funciones de `domain` que aplican
// abrir y cerrar, así que la hoja nunca promete lo que el servidor va a rechazar.
func (s *BackofficeService) CashAttendanceState(ctx context.Context, registerID, userID int64, perms []domain.Permission) (*CashAttendanceView, error) {
	var view *CashAttendanceView
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		if _, err := q.GetCashRegister(ctx, registerID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		stage, personID := "open", userID
		sess, err := q.GetOpenSessionByRegister(ctx, registerID)
		switch {
		case err == nil:
			stage, personID = "close", sess.OpenedBy
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		p, err := cashAttendanceOf(ctx, q, personID)
		if err != nil {
			return err
		}
		now := s.now()
		var g domain.CashGate
		if stage == "open" {
			g = domain.CashOpenAttendance(p.Attendance, now)
		} else {
			g = domain.CashCloseGate(domain.CashCloseInput{Opener: p.Attendance, OpenerID: personID,
				OpenerName: p.Name, CloserID: userID, Perms: perms}, now)
		}
		view = &CashAttendanceView{Stage: stage, Applies: g.Applies, Ready: g.Ready, Stale: g.Stale,
			IsSelf: g.IsSelf, CanAuthorize: g.CanAuthorize, OpenSince: p.Attendance.OpenSince,
			Person: CashAttendancePerson{ID: personID, Name: p.Name, Active: p.Attendance.Active}}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}
