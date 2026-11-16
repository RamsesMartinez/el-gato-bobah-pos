//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/mailer"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// El cambio obligatorio de contraseña (#311) se evadía recargando la página: solo lo hacía cumplir
// el desvío de la pantalla de login. Estas pruebas van contra la API, sin pantalla, y como
// `gatobobah_app`, que es como corre en producción.

const adminAssignedPassword = "Asignada-Por-El-Admin-1"

type forcedChangeWorld struct {
	r       http.Handler
	st      *store.Store
	users   *app.UsersService
	forced  int64 // con el cambio obligatorio pendiente y PIN 482915
	regular int64 // con su contraseña propia
}

func newForcedChangeWorld(t *testing.T) forcedChangeWorld {
	t.Helper()
	owner := newTestStore(t)
	ctx := context.Background()
	forced := makeUser(t, owner, "nuevo_forzado", "cajero")
	regular := makeUser(t, owner, "cajera_normal", "cajero")
	hash, err := auth.HashSecret(adminAssignedPassword)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Pool.Exec(ctx, `update users set password_hash = $2, must_change_password = $3 where id = $1`, forced, hash, true); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, regular, hash); err != nil {
		t.Fatal(err)
	}
	conPIN(t, owner, forced, "482915")

	st := appRoleStore(t)
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	users := app.NewUsersService(st, nil, false, "pepper-de-prueba")
	h := httpapi.NewHandlers(httpapi.Deps{
		JWT: jm, Auth: app.NewAuthServiceConPepper(st, jm, clock, "pepper-de-prueba"), Users: users,
		Company: app.NewCompanyService(st), Broker: realtime.NewBroker(),
	})
	return forcedChangeWorld{r: httpapi.Router(config.Config{}, jm, h, st), st: st, users: users, forced: forced, regular: regular}
}

type testSession struct {
	token  string
	cookie *http.Cookie
	body   []byte
}

func (m forcedChangeWorld) call(t *testing.T, method, path, token string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	// Con plazo: si el control deja pasar `/events`, el stream no termina nunca y la prueba se
	// colgaría en vez de fallar.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req := httptest.NewRequest(method, path, bytes.NewReader(raw)).WithContext(ctx)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	m.r.ServeHTTP(w, req)
	return w
}

func (m forcedChangeWorld) sessionFrom(t *testing.T, w *httptest.ResponseRecorder) testSession {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("la sesión no se emitió: %d %s", w.Code, w.Body.String())
	}
	var s struct {
		AccessToken string `json:"accessToken"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &s)
	out := testSession{token: s.AccessToken, body: w.Body.Bytes()}
	for _, c := range w.Result().Cookies() {
		if c.Value != "" {
			out.cookie = c
		}
	}
	return out
}

func (m forcedChangeWorld) login(t *testing.T, username, password string) testSession {
	t.Helper()
	return m.sessionFrom(t, m.call(t, http.MethodPost, "/api/v1/auth/login", "",
		map[string]string{"username": username + "@gatobobah", "password": password}, nil))
}

func errorCode(w *httptest.ResponseRecorder) string {
	var b struct {
		Error struct{ Code string } `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &b)
	return b.Error.Code
}

// rejectedForPasswordChange exige el 403 con el código del cambio obligatorio: un 403 de permisos o un
// 404 de una ruta que no existe no prueban que el control esté puesto.
func rejectedForPasswordChange(t *testing.T, w *httptest.ResponseRecorder, what string) {
	t.Helper()
	if w.Code != http.StatusForbidden || errorCode(w) != "PASSWORD_CHANGE_REQUIRED" {
		t.Errorf("%s = %d %s; quiere 403 PASSWORD_CHANGE_REQUIRED: el cambio obligatorio se evade por aquí",
			what, w.Code, w.Body.String())
	}
}

func mustChangeIn(t *testing.T, body []byte, path ...string) bool {
	t.Helper()
	var node any
	if err := json.Unmarshal(body, &node); err != nil {
		t.Fatalf("respuesta ilegible: %s", body)
	}
	for _, k := range path {
		node = node.(map[string]any)[k]
	}
	v, ok := node.(bool)
	if !ok {
		t.Fatalf("%v no trae mustChangePassword: la pantalla no puede saberlo tras recargar (%s)", path, body)
	}
	return v
}

func TestTheForcedPasswordChangeIsEnforcedByTheServer(t *testing.T) {
	t.Parallel()
	m := newForcedChangeWorld(t)
	s := m.login(t, "nuevo_forzado", adminAssignedPassword)
	if !mustChangeIn(t, s.body, "user", "mustChangePassword") {
		t.Fatal("el login no dice que la contraseña se tiene que cambiar")
	}

	t.Run("una llamada directa a la API se rechaza en toda ruta", func(t *testing.T) {
		for _, c := range []struct {
			method, path string
			body         any
		}{
			{http.MethodGet, "/api/v1/company", nil},
			{http.MethodPost, "/api/v1/orders", map[string]any{}},
			{http.MethodGet, "/api/v1/me/preferences/pos.orden", nil},
			{http.MethodPost, "/api/v1/me/pin", map[string]string{"pin": "730194"}},
			// Fuera del grupo con empresa: los pedidos en vivo y la versión.
			{http.MethodGet, "/api/v1/events", nil},
			{http.MethodGet, "/api/v1/version", nil},
			// Las rutas autenticadas de /auth que no son leer la propia cuenta.
			{http.MethodGet, "/api/v1/auth/unlock-options", nil},
			{http.MethodPost, "/api/v1/auth/pin-switch", map[string]any{"userId": m.regular, "pin": "000000"}},
		} {
			rejectedForPasswordChange(t, m.call(t, c.method, c.path, s.token, c.body, s.cookie), c.method+" "+c.path)
		}
	})

	t.Run("leer la propia cuenta sí, y dice que falta el cambio", func(t *testing.T) {
		w := m.call(t, http.MethodGet, "/api/v1/auth/me", s.token, nil, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("/auth/me = %d %s", w.Code, w.Body.String())
		}
		if !mustChangeIn(t, w.Body.Bytes(), "mustChangePassword") {
			t.Fatal("/auth/me no dice que falta el cambio: tras recargar la pantalla no lo sabría")
		}
	})

	t.Run("recargar (canjear el refresh) no lo evade", func(t *testing.T) {
		r := m.sessionFrom(t, m.call(t, http.MethodPost, "/api/v1/auth/refresh", "", nil, s.cookie))
		if !mustChangeIn(t, r.body, "user", "mustChangePassword") {
			t.Fatal("la sesión que sale del refresh no trae el indicador: la recarga lo borra")
		}
		rejectedForPasswordChange(t, m.call(t, http.MethodGet, "/api/v1/company", r.token, nil, nil), "GET /company tras el refresh")
		s = r
	})

	t.Run("cambiarla por la misma que puso el admin no cuenta", func(t *testing.T) {
		w := m.call(t, http.MethodPost, "/api/v1/me/password", s.token,
			map[string]string{"currentPassword": adminAssignedPassword, "newPassword": adminAssignedPassword}, nil)
		if w.Code == http.StatusNoContent {
			t.Fatal("aceptó «cambiar» a la contraseña que conoce el admin")
		}
		rejectedForPasswordChange(t, m.call(t, http.MethodGet, "/api/v1/company", s.token, nil, nil), "GET /company tras repetirla")
	})

	t.Run("al cambiarla todo vuelve a funcionar, con el mismo token", func(t *testing.T) {
		w := m.call(t, http.MethodPost, "/api/v1/me/password", s.token,
			map[string]string{"currentPassword": adminAssignedPassword, "newPassword": "Mi-Propia-Contrasena-9"}, nil)
		if w.Code != http.StatusNoContent {
			t.Fatalf("cambiar la contraseña = %d %s", w.Code, w.Body.String())
		}
		for _, p := range []string{"/api/v1/company", "/api/v1/version", "/api/v1/auth/unlock-options"} {
			if w := m.call(t, http.MethodGet, p, s.token, nil, nil); w.Code != http.StatusOK {
				t.Errorf("GET %s = %d %s tras cambiarla", p, w.Code, w.Body.String())
			}
		}
		w = m.call(t, http.MethodGet, "/api/v1/auth/me", s.token, nil, nil)
		if mustChangeIn(t, w.Body.Bytes(), "mustChangePassword") {
			t.Fatal("/auth/me sigue pidiendo el cambio después de hacerlo")
		}
	})

	t.Run("cerrar sesión sí", func(t *testing.T) {
		if w := m.call(t, http.MethodPost, "/api/v1/auth/logout", "", nil, s.cookie); w.Code != http.StatusNoContent {
			t.Fatalf("logout = %d", w.Code)
		}
	})
}

// El relevo por PIN emite una sesión sin pasar por el login: si el control viviera en el login o en
// el token, entrar con el PIN de la persona lo saltaría.
func TestThePinUnlockDoesNotSkipTheForcedChange(t *testing.T) {
	t.Parallel()
	m := newForcedChangeWorld(t)
	station := m.login(t, "cajera_normal", adminAssignedPassword)
	handover := m.sessionFrom(t, m.call(t, http.MethodPost, "/api/v1/auth/pin-switch", station.token,
		map[string]any{"userId": m.forced, "pin": "482915"}, station.cookie))
	if !mustChangeIn(t, handover.body, "user", "mustChangePassword") {
		t.Fatal("el relevo por PIN no trae el indicador: la pantalla dejaría operar")
	}
	rejectedForPasswordChange(t, m.call(t, http.MethodGet, "/api/v1/company", handover.token, nil, nil), "GET /company tras el relevo")
}

// Un admin que restablece la contraseña de alguien con la sesión abierta la detiene en la siguiente
// petición: el token se emitió sin el indicador, así que leerlo del token no serviría.
func TestAnAdminResetStopsTheOpenSessionAtTheNextRequest(t *testing.T) {
	t.Parallel()
	m := newForcedChangeWorld(t)
	s := m.login(t, "cajera_normal", adminAssignedPassword)
	if w := m.call(t, http.MethodGet, "/api/v1/company", s.token, nil, nil); w.Code != http.StatusOK {
		t.Fatalf("antes del restablecimiento GET /company = %d", w.Code)
	}
	ctx, release, err := m.st.AcquireTenant(context.Background(), defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	err = m.users.AdminSetPassword(ctx, m.regular, "Otra-Asignada-Por-Admin-2")
	release()
	if err != nil {
		t.Fatal(err)
	}
	rejectedForPasswordChange(t, m.call(t, http.MethodGet, "/api/v1/company", s.token, nil, nil), "GET /company con el token de antes")
}

// La lectura del indicador corre con la empresa de la sesión y nunca ve a la persona de otra.
func TestTheForcedChangeFlagIsReadInsideTheCompany(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	otherCompany := makeCompany(t, owner, "otra-del-cambio")
	forced := makeUser(t, owner, "forzado_aislado", "cajero")
	if _, err := owner.Pool.Exec(context.Background(), `update users set must_change_password = true where id = $1`, forced); err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, defaultCompanyID, otherCompany, func(t *testing.T, st *store.Store, ctx context.Context) {
		if _, err := st.QC(ctx).UserMustChangePassword(ctx, forced); err == nil {
			t.Fatal("leyó el indicador de una persona de otra empresa")
		}
	})
	t.Run("una sesión de otra empresa que nombra a esa persona no pasa", func(t *testing.T) {
		st := appRoleStore(t)
		ctx, release, err := st.AcquireTenant(context.Background(), otherCompany)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if _, err := app.MustChangePassword(ctx, st, defaultCompanyID, forced); err == nil {
			t.Fatal("con la conexión de otra empresa respondió en vez de rechazar")
		}
	})
	t.Run("sin conexión de empresa la abre con la de la sesión", func(t *testing.T) {
		got, err := app.MustChangePassword(context.Background(), appRoleStore(t), defaultCompanyID, forced)
		if err != nil || !got {
			t.Fatalf("= %v, %v; quiere true", got, err)
		}
	})
}

// Un token de agente (MCP) de la persona despacha rutas de este mismo router: con el cambio
// obligatorio pendiente, cada herramienta llega al control y se rechaza. El token no se revoca;
// vuelve a servir cuando la persona cambia su contraseña.
func TestAnAgentTokenDoesNotSkipTheForcedChange(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	tok := w.token(t, w.company, w.admin, "write:expenses")
	if res := callTool(t, w.router, tok, "expenses_list", map[string]any{}); res.isError {
		t.Fatalf("antes del restablecimiento la herramienta falló: %s", res.text)
	}
	if _, err := w.owner.Pool.Exec(context.Background(), `update users set must_change_password = true where id = $1`, w.admin); err != nil {
		t.Fatal(err)
	}
	res := callTool(t, w.router, tok, "expenses_list", map[string]any{})
	if !res.isError {
		t.Fatalf("con el cambio pendiente la herramienta respondió: %s", res.text)
	}
	if _, err := w.owner.Pool.Exec(context.Background(), `update users set must_change_password = false where id = $1`, w.admin); err != nil {
		t.Fatal(err)
	}
	if res := callTool(t, w.router, tok, "expenses_list", map[string]any{}); res.isError {
		t.Fatalf("tras cambiarla la herramienta sigue fallando: %s", res.text)
	}
}

// Quien solo conoce el PIN de un empleado nuevo hace el relevo y le queda `/me/password`: sin
// límite, prueba la contraseña que puso el admin hasta acertar y se queda con la cuenta.
func TestGuessingTheCurrentPasswordIsLockedOut(t *testing.T) {
	t.Parallel()
	m := newForcedChangeWorld(t)
	s := m.login(t, "nuevo_forzado", adminAssignedPassword)
	locked := false
	for i := 0; i < 30 && !locked; i++ {
		w := m.call(t, http.MethodPost, "/api/v1/me/password", s.token,
			map[string]string{"currentPassword": "Adivinanza-" + itoa(i), "newPassword": "Mi-Propia-Contrasena-9"}, nil)
		locked = w.Code == http.StatusTooManyRequests
	}
	if !locked {
		t.Fatal("30 contraseñas actuales equivocadas y ninguna se frenó")
	}
	w := m.call(t, http.MethodPost, "/api/v1/me/password", s.token,
		map[string]string{"currentPassword": adminAssignedPassword, "newPassword": "Mi-Propia-Contrasena-9"}, nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("con la cuenta frenada, la contraseña correcta pasó (%d): el bloqueo no sirve", w.Code)
	}
}

// La recuperación por correo también apaga el cambio obligatorio: «recuperarla» poniendo la misma
// contraseña que puso el admin lo saltaba igual que hacerlo desde la cuenta.
func TestTheEmailResetRejectsTheSamePassword(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	appSt := appRoleStore(t)
	uid := makeUser(t, owner, "reset_misma", "cajero")
	hash, _ := auth.HashSecret(adminAssignedPassword)
	if _, err := owner.Pool.Exec(context.Background(),
		`update users set password_hash = $2, must_change_password = true where id = $1`, uid, hash); err != nil {
		t.Fatal(err)
	}
	token, tokenHash, _ := auth.NewRefreshToken()
	if err := appSt.WithTenant(context.Background(), defaultCompanyID, func(q *db.Queries) error {
		_, e := q.CreatePasswordResetToken(context.Background(), db.CreatePasswordResetTokenParams{
			UserID: uid, TokenHash: tokenHash, ExpiresAt: fixedNow.Add(app.PasswordResetTTL),
		})
		return e
	}); err != nil {
		t.Fatal(err)
	}
	svc := app.NewResetService(appSt, mailer.New("", 0, "", "", ""), nil, false, "http://x", clock)
	if err := svc.Confirm(context.Background(), defaultCompanyID, token, adminAssignedPassword); !errors.Is(err, domain.ErrSamePassword) {
		t.Fatalf("Confirm con la misma contraseña = %v; quiere ErrSamePassword", err)
	}
	got, err := app.MustChangePassword(context.Background(), appSt, defaultCompanyID, uid)
	if err != nil || !got {
		t.Fatalf("el indicador quedó en %v (%v): la recuperación lo apagó sin cambiar nada", got, err)
	}
}

// Toda ruta autenticada lleva el control, también las que se agreguen después: se recorre el
// router entero con la sesión de alguien con el cambio pendiente. Las que pasan sin él están
// nombradas aquí, cada una con su razón.
func TestEveryAuthenticatedRouteEnforcesTheForcedChange(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	if _, err := w.owner.Pool.Exec(context.Background(), `update users set must_change_password = true where id = $1`, w.admin); err != nil {
		t.Fatal(err)
	}
	tok, err := w.jm.Issue(domain.User{ID: w.admin, CompanyID: w.company, Name: "admin", Role: domain.RoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	// `/mcp` (todos sus métodos) tampoco: cada herramienta vuelve a entrar a este router con la
	// identidad del token y pasa por el control (TestAnAgentTokenDoesNotSkipTheForcedChange).
	exempt := map[string]string{
		"POST /api/v1/auth/login":            "entrar: todavía no hay sesión",
		"POST /api/v1/auth/refresh":          "renueva la sesión; la sesión que sale trae el indicador",
		"POST /api/v1/auth/logout":           "cerrar sesión",
		"POST /api/v1/auth/forgot":           "recuperación pública, sin sesión",
		"POST /api/v1/auth/reset":            "recuperación pública, sin sesión",
		"GET /api/v1/auth/me":                "leer la propia cuenta: así sabe la pantalla que tiene que pedir el cambio",
		"POST /api/v1/me/password":           "cambiar la contraseña",
		"POST /api/v1/webhooks/{plataforma}": "la llama la plataforma con su firma, no una persona",
		"POST /api/v1/platform/auth/login":   "consola de plataforma: otra identidad",
		"GET /api/v1/platform/companies":     "consola de plataforma: otra identidad",
		"GET /api/v1/platform/usage":         "consola de plataforma: otra identidad",
		"GET /api/v1/platform/touches":       "consola de plataforma: otra identidad",
		"GET /healthz":                       "salud, sin datos",
		"GET /readyz":                        "salud, sin datos",
	}
	r, ok := w.router.(chi.Routes)
	if !ok {
		t.Fatal("el router no se deja recorrer")
	}
	param := regexp.MustCompile(`\{[^}]+\}`)
	checked := 0
	err = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.Replace(route, "/*/", "/", -1)
		key := method + " " + route
		if _, ok := exempt[key]; ok || route == "/mcp" {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		req := httptest.NewRequest(method, param.ReplaceAllString(route, "1"), strings.NewReader("{}")).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		w.router.ServeHTTP(rec, req)
		rejectedForPasswordChange(t, rec, key)
		checked++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 100 {
		t.Fatalf("solo se recorrieron %d rutas: el recorrido no está viendo el router", checked)
	}
}
