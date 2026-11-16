package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// RequireAnyPermission deja pasar con uno de los permisos y responde 403 sin ninguno: listar los
// lugares lo necesita quien cuenta y quien configura, y un rol propio puede tener solo uno.
func TestRequireAnyPermission(t *testing.T) {
	only := func(p domain.Permission) PermissionResolver {
		return func(context.Context, AuthUser) ([]domain.Permission, error) { return []domain.Permission{p}, nil }
	}
	call := func(resolve PermissionResolver) int {
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		req := httptest.NewRequest(http.MethodGet, "/api/v1/stock/count-places", nil)
		req = req.WithContext(context.WithValue(req.Context(), userCtxKey, AuthUser{ID: 4, Role: domain.RoleGerente}))
		rec := httptest.NewRecorder()
		RequireAnyPermission(resolve, domain.PermInventoryCount, domain.PermInventoryConfigure)(next).ServeHTTP(rec, req)
		return rec.Code
	}
	if c := call(only(domain.PermInventoryCount)); c != http.StatusOK {
		t.Errorf("con contar = %d", c)
	}
	if c := call(only(domain.PermInventoryConfigure)); c != http.StatusOK {
		t.Errorf("con configurar = %d", c)
	}
	if c := call(only(domain.PermOrdersCancelPending)); c != http.StatusForbidden {
		t.Errorf("sin ninguno = %d; quiere 403", c)
	}
	// Sin usuario en el contexto también es 403: invertir el `if ok` no puede dejar pasar a nadie.
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	rec := httptest.NewRecorder()
	RequireAnyPermission(only(domain.PermInventoryCount), domain.PermInventoryCount)(next).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/stock/count-places", nil))
	if rec.Code != http.StatusForbidden {
		t.Errorf("sin usuario = %d; quiere 403", rec.Code)
	}
	defer func() {
		if recover() == nil {
			t.Error("RequireAnyPermission sin permisos se armó sin tronar")
		}
	}()
	RequireAnyPermission(only(domain.PermInventoryCount))
}

// RequirePermission pregunta por permiso, no por rol. El resolutor se inyecta porque hoy todos los
// roles tienen `orders.cancel_pending`: sin inyectarlo no existe usuario con quien ver el 403, y
// una ruta que perdió su gate pasaría todas las pruebas.
func TestRequirePermission(t *testing.T) {
	none := func(context.Context, AuthUser) ([]domain.Permission, error) { return []domain.Permission{}, nil }
	call := func(resolve PermissionResolver, p domain.Permission, u *AuthUser) *httptest.ResponseRecorder {
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/9/lines/cancel-pending", nil)
		if u != nil {
			req = req.WithContext(context.WithValue(req.Context(), userCtxKey, *u))
		}
		rec := httptest.NewRecorder()
		RequirePermission(resolve, p)(next).ServeHTTP(rec, req)
		return rec
	}

	t.Run("sin el permiso es 403 con el texto del permiso", func(t *testing.T) {
		rec := call(none, domain.PermOrdersCancelPending, &AuthUser{ID: 4, Role: domain.RoleAdmin})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, quiere 403", rec.Code)
		}
		var env errorEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("cuerpo ilegible: %v (%s)", err, rec.Body.String())
		}
		if want := domain.PermissionDeniedMessage(domain.PermOrdersCancelPending); env.Error.Message != want {
			t.Fatalf("mensaje = %q, quiere %q", env.Error.Message, want)
		}
		if env.Error.Code != "FORBIDDEN" {
			t.Fatalf("código = %q, quiere FORBIDDEN", env.Error.Code)
		}
	})

	t.Run("con el permiso pasa", func(t *testing.T) {
		all := func(context.Context, AuthUser) ([]domain.Permission, error) { return domain.AllPermissions(), nil }
		if rec := call(all, domain.PermPaymentsVoid, &AuthUser{ID: 5, Role: domain.RoleMesero}); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, quiere 200", rec.Code)
		}
	})

	t.Run("el resolutor de hoy niega cancelar pedidos al cajero", func(t *testing.T) {
		if rec := call(byRole, domain.PermOrdersCancel, &AuthUser{ID: 6, Role: domain.RoleCajero}); rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, quiere 403", rec.Code)
		}
	})

	t.Run("sin usuario autenticado es 403", func(t *testing.T) {
		if rec := call(byRole, domain.PermOrdersMoveLines, nil); rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, quiere 403", rec.Code)
		}
	})

	t.Run("el 403 deja evento de seguridad", func(t *testing.T) {
		var buf bytes.Buffer
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
		defer slog.SetDefault(prev)
		call(none, domain.PermPaymentsVoid, &AuthUser{ID: 7, Role: domain.RoleGerente})
		var m map[string]any
		if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
			t.Fatalf("no se emitió evento parseable: %v (%s)", err, buf.String())
		}
		if m["security_event"] != "forbidden" || m["permission"] != "payments.void" || m["path"] == nil {
			t.Fatalf("evento sin contexto de detección: %v", m)
		}
	})
}

func byRole(_ context.Context, u AuthUser) ([]domain.Permission, error) {
	return domain.PermissionsFor(u.Role), nil
}

// El cambio obligatorio de contraseña (#311) lo hace cumplir el servidor, no la pantalla: con el
// indicador encendido nada pasa, el rechazo tiene código propio para que la pantalla mande a
// cambiarla, y deja evento de seguridad. Si la lectura falla, tampoco pasa: el control falla cerrado.
func TestRequirePasswordChanged(t *testing.T) {
	call := func(check PasswordChangeCheck, u *AuthUser) (*httptest.ResponseRecorder, bool, string) {
		var buf bytes.Buffer
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
		defer slog.SetDefault(prev)
		reached := false
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reached = true
			w.WriteHeader(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/company", nil)
		if u != nil {
			req = req.WithContext(context.WithValue(req.Context(), userCtxKey, *u))
		}
		rec := httptest.NewRecorder()
		RequirePasswordChanged(check)(next).ServeHTTP(rec, req)
		return rec, reached, buf.String()
	}
	who := &AuthUser{ID: 7, CompanyID: 2}
	pending := func(context.Context, AuthUser) (bool, error) { return true, nil }
	changed := func(context.Context, AuthUser) (bool, error) { return false, nil }
	broken := func(context.Context, AuthUser) (bool, error) { return false, errors.New("la base no responde") }

	t.Run("con el indicador encendido: 403 con código propio y evento", func(t *testing.T) {
		rec, reached, logs := call(pending, who)
		if reached {
			t.Fatal("la ruta corrió con el cambio de contraseña pendiente")
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("código = %d; quiere 403", rec.Code)
		}
		var body struct {
			Error struct{ Code string } `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body.Error.Code != "PASSWORD_CHANGE_REQUIRED" {
			t.Fatalf("código de error = %q; la pantalla no sabría mandar a cambiarla", body.Error.Code)
		}
		if !strings.Contains(logs, `"security_event":"password_change_required"`) || !strings.Contains(logs, `"user_id":7`) {
			t.Fatalf("sin evento de seguridad con la persona: %s", logs)
		}
	})
	t.Run("con la contraseña ya cambiada pasa", func(t *testing.T) {
		if rec, reached, _ := call(changed, who); !reached || rec.Code != http.StatusOK {
			t.Fatalf("código = %d, llegó = %v", rec.Code, reached)
		}
	})
	t.Run("si la lectura falla no pasa", func(t *testing.T) {
		if rec, reached, _ := call(broken, who); reached || rec.Code != http.StatusInternalServerError {
			t.Fatalf("con la base caída la ruta corrió (código %d): el control falla abierto", rec.Code)
		}
	})
	t.Run("sin persona en el contexto no pasa", func(t *testing.T) {
		if rec, reached, _ := call(changed, nil); reached || rec.Code != http.StatusUnauthorized {
			t.Fatalf("código = %d, llegó = %v", rec.Code, reached)
		}
	})
}
