package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
)

// GET /attendance/roster
//
// Quiénes pueden checar en esta estación. Solo nombre y si está dentro: la lista se pinta en una
// tableta a la vista del público, nunca con IP ni aparato.
func (h *Handlers) AttendanceRoster(w http.ResponseWriter, r *http.Request) {
	enabled, people, err := h.attendance.Roster(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"enabled": enabled, "people": people})
}

// POST /attendance/clock
//
// Registra entrada o salida con usuario y PIN, sin cambiar la sesión de la estación. Comparte el
// bloqueo por intentos del desbloqueo (llave `pin:<persona>`): con una llave propia, quien agota
// los intentos en una pantalla seguiría probando en la otra.
func (h *Handlers) ClockAttendance(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID   int64  `json:"userId"`
		PIN      string `json:"pin"`
		Expected string `json:"expected"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if body.UserID <= 0 {
		Error(w, fmt.Errorf("%w: falta indicar quién checa", domain.ErrValidation))
		return
	}
	key := "pin:" + strconv.FormatInt(body.UserID, 10)
	if h.authFails.blocked(r.Context(), key) {
		logging.SecurityEvent(r.Context(), "auth_lockout", "kind", "attendance", "target_user_id", body.UserID, "ip", clientIP(r))
		tooManyRequests(w, h.authFails.retryAfter(r.Context(), key))
		return
	}
	res, err := h.attendance.Clock(r.Context(), body.UserID, body.PIN, domain.ClockKind(body.Expected))
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			h.authFails.record(r.Context(), key)
			logging.SecurityEvent(r.Context(), "pin_failed", "kind", "attendance", "target_user_id", body.UserID, "ip", clientIP(r))
		}
		Error(w, err)
		return
	}
	h.authFails.reset(r.Context(), key)
	JSON(w, http.StatusOK, res)
}

// GET /admin/attendance/settings
func (h *Handlers) AttendanceSettings(w http.ResponseWriter, r *http.Request) {
	v, err := h.attendance.Settings(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, v)
}

// PUT /admin/attendance/settings
func (h *Handlers) SetAttendanceSettings(w http.ResponseWriter, r *http.Request) {
	var body app.AttendanceSettingsView
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.attendance.SetSettings(r.Context(), u.ID, body); err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, body)
}

// PUT /admin/attendance/people/{userId}/tracks
func (h *Handlers) SetTracksAttendance(w http.ResponseWriter, r *http.Request) {
	id, err := pathUserID(r)
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		Tracks bool `json:"tracks"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.attendance.SetTracksAttendance(r.Context(), u.ID, id, body.Tracks); err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]bool{"tracks": body.Tracks})
}

// GET /admin/attendance/schedules/{userId}
func (h *Handlers) AttendanceSchedules(w http.ResponseWriter, r *http.Request) {
	id, err := pathUserID(r)
	if err != nil {
		Error(w, err)
		return
	}
	tracks, err := h.attendance.TracksAttendance(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	v, err := h.attendance.Schedules(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"tracksAttendance": tracks, "versions": v})
}

// POST /admin/attendance/schedules/{userId}
func (h *Handlers) SetAttendanceSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := pathUserID(r)
	if err != nil {
		Error(w, err)
		return
	}
	var body struct {
		EffectiveFrom string                  `json:"effectiveFrom"`
		Days          []app.ScheduleShiftView `json:"days"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	from, err := time.Parse(time.DateOnly, body.EffectiveFrom)
	if err != nil {
		Error(w, fmt.Errorf("%w: la fecha de vigencia no tiene el formato AAAA-MM-DD", domain.ErrValidation))
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.attendance.SetSchedule(r.Context(), u.ID, id, from, body.Days); err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]string{"effectiveFrom": body.EffectiveFrom})
}

// GET /admin/attendance/report?from&to&userId
func (h *Handlers) AttendanceReport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	today := h.backoffice.Now()
	from, err := parseDate(q.Get("from"), today)
	if err != nil {
		Error(w, err)
		return
	}
	to, err := parseDate(q.Get("to"), from)
	if err != nil {
		Error(w, err)
		return
	}
	var userID *int64
	if v := q.Get("userId"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			Error(w, fmt.Errorf("%w: persona inválida", domain.ErrValidation))
			return
		}
		userID = &id
	}
	rep, err := h.attendance.Report(r.Context(), from, to, userID)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, rep)
}

func pathUserID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: persona inválida", domain.ErrValidation)
	}
	return id, nil
}

// POST /admin/attendance/entries/{id}/corrections
//
// El permiso lo decide el servicio y no el router: depende de si el registro es de quien corrige
// (corregir lo propio es otro permiso), y eso solo se sabe leyendo el registro.
func (h *Handlers) CorrectAttendance(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		Error(w, fmt.Errorf("%w: registro inválido", domain.ErrValidation))
		return
	}
	var body struct {
		Field  string    `json:"field"`
		At     time.Time `json:"at"`
		Reason string    `json:"reason"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	if err := h.attendance.Correct(r.Context(), u.ID, h.permissionsOf(u.Role), app.CorrectCmd{
		EntryID: id, Field: body.Field, At: body.At, Reason: body.Reason}); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /admin/attendance/entries — captura a mano un turno completo.
func (h *Handlers) ManualAttendanceEntry(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID int64     `json:"userId"`
		In     time.Time `json:"in"`
		Out    time.Time `json:"out"`
		Reason string    `json:"reason"`
	}
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	u, _ := userFrom(r.Context())
	id, err := h.attendance.ManualEntry(r.Context(), u.ID, h.permissionsOf(u.Role), app.ManualEntryCmd{
		UserID: body.UserID, In: body.In, Out: body.Out, Reason: body.Reason})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]int64{"id": id})
}
