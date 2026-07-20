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
