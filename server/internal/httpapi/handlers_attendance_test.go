package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// CHECAR COMPARTE EL BLOQUEO DEL DESBLOQUEO (FR-002). Si tuviera su propia llave, quien agota los
// intentos del PIN de alguien en la pantalla de bloqueo seguiría probando en la de asistencia, y
// duplicaría su presupuesto. Y el evento dice a quién, nunca el PIN.
func TestClockSharesTheUnlockLockoutAndNeverLogsThePIN(t *testing.T) {
	const secret = "913571"
	h := NewHandlers(Deps{Cfg: config.Config{}})
	ctx := context.Background()
	for i := 0; i < authFailMax+1; i++ {
		h.authFails.record(ctx, "pin:42") // la llave con la que el desbloqueo cuenta los fallos
	}

	var log bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&log, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(previous)

	body, _ := json.Marshal(map[string]any{"userId": 42, "pin": secret, "expected": "in"})
	req := httptest.NewRequest(http.MethodPost, "/attendance/clock", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), userCtxKey, AuthUser{ID: 7, CompanyID: 1}))
	w := httptest.NewRecorder()
	h.ClockAttendance(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, quiere 429: los fallos del desbloqueo no frenan a checar", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("sin Retry-After: la pantalla no puede decir en cuántos minutos reintentar")
	}
	if strings.Contains(log.String(), secret) {
		t.Fatalf("el PIN acabó en el log: %s", log.String())
	}
	if !strings.Contains(log.String(), "target_user_id") {
		t.Errorf("el evento no dice a quién se intentó checar: %s", log.String())
	}
}

func TestClockWithoutAPersonIsRejectedBeforeTheService(t *testing.T) {
	h := NewHandlers(Deps{Cfg: config.Config{}})
	body, _ := json.Marshal(map[string]any{"pin": "1234", "expected": "in"})
	req := httptest.NewRequest(http.MethodPost, "/attendance/clock", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), userCtxKey, AuthUser{ID: 7, CompanyID: 1}))
	w := httptest.NewRecorder()
	h.ClockAttendance(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quiere 400", w.Code)
	}
}

// Lo que ve la tableta al checar nunca trae IP, aparato ni clave: la pantalla está a la vista del
// mostrador y esos datos son solo para quien tiene permiso (FR-006).
func TestWhatTheTabletSeesNeverCarriesTheOrigin(t *testing.T) {
	worked := 490
	for _, v := range []any{
		app.RosterPerson{UserID: 1, Name: "Ana", HasPin: true},
		app.ClockResult{Kind: domain.ClockOut, WorkedMinutes: &worked},
	} {
		b, _ := json.Marshal(v)
		for _, fieldName := range []string{"ip", "device", "deviceKey", "Ip", "Device"} {
			if strings.Contains(string(b), `"`+fieldName) {
				t.Errorf("%T expone %q: %s", v, fieldName, b)
			}
		}
	}
}
