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

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
)

// FR-015. El evento de seguridad del desbloqueo lleva a QUIÉN se intentó desbloquear y desde dónde,
// nunca el PIN.
//
// Un secreto en un log es peor que no tener el log: sobrevive a la rotación, viaja a donde sea que
// se envíen los registros, y lo lee gente que nunca tuvo por qué conocerlo. El principio V no deja
// mergear un control de seguridad sin su test, y este es el que lo cubre.
//
// Se ejercita el camino del LOCKOUT porque es el único que emite sin tocar la base: basta agotar el
// limitador para que el siguiente intento registre `auth_lockout` con el PIN en el cuerpo.
func TestElEventoDeDesbloqueoNoLlevaElPin(t *testing.T) {
	const secreto = "913571"
	const objetivo = 42

	h := NewHandlers(Deps{Cfg: config.Config{}})

	// Se agota el limitador del usuario objetivo para forzar la rama que registra el evento.
	ctx := context.Background()
	key := "pin:42"
	for i := 0; i < authFailMax+1; i++ {
		h.authFails.record(ctx, key)
	}

	var log bytes.Buffer
	anterior := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&log, &slog.HandlerOptions{Level: slog.LevelInfo})))
	defer slog.SetDefault(anterior)

	cuerpo, _ := json.Marshal(map[string]any{"userId": objetivo, "pin": secreto})
	req := httptest.NewRequest(http.MethodPost, "/auth/pin-switch", bytes.NewReader(cuerpo))
	// La sesión del dispositivo: el handler la exige antes de cualquier otra cosa.
	req = req.WithContext(context.WithValue(req.Context(), userCtxKey, AuthUser{ID: 7, CompanyID: 1}))
	w := httptest.NewRecorder()
	h.PinSwitch(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, quiere 429: el test necesita la rama que registra el evento", w.Code)
	}
	registrado := log.String()
	if !strings.Contains(registrado, "auth_lockout") {
		t.Fatalf("no se registró el evento de seguridad: %s", registrado)
	}
	if strings.Contains(registrado, secreto) {
		t.Fatalf("el PIN acabó en el log: %s", registrado)
	}
	// Y sí lleva a quién se intentó desbloquear, que es lo que vuelve accionable el evento.
	if !strings.Contains(registrado, "target_user_id") {
		t.Errorf("el evento no dice a quién se intentó desbloquear: %s", registrado)
	}
}
