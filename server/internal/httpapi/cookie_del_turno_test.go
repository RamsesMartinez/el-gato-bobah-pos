package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
)

// LA COOKIE NO PUEDE DURAR MÁS QUE EL TURNO QUE GUARDA.
//
// Se ponía con 30 días fijos aunque la sesión venza en ocho horas, así que la tableta seguía
// mandando durante un mes una credencial muerta desde el segundo día. Cada arranque canjea esa
// cookie, el servidor la rechaza y el operador ve "terminó el turno" en vez de la pantalla de
// entrar — y en el navegador queda un mes de rastro de una sesión que ya no existe.
func TestLaCookieDeRefreshVenceConLaSesion(t *testing.T) {
	h := NewHandlers(Deps{Cfg: config.Config{}})
	vence := time.Now().Add(8 * time.Hour)

	w := httptest.NewRecorder()
	h.writeSession(w, &app.Session{
		AccessToken: "a", RefreshToken: "t", CompanyID: 1, RefreshExpiresAt: vence,
	}, http.StatusOK)

	var ck *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == refreshCookie {
			ck = c
		}
	}
	if ck == nil {
		t.Fatal("no se puso la cookie de refresh")
	}
	if d := ck.Expires.Sub(vence); d > time.Minute || d < -time.Minute {
		t.Errorf("la cookie vence %s y la sesión %s: la tableta manda una credencial muerta hasta que el navegador la tire",
			ck.Expires.Format(time.RFC3339), vence.Format(time.RFC3339))
	}
}
