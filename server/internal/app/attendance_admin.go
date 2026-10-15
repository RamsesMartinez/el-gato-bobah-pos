package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// AttendanceSettingsView es cómo mide asistencia el negocio.
type AttendanceSettingsView struct {
	Enabled         bool `json:"enabled"`
	MeasureSchedule bool `json:"measureSchedule"`
	GraceInMin      int  `json:"graceInMin"`
	GraceOutMin     int  `json:"graceOutMin"`
}

// Settings devuelve los ajustes de asistencia. Sin fila de ajustes, los de fábrica (todo apagado).
func (s *AttendanceService) Settings(ctx context.Context) (AttendanceSettingsView, error) {
	c, err := s.store.QC(ctx).GetAttendanceSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return AttendanceSettingsView{}, nil
	}
	if err != nil {
		return AttendanceSettingsView{}, err
	}
	return AttendanceSettingsView{Enabled: c.AttendanceEnabled, MeasureSchedule: c.AttendanceMeasureSchedule,
		GraceInMin: int(c.AttendanceGraceInMin), GraceOutMin: int(c.AttendanceGraceOutMin)}, nil
}

// SetSettings guarda los ajustes. Cambiar la tolerancia cambia también los reportes de días
// pasados: el retardo se calcula con el ajuste vigente (decisión del plan, puertas).
func (s *AttendanceService) SetSettings(ctx context.Context, actor int64, v AttendanceSettingsView) error {
	if v.GraceInMin < 0 || v.GraceInMin > 60 || v.GraceOutMin < 0 || v.GraceOutMin > 60 {
		return fmt.Errorf("%w: la tolerancia va de 0 a 60 minutos", domain.ErrValidation)
	}
	before, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		n, err := q.SetAttendanceSettings(ctx, db.SetAttendanceSettingsParams{Enabled: v.Enabled, Measure: v.MeasureSchedule,
			GraceIn: int16(v.GraceInMin), GraceOut: int16(v.GraceOutMin)})
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("%w: primero captura los datos del negocio", domain.ErrValidation)
		}
		return auditEntity(ctx, q, "attendance.settings_changed", "business_settings", 0, nil, &actor, before, v)
	})
}

// SetTracksAttendance prende o apaga «Lleva asistencia» de una persona.
func (s *AttendanceService) SetTracksAttendance(ctx context.Context, actor, userID int64, tracks bool) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		before, err := q.GetUserTracksAttendance(ctx, userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := q.SetUserTracksAttendance(ctx, db.SetUserTracksAttendanceParams{ID: userID, TracksAttendance: tracks}); err != nil {
			return err
		}
		return recordAudit(ctx, q, domain.AuditEvent{Action: "user.tracks_attendance_changed", ActorKind: domain.ActorUser,
			ActorUserID: &actor, SubjectUserID: &userID, EntityType: "user", EntityID: userID,
			Before: map[string]any{"tracksAttendance": before}, After: map[string]any{"tracksAttendance": tracks}})
	})
}

// ScheduleShiftView es un turno en minutos desde la medianoche.
type ScheduleShiftView struct {
	Weekday int `json:"weekday"`
	ShiftNo int `json:"shiftNo"`
	Start   int `json:"start"`
	End     int `json:"end"`
}

// ScheduleVersionView es una versión del horario de una persona.
type ScheduleVersionView struct {
	EffectiveFrom string              `json:"effectiveFrom"`
	Days          []ScheduleShiftView `json:"days"`
}

// TracksAttendance dice si a la persona se le mide asistencia.
func (s *AttendanceService) TracksAttendance(ctx context.Context, userID int64) (bool, error) {
	v, err := s.store.QC(ctx).GetUserTracksAttendance(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, domain.ErrNotFound
	}
	return v, err
}

// Schedules devuelve las versiones del horario de una persona, la más reciente primero.
func (s *AttendanceService) Schedules(ctx context.Context, userID int64) ([]ScheduleVersionView, error) {
	rows, err := s.store.QC(ctx).ListScheduleDays(ctx, &userID)
	if err != nil {
		return nil, err
	}
	out := []ScheduleVersionView{}
	idx := map[int64]int{}
	for _, r := range rows {
		i, ok := idx[r.VersionID]
		if !ok {
			out = append(out, ScheduleVersionView{EffectiveFrom: r.EffectiveFrom.Time.Format(time.DateOnly), Days: []ScheduleShiftView{}})
			i = len(out) - 1
			idx[r.VersionID] = i
		}
		if r.Weekday != nil {
			out[i].Days = append(out[i].Days, ScheduleShiftView{Weekday: int(*r.Weekday), ShiftNo: int(*r.ShiftNo),
				Start: minutesOf(r.StartTime), End: minutesOf(r.EndTime)})
		}
	}
	for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
		out[l], out[r] = out[r], out[l]
	}
	return out, nil
}

func minutesOf(t pgtype.Time) int { return int(t.Microseconds / 60_000_000) }

func timeOf(min int) pgtype.Time {
	return pgtype.Time{Microseconds: int64(min) * 60_000_000, Valid: true}
}

// SetSchedule guarda una versión nueva del horario de una persona: todos sus días a la vez, en una
// transacción. Nunca se agregan días a una versión existente, que reescribiría el pasado.
func (s *AttendanceService) SetSchedule(ctx context.Context, actor, userID int64, effectiveFrom time.Time, days []ScheduleShiftView) error {
	in := make([]domain.ScheduleDay, len(days))
	for i, d := range days {
		in[i] = domain.ScheduleDay{Weekday: d.Weekday, ShiftNo: d.ShiftNo, Start: d.Start, End: d.End}
	}
	if err := domain.ValidateSchedule(in); err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if _, err := q.LockUserForAttendance(ctx, userID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		v, err := q.InsertScheduleVersion(ctx, db.InsertScheduleVersionParams{UserID: userID,
			EffectiveFrom: pgtype.Date{Time: effectiveFrom, Valid: true}, CreatedBy: actor})
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: ya hay un horario que empieza ese día; elige otra fecha", domain.ErrConflict)
		}
		if err != nil {
			return err
		}
		for _, d := range days {
			if err := q.InsertScheduleDay(ctx, db.InsertScheduleDayParams{VersionID: v, Weekday: int16(d.Weekday),
				ShiftNo: int16(d.ShiftNo), StartTime: timeOf(d.Start), EndTime: timeOf(d.End)}); err != nil {
				return err
			}
		}
		return recordAudit(ctx, q, domain.AuditEvent{Action: "attendance.schedule_set", ActorKind: domain.ActorUser,
			ActorUserID: &actor, SubjectUserID: &userID, EntityType: "work_schedule_version", EntityID: v,
			After: map[string]any{"effectiveFrom": effectiveFrom.Format(time.DateOnly), "days": days}})
	})
}

// ReportEntry es un registro en el reporte, con su origen.
type ReportEntry struct {
	ID           int64           `json:"id"`
	In           time.Time       `json:"in"`
	Out          *time.Time      `json:"out,omitempty"`
	Status       string          `json:"status"`
	Source       string          `json:"source"`
	Corrected    bool            `json:"corrected"`
	InIP         string          `json:"inIp"`
	OutIP        string          `json:"outIp"`
	InDevice     json.RawMessage `json:"inDevice,omitempty"`
	OutDevice    json.RawMessage `json:"outDevice,omitempty"`
	InDeviceKey  string          `json:"inDeviceKey"`
	OutDeviceKey string          `json:"outDeviceKey"`
}

// ReportDay es el día de una persona.
type ReportDay struct {
	Date            string        `json:"date"`
	Entries         []ReportEntry `json:"entries"`
	WorkedMinutes   int           `json:"workedMinutes"`
	LateMinutes     *int          `json:"lateMinutes,omitempty"`
	OvertimeMinutes *int          `json:"overtimeMinutes,omitempty"`
	MissingOut      bool          `json:"missingOut"`
	NoSchedule      bool          `json:"noSchedule"`
}

// ReportPerson es una persona con sus días y totales.
type ReportPerson struct {
	UserID          int64       `json:"userId"`
	Name            string      `json:"name"`
	Days            []ReportDay `json:"days"`
	WorkedMinutes   int         `json:"workedMinutes"`
	LateMinutes     *int        `json:"lateMinutes,omitempty"`
	OvertimeMinutes *int        `json:"overtimeMinutes,omitempty"`
	MissingOutDays  int         `json:"missingOutDays"`
}

// AttendanceReport es el reporte de un rango. Las tres vistas de la pantalla salen de aquí.
type AttendanceReport struct {
	MeasureSchedule bool           `json:"measureSchedule"`
	People          []ReportPerson `json:"people"`
}

// maxReportDays acota el rango del reporte.
const maxReportDays = 62

// Report arma el reporte de asistencia entre dos días del negocio (ambos incluidos).
func (s *AttendanceService) Report(ctx context.Context, from, to time.Time, userID *int64) (*AttendanceReport, error) {
	if to.Before(from) || to.Sub(from) > maxReportDays*24*time.Hour {
		return nil, fmt.Errorf("%w: el rango va de un día a dos meses", domain.ErrValidation)
	}
	q := s.store.QC(ctx)
	cfg, err := q.GetAttendanceSettings(ctx)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	settings := domain.AttendanceSettings{Measure: cfg.AttendanceMeasureSchedule,
		GraceIn: int(cfg.AttendanceGraceInMin), GraceOut: int(cfg.AttendanceGraceOutMin)}
	loc := domain.LoadBusinessLocation(cfg.Timezone)

	versions := map[int64][]domain.ScheduleVersion{}
	if settings.Measure {
		rows, err := q.ListScheduleDays(ctx, userID)
		if err != nil {
			return nil, err
		}
		// Las filas vienen por persona y vigencia; una versión sin días (todo libre) trae weekday nulo.
		byVersion := map[int64]*domain.ScheduleVersion{}
		owner := map[int64]int64{}
		var order []int64
		for _, r := range rows {
			v, ok := byVersion[r.VersionID]
			if !ok {
				v = &domain.ScheduleVersion{EffectiveFrom: r.EffectiveFrom.Time, Days: map[time.Weekday][]domain.Shift{}}
				byVersion[r.VersionID], owner[r.VersionID] = v, r.UserID
				order = append(order, r.VersionID)
			}
			if r.Weekday != nil {
				wd := time.Weekday(*r.Weekday)
				v.Days[wd] = append(v.Days[wd], domain.Shift{Start: minutesOf(r.StartTime), End: minutesOf(r.EndTime)})
			}
		}
		for _, id := range order {
			versions[owner[id]] = append(versions[owner[id]], *byVersion[id])
		}
	}

	rows, err := q.ListAttendanceEntriesInRange(ctx, db.ListAttendanceEntriesInRangeParams{
		FromDate: pgtype.Date{Time: from, Valid: true}, ToDate: pgtype.Date{Time: to, Valid: true}, UserID: userID})
	if err != nil {
		return nil, err
	}
	rep := &AttendanceReport{MeasureSchedule: settings.Measure, People: []ReportPerson{}}
	personIdx := map[int64]int{}
	dayIdx := map[[2]int64]int{}
	dayEntries := map[[2]int64][]domain.DayEntry{}
	for _, r := range rows {
		pi, ok := personIdx[r.UserID]
		if !ok {
			rep.People = append(rep.People, ReportPerson{UserID: r.UserID, Name: r.Name, Days: []ReportDay{}})
			pi = len(rep.People) - 1
			personIdx[r.UserID] = pi
		}
		k := [2]int64{r.UserID, r.BusinessDate.Time.Unix()}
		di, ok := dayIdx[k]
		if !ok {
			rep.People[pi].Days = append(rep.People[pi].Days, ReportDay{Date: r.BusinessDate.Time.Format(time.DateOnly), Entries: []ReportEntry{}})
			di = len(rep.People[pi].Days) - 1
			dayIdx[k] = di
		}
		e := ReportEntry{ID: r.ID, In: r.ClockInAt, Status: r.Status, Source: r.Source, Corrected: r.Corrected,
			InIP: r.InIp, OutIP: r.OutIp, InDevice: r.InDevice, OutDevice: r.OutDevice,
			InDeviceKey: derefStr(r.InDeviceKey), OutDeviceKey: derefStr(r.OutDeviceKey)}
		de := domain.DayEntry{In: r.ClockInAt, Status: r.Status}
		if r.ClockOutAt.Valid {
			t := r.ClockOutAt.Time
			e.Out, de.Out = &t, &t
		}
		rep.People[pi].Days[di].Entries = append(rep.People[pi].Days[di].Entries, e)
		dayEntries[k] = append(dayEntries[k], de)
	}
	for pi := range rep.People {
		p := &rep.People[pi]
		for di := range p.Days {
			d := &p.Days[di]
			businessDay, _ := time.Parse(time.DateOnly, d.Date)
			sum := domain.AttendanceDay(businessDay, dayEntries[[2]int64{p.UserID, businessDay.Unix()}], versions[p.UserID], settings, loc)
			d.WorkedMinutes, d.LateMinutes, d.OvertimeMinutes = sum.WorkedMinutes, sum.LateMinutes, sum.OvertimeMinutes
			d.MissingOut, d.NoSchedule = sum.MissingOut, sum.NoSchedule
			p.WorkedMinutes += sum.WorkedMinutes
			p.LateMinutes = addPtr(p.LateMinutes, sum.LateMinutes)
			p.OvertimeMinutes = addPtr(p.OvertimeMinutes, sum.OvertimeMinutes)
			if sum.MissingOut {
				p.MissingOutDays++
			}
		}
	}
	return rep, nil
}

func addPtr(total, v *int) *int {
	if v == nil {
		return total
	}
	n := *v
	if total != nil {
		n += *total
	}
	return &n
}
