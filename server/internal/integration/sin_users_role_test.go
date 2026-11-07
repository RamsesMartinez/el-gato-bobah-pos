//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// ESTA API AGUANTA QUE SE BORRE users.role (spec 068, entrega 3). La 0109 borra la columna mientras
// esta versión sigue sirviendo (constitución IV): ninguna consulta suya puede nombrarla. Se borra en
// la base de la prueba y se recorre lo que toca a usuarios: alta, edición, lista, login y /auth/me.
func TestTheAPIWorksWithoutUsersRole(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	adminID := makeUser(t, st, "sin_role_admin", "admin")
	for _, stmt := range []string{
		`drop trigger users_sync_role on users`,
		`alter table users alter column role_id set not null`,
		`alter table users drop column role`,
	} {
		if _, err := st.Pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Broker: realtime.NewBroker(),
		Auth:  app.NewAuthServiceConPepper(st, jm, clock, "pepper-de-prueba"),
		Users: app.NewUsersService(st, nil, false, "pepper-de-prueba")})
	r := httpapi.Router(config.Config{}, jm, h, st)
	tok, err := jm.Issue(domain.User{ID: adminID, CompanyID: defaultCompanyID, Name: "a"})
	if err != nil {
		t.Fatal(err)
	}
	cajero := roleIDOf(t, st, defaultCompanyID, "cajero")

	const pass = "Contrasena-Larga-1!"
	body, _ := json.Marshal(map[string]any{"name": "Nueva", "username": "sin_role_nueva", "roleId": cajero, "password": pass})
	w := do(t, r, http.MethodPost, "/api/v1/users", tok, body, "application/json")
	if w.Code != http.StatusCreated {
		t.Fatalf("alta = %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	body, _ = json.Marshal(map[string]any{"name": "Nueva 2", "roleId": cajero, "isActive": true})
	if w := do(t, r, http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", created.ID), tok, body, "application/json"); w.Code != http.StatusOK {
		t.Fatalf("edición = %d: %s", w.Code, w.Body.String())
	}
	if w := do(t, r, http.MethodGet, "/api/v1/users", tok, nil, ""); w.Code != http.StatusOK {
		t.Fatalf("lista = %d: %s", w.Code, w.Body.String())
	}
	if _, err := st.Pool.Exec(ctx, `update users set must_change_password = false where id = $1`, created.ID); err != nil {
		t.Fatal(err)
	}
	body, _ = json.Marshal(map[string]string{"username": "sin_role_nueva@gatobobah", "password": pass})
	login := do(t, r, http.MethodPost, "/api/v1/auth/login", "", body, "application/json")
	if login.Code != http.StatusOK {
		t.Fatalf("login = %d: %s", login.Code, login.Body.String())
	}
	if w := do(t, r, http.MethodGet, "/api/v1/auth/me", tok, nil, ""); w.Code != http.StatusOK {
		t.Fatalf("/auth/me = %d: %s", w.Code, w.Body.String())
	}
}
