//go:build integration

package integration

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
)

// CHECAR ESTÁ DETRÁS DEL LÍMITE POR IP. El bloqueo por persona frena a quien prueba PINs de UNA
// persona; quien recorre la lista probando un PIN por cabeza no lo toca nunca. Quitar el
// `.With(rateLimit(...))` de la ruta no rompe nada visible, y por eso se prueba con el router real.
func TestClockIsBehindThePerIPThrottle(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	appSt := appRoleStore(t)
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Attendance: app.NewAttendanceService(appSt, clock)})
	r := httpapi.Router(config.Config{}, jm, h, appSt)
	id := makeUser(t, st, "estacion-throttle", "cajero")
	tok, err := jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: "estacion-throttle", Role: domain.RoleCajero})
	if err != nil {
		t.Fatal(err)
	}

	// Sin persona en el cuerpo el handler responde 400 sin tocar la base: el 429 tiene que llegar
	// antes que él, así que cualquier cuerpo sirve para gastar el presupuesto.
	last := 0
	for i := 0; i < 62; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/attendance/clock", bytes.NewReader([]byte(`{"pin":"0000","expected":"in"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		last = w.Code
		if last == http.StatusTooManyRequests {
			return
		}
	}
	t.Fatalf("tras 62 intentos seguidos desde una IP checar sigue respondiendo %d: no está detrás del límite por IP", last)
}
