//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

func TestIntegrationsTokensHTTP(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	jwtFor := func(id, company int64, role domain.Role) string {
		tok, err := w.jm.Issue(domain.User{ID: id, CompanyID: company, Name: "x", Role: role})
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	admin := jwtFor(w.admin, w.company, domain.RoleAdmin)
	gerente := jwtFor(makeUser(t, w.owner, "gerente-int", "gerente"), w.company, domain.RoleGerente)
	body := func(v any) []byte { b, _ := json.Marshal(v); return b }
	const js = "application/json"

	t.Run("sin integrations.manage no se lista ni se crea", func(t *testing.T) {
		if rec := do(t, w.router, http.MethodGet, "/api/v1/integrations/tokens", gerente, nil, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("GET = %d", rec.Code)
		}
		rec := do(t, w.router, http.MethodPost, "/api/v1/integrations/tokens", gerente,
			body(map[string]any{"name": "x", "expiresInDays": 30, "password": tokenPassword}), js)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("POST = %d", rec.Code)
		}
	})

	t.Run("contraseña equivocada: 401 sin token", func(t *testing.T) {
		rec := do(t, w.router, http.MethodPost, "/api/v1/integrations/tokens", admin,
			body(map[string]any{"name": "x", "expiresInDays": 30, "password": "otra"}), js)
		if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), "egbpat_") {
			t.Fatalf("POST con contraseña equivocada = %d %s", rec.Code, rec.Body.String())
		}
	})

	var created struct {
		ID      int64    `json:"id"`
		Token   string   `json:"token"`
		OwnerID int64    `json:"ownerId"`
		Scopes  []string `json:"scopes"`
	}
	t.Run("crear: a nombre de quien llama, aunque mande otro userId", func(t *testing.T) {
		rec := do(t, w.router, http.MethodPost, "/api/v1/integrations/tokens", admin,
			body(map[string]any{"name": "Claude de la laptop", "scopes": []string{"write:expenses"}, "expiresInDays": 90,
				"password": tokenPassword, "userId": w.otherAdm, "ownerId": w.otherAdm}), js)
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST = %d %s", rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatal(err)
		}
		if created.OwnerID != w.admin || !strings.HasPrefix(created.Token, "egbpat_") ||
			strings.Join(created.Scopes, ",") != "read,write:expenses" {
			t.Fatalf("creado: %+v", created)
		}
	})

	t.Run("la lista no trae el token", func(t *testing.T) {
		rec := do(t, w.router, http.MethodGet, "/api/v1/integrations/tokens", admin, nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET = %d", rec.Code)
		}
		p, _ := domain.ParseAPIToken(created.Token)
		if strings.Contains(rec.Body.String(), p.Secret) || !strings.Contains(rec.Body.String(), `"items"`) {
			t.Fatalf("lista: %s", rec.Body.String())
		}
	})

	t.Run("revocar dos veces: 204 y 204; de otra empresa: 404", func(t *testing.T) {
		path := "/api/v1/integrations/tokens/" + strconv.FormatInt(created.ID, 10) + "/revoke"
		otherAdmin := jwtFor(w.otherAdm, w.other, domain.RoleAdmin)
		if rec := do(t, w.router, http.MethodPost, path, otherAdmin, nil, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("revocar de otra empresa = %d", rec.Code)
		}
		for range 2 {
			if rec := do(t, w.router, http.MethodPost, path, admin, nil, ""); rec.Code != http.StatusNoContent {
				t.Fatalf("revocar = %d %s", rec.Code, rec.Body.String())
			}
		}
		var revoked bool
		_ = w.owner.Pool.QueryRow(ctx, `select revoked_at is not null from api_tokens where id = $1`, created.ID).Scan(&revoked)
		if !revoked {
			t.Fatal("no quedó revocado")
		}
	})

	t.Run("los intentos con contraseña equivocada terminan en bloqueo", func(t *testing.T) {
		var last int
		for range 12 {
			rec := do(t, w.router, http.MethodPost, "/api/v1/integrations/tokens", admin,
				body(map[string]any{"name": "x", "expiresInDays": 30, "password": "mala"}), js)
			last = rec.Code
			if last == http.StatusTooManyRequests {
				break
			}
		}
		if last != http.StatusTooManyRequests {
			t.Fatalf("12 contraseñas equivocadas y sin bloqueo (último %d)", last)
		}
	})
}
