//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
)

// La pantalla pregunta por permiso (`can()`), así que los cuatro caminos por los que llega una
// sesión —login, refresh, relevo por PIN y /auth/me— tienen que traer `permissions`. Se mira el
// JSON CRUDO: deserializar a un []string borra la diferencia entre `null` y `[]`, y un `null`
// tumba la pantalla al primer `.includes()` sin un solo error en el servidor.
func TestEverySessionPathCarriesPermissionsAsAnArray(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT:  jm,
		Auth: app.NewAuthServiceConPepper(st, jm, clock, "pepper-de-prueba"),
	})
	r := httpapi.Router(config.Config{}, jm, h, st)

	const pass = "Contrasena-Larga-1!"
	hash, err := auth.HashSecret(pass)
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	cajero := makeUser(t, st, "cajero_permisos", "cajero")
	gerente := makeUser(t, st, "gerente_permisos", "gerente")
	if _, err := st.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, cajero, hash); err != nil {
		t.Fatalf("fijar password: %v", err)
	}
	conPIN(t, st, gerente, "4827")

	// permissionsOf saca `user.permissions` (o `permissions` en /auth/me) sin pasar por un tipo de Go.
	permissionsOf := func(t *testing.T, body []byte, path ...string) []string {
		t.Helper()
		var node any
		if err := json.Unmarshal(body, &node); err != nil {
			t.Fatalf("respuesta ilegible: %v (%s)", err, body)
		}
		for _, k := range path {
			m, ok := node.(map[string]any)
			if !ok {
				t.Fatalf("%v no es objeto en %s", path, body)
			}
			node = m[k]
		}
		arr, ok := node.([]any)
		if !ok {
			t.Fatalf("%v = %#v, quiere un arreglo (nunca null ni ausente): %s", path, node, body)
		}
		out := make([]string, 0, len(arr))
		for _, v := range arr {
			out = append(out, v.(string))
		}
		return out
	}

	cuerpo, _ := json.Marshal(map[string]string{"username": "cajero_permisos@gatobobah", "password": pass})
	login := do(t, r, http.MethodPost, "/api/v1/auth/login", "", cuerpo, "application/json")
	if login.Code != http.StatusOK {
		t.Fatalf("login = %d: %s", login.Code, login.Body.String())
	}
	perms := permissionsOf(t, login.Body.Bytes(), "user", "permissions")
	if !slices.Contains(perms, "orders.cancel_pending") || slices.Contains(perms, "payments.void") {
		t.Fatalf("login del cajero trae %v: quiere quitar lo que falta y no devolver pagos", perms)
	}
	var sesion struct {
		AccessToken string `json:"accessToken"`
	}
	_ = json.Unmarshal(login.Body.Bytes(), &sesion)
	cookie := login.Result().Cookies()
	if len(cookie) == 0 {
		t.Fatal("el login no dejó la cookie de refresh")
	}

	withCookie := func(method, path, token string, body []byte, c *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(c)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	t.Run("me", func(t *testing.T) {
		w := do(t, r, http.MethodGet, "/api/v1/auth/me", sesion.AccessToken, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("me = %d: %s", w.Code, w.Body.String())
		}
		permissionsOf(t, w.Body.Bytes(), "permissions")
	})

	t.Run("pin-switch", func(t *testing.T) {
		pin, _ := json.Marshal(map[string]any{"userId": gerente, "pin": "4827"})
		w := withCookie(http.MethodPost, "/api/v1/auth/pin-switch", sesion.AccessToken, pin, cookie[0])
		if w.Code != http.StatusOK {
			t.Fatalf("pin-switch = %d: %s", w.Code, w.Body.String())
		}
		if p := permissionsOf(t, w.Body.Bytes(), "user", "permissions"); !slices.Contains(p, "payments.void") {
			t.Fatalf("el relevo al gerente trae %v: le falta devolver pagos", p)
		}
		cookie = w.Result().Cookies()
	})

	t.Run("refresh", func(t *testing.T) {
		w := withCookie(http.MethodPost, "/api/v1/auth/refresh", "", nil, cookie[0])
		if w.Code != http.StatusOK {
			t.Fatalf("refresh = %d: %s", w.Code, w.Body.String())
		}
		permissionsOf(t, w.Body.Bytes(), "user", "permissions")
	})
}
