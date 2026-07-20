package httpapi

import (
	"context"
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
