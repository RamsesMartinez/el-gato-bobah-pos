package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

func TestRequireRole(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	guard := RequireRole(domain.RoleAdmin, domain.RoleGerente)(next)

	call := func(u *AuthUser) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/stock/movements", nil)
		if u != nil {
			req = req.WithContext(context.WithValue(req.Context(), userCtxKey, *u))
		}
		rec := httptest.NewRecorder()
		guard.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := call(&AuthUser{ID: 1, Role: domain.RoleMesero}); code != http.StatusForbidden {
		t.Errorf("mesero should be forbidden on a manager route, got %d", code)
	}
	if code := call(&AuthUser{ID: 2, Role: domain.RoleCajero}); code != http.StatusForbidden {
		t.Errorf("cajero should be forbidden on an admin/gerente route, got %d", code)
	}
	if code := call(&AuthUser{ID: 3, Role: domain.RoleGerente}); code != http.StatusOK {
		t.Errorf("gerente should be allowed, got %d", code)
	}
	if code := call(nil); code != http.StatusForbidden {
		t.Errorf("no authenticated user should be forbidden, got %d", code)
	}
}

// A09: un 403 debe emitir un evento de seguridad distinto (para detección), con quién y
// qué intentó tocar, sin secretos.
func TestRequireRole_EmitsForbiddenEvent(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(prev)

	guard := RequireRole(domain.RoleAdmin)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/stock/movements", nil)
	req = req.WithContext(context.WithValue(req.Context(), userCtxKey, AuthUser{ID: 7, Role: domain.RoleMesero}))
	guard.ServeHTTP(httptest.NewRecorder(), req)

	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("no se emitió evento parseable: %v (%s)", err, buf.String())
	}
	if m["security_event"] != "forbidden" {
		t.Fatalf("security_event=forbidden esperado, got %v", m["security_event"])
	}
	if m["path"] != "/api/v1/stock/movements" || m["role"] != "mesero" {
		t.Fatalf("evento sin contexto de detección: %v", m)
	}
}

// RequirePermission pregunta por permiso, no por rol. El resolutor se inyecta porque hoy todos los
// roles tienen `orders.cancel_pending`: sin inyectarlo no existe usuario con quien ver el 403, y
// una ruta que perdió su gate pasaría todas las pruebas.
func TestRequirePermission(t *testing.T) {
	none := func(domain.Role) []domain.Permission { return []domain.Permission{} }
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
		all := func(domain.Role) []domain.Permission { return domain.AllPermissions() }
		if rec := call(all, domain.PermPaymentsVoid, &AuthUser{ID: 5, Role: domain.RoleMesero}); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, quiere 200", rec.Code)
		}
	})

	t.Run("el resolutor de hoy niega cancelar pedidos al cajero", func(t *testing.T) {
		if rec := call(domain.PermissionsFor, domain.PermOrdersCancel, &AuthUser{ID: 6, Role: domain.RoleCajero}); rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, quiere 403", rec.Code)
		}
	})

	t.Run("sin usuario autenticado es 403", func(t *testing.T) {
		if rec := call(domain.PermissionsFor, domain.PermOrdersMoveLines, nil); rec.Code != http.StatusForbidden {
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
