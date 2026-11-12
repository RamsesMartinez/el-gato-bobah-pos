//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// PERSONAS DADAS DE BAJA (#236): la pantalla de Empleados las lista aparte y las reactiva.

func (a *rolesAPI) userNames(path, tok string) (int, []string) {
	a.t.Helper()
	w := do(a.t, a.r, http.MethodGet, path, tok, nil, "")
	if w.Code != http.StatusOK {
		return w.Code, nil
	}
	var body struct {
		Items []domain.User `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		a.t.Fatal(err)
	}
	out := []string{}
	for _, u := range body.Items {
		out = append(out, *u.Username)
	}
	return w.Code, out
}

func has(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func TestInactiveUsersAreListedApartAndCanBeReactivated(t *testing.T) {
	t.Parallel()
	a := newRolesAPI(t)
	ida := makeUser(t, a.st, "ida_baja", "cajero")
	makeUser(t, a.st, "sigue_activa", "cajero")
	if code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", ida), a.admin, map[string]any{
		"name": "Ida", "roleId": roleIDOf(t, a.st, defaultCompanyID, "cajero"), "isActive": false}); code != http.StatusOK {
		t.Fatalf("dar de baja = %d %v", code, body)
	}

	t.Run("la lista de siempre no cambia: solo activos", func(t *testing.T) {
		_, names := a.userNames("/api/v1/users", a.admin)
		if has(names, "ida_baja") || !has(names, "sigue_activa") {
			t.Fatalf("GET /users = %v", names)
		}
	})
	t.Run("status=inactive trae solo a las bajas", func(t *testing.T) {
		code, names := a.userNames("/api/v1/users?status=inactive", a.admin)
		if code != http.StatusOK || !has(names, "ida_baja") || has(names, "sigue_activa") {
			t.Fatalf("= %d %v", code, names)
		}
	})
	t.Run("una lista vacía es [] y no null", func(t *testing.T) {
		co := makeCompany(t, a.st, "bajas-vacia")
		tok, err := a.jm.Issue(domain.User{ID: makeUserIn(t, a.st, co, "admin_vacia", "admin"), CompanyID: co, Name: "u"})
		if err != nil {
			t.Fatal(err)
		}
		w := do(t, a.r, http.MethodGet, "/api/v1/users?status=inactive", tok, nil, "")
		if w.Code != http.StatusOK {
			t.Fatalf("= %d %s", w.Code, w.Body.String())
		}
		var raw map[string]json.RawMessage
		_ = json.Unmarshal(w.Body.Bytes(), &raw)
		if string(raw["items"]) == "null" {
			t.Fatal("items llegó como null: la pantalla hace .filter() y se cae")
		}
	})
	t.Run("un status desconocido se rechaza, no cae a activos", func(t *testing.T) {
		for _, s := range []string{"inactivo", "all", "INACTIVE"} {
			if code, _ := a.userNames("/api/v1/users?status="+s, a.admin); code != http.StatusBadRequest {
				t.Errorf("status=%s = %d, quiere 400", s, code)
			}
		}
	})
	t.Run("sin permiso de personal: 403 en la lista de bajas y al reactivar", func(t *testing.T) {
		cajero := a.token(makeUser(t, a.st, "cajero_sin_personal", "cajero"))
		if code, _ := a.userNames("/api/v1/users?status=inactive", cajero); code != http.StatusForbidden {
			t.Errorf("lista de bajas = %d, quiere 403", code)
		}
		code, _ := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", ida), cajero, map[string]any{
			"name": "Ida", "roleId": roleIDOf(t, a.st, defaultCompanyID, "cajero"), "isActive": true})
		if code != http.StatusForbidden {
			t.Errorf("reactivar = %d, quiere 403", code)
		}
		if _, names := a.userNames("/api/v1/users?status=inactive", a.admin); !has(names, "ida_baja") {
			t.Error("el 403 la reactivó de todos modos")
		}
	})
	t.Run("reactivar la regresa a la lista de siempre", func(t *testing.T) {
		code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", ida), a.admin, map[string]any{
			"name": "Ida", "roleId": roleIDOf(t, a.st, defaultCompanyID, "cajero"), "isActive": true})
		if code != http.StatusOK {
			t.Fatalf("reactivar = %d %v", code, body)
		}
		_, active := a.userNames("/api/v1/users", a.admin)
		_, inactive := a.userNames("/api/v1/users?status=inactive", a.admin)
		if !has(active, "ida_baja") || has(inactive, "ida_baja") {
			t.Fatalf("activos %v, bajas %v", active, inactive)
		}
	})
}

// Un rol propio inactivo no se le devuelve a quien se reactiva: la misma petición lleva otro rol
// activo, y así se reactiva en un solo paso.
func TestReactivatingSomeoneWhoseCustomRoleIsInactiveAsksForAnotherRole(t *testing.T) {
	t.Parallel()
	a := newRolesAPI(t)
	_, _ = a.call(http.MethodPost, "/api/v1/roles", a.admin, map[string]any{"name": "Barista de noche", "permissions": []string{"orders.move_lines"}})
	barista := a.roles()["Barista de noche"]
	u := makeUser(t, a.st, "barista_noche", "cajero")
	patch := func(roleID int64, active bool) (int, map[string]any) {
		return a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", u), a.admin, map[string]any{
			"name": "Barista", "roleId": roleID, "isActive": active})
	}
	if code, body := patch(barista.ID, true); code != http.StatusOK {
		t.Fatalf("asignar = %d %v", code, body)
	}
	if code, body := patch(barista.ID, false); code != http.StatusOK {
		t.Fatalf("baja = %d %v", code, body)
	}
	b := a.roles()["Barista de noche"]
	if code, body := a.call(http.MethodPost, fmt.Sprintf("/api/v1/roles/%d/deactivate", b.ID), a.admin, map[string]any{"version": b.Version}); code != http.StatusOK {
		t.Fatalf("inactivar rol = %d %v", code, body)
	}

	if code, body := patch(barista.ID, true); code != http.StatusConflict || errCode(body) != "ROLE_INACTIVE" {
		t.Fatalf("reactivar con su rol inactivo = %d %v, quiere ROLE_INACTIVE", code, body)
	}
	cajero := roleIDOf(t, a.st, defaultCompanyID, "cajero")
	if code, body := patch(cajero, true); code != http.StatusOK {
		t.Fatalf("reactivar con otro rol activo = %d %v", code, body)
	}
	if n := a.auditCount("user.role_changed"); n != 2 {
		t.Errorf("eventos de cambio de rol = %d, quiere 2 (asignar el propio y volver a cajero)", n)
	}
}

// El usuario y el PIN de una persona dada de baja siguen siendo suyos: la unicidad cubre a las bajas,
// así que reactivarla nunca choca con alguien que los tomó. Lo que se prueba es que nadie los toma, y
// que el rechazo dice qué está repetido.
func TestTheUsernameAndPINOfSomeoneInactiveAreNotTaken(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	co := makeCompany(t, st, "bajas-unicidad")
	svc := app.NewUsersService(st, nil, false, "pepper-de-prueba")
	admin := domain.PermissionsFor(domain.RoleAdmin)
	// Por id y no por código: este store es el owner, que no tiene RLS que elija el sugerido de su empresa.
	cajero := roleIDOf(t, st, co, "cajero")
	makeUserIn(t, st, co, "admin_unicidad", "admin") // sin quien administre roles, ninguna baja procede

	tctx, release, err := st.AcquireTenant(ctx, co)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	user := "ida_unica"
	ida, err := svc.Create(tctx, app.CreateUserInput{Name: "Ida", Username: &user, RoleID: &cajero, ActorPerms: admin,
		PIN: "482915", Password: "Contrasena-Larga-1!"})
	if err != nil {
		t.Fatalf("alta: %v", err)
	}
	if _, err := svc.Update(tctx, app.UpdateUserInput{ID: ida.ID, Name: "Ida", RoleID: ida.RoleID, ActorPerms: admin, IsActive: false}); err != nil {
		t.Fatalf("baja: %v", err)
	}

	t.Run("su usuario no se reparte, y el rechazo dice que es el usuario", func(t *testing.T) {
		_, err := svc.Create(tctx, app.CreateUserInput{Name: "Otra Ida", Username: &user, RoleID: &cajero, ActorPerms: admin,
			Password: "Contrasena-Larga-2!"})
		if !errors.Is(err, domain.ErrUsernameTaken) {
			t.Fatalf("alta con el usuario de una baja: %v, quiere ErrUsernameTaken (no «ese PIN ya lo usa otra persona»)", err)
		}
	})
	t.Run("su PIN no se reparte", func(t *testing.T) {
		other := "beto_pin"
		_, err := svc.Create(tctx, app.CreateUserInput{Name: "Beto", Username: &other, RoleID: &cajero, ActorPerms: admin,
			PIN: "482915", Password: "Contrasena-Larga-3!"})
		if !errors.Is(err, domain.ErrPinRepetido) {
			t.Fatalf("alta con el PIN de una baja: %v, quiere ErrPinRepetido", err)
		}
	})
	t.Run("al reactivarla conserva usuario y PIN", func(t *testing.T) {
		u, err := svc.Update(tctx, app.UpdateUserInput{ID: ida.ID, Name: "Ida", RoleID: ida.RoleID, ActorPerms: admin, IsActive: true})
		if err != nil || !u.IsActive || u.Username == nil || *u.Username != user {
			t.Fatalf("reactivar: %+v %v", u, err)
		}
	})
}

// Quien se reactiva vuelve a entrar con su contraseña, y su asistencia de antes de la baja sigue ahí.
func TestAReactivatedPersonLogsInAgainAndKeepsTheirAttendance(t *testing.T) {
	t.Parallel()
	a := newRolesAPI(t)
	ctx := context.Background()
	u := makeUser(t, a.st, "vuelve_a_entrar", "cajero")
	hash, err := auth.HashSecret("Contrasena-Larga-1!")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.st.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, u, hash); err != nil {
		t.Fatal(err)
	}
	if _, err := a.st.Pool.Exec(ctx, `insert into attendance_entries (company_id, user_id, business_date, clock_in_at, clock_out_at, status)
		values ($1, $2, '2026-10-10', '2026-10-10 09:00-06', '2026-10-10 17:00-06', 'closed')`, defaultCompanyID, u); err != nil {
		t.Fatal(err)
	}
	authSvc := app.NewAuthService(a.st, a.jm, nil)
	cajero := roleIDOf(t, a.st, defaultCompanyID, "cajero")
	setActive := func(active bool) {
		t.Helper()
		if code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", u), a.admin, map[string]any{
			"name": "Vuelve", "roleId": cajero, "isActive": active}); code != http.StatusOK {
			t.Fatalf("isActive=%v = %d %v", active, code, body)
		}
	}

	setActive(false)
	if _, err := authSvc.Login(ctx, "vuelve_a_entrar", "gatobobah", "Contrasena-Larga-1!"); err == nil {
		t.Fatal("dada de baja y entra")
	}
	setActive(true)
	if _, err := authSvc.Login(ctx, "vuelve_a_entrar", "gatobobah", "Contrasena-Larga-1!"); err != nil {
		t.Fatalf("reactivada y no entra: %v", err)
	}
	var n int
	if err := a.st.Pool.QueryRow(ctx, `select count(*) from attendance_entries where user_id = $1`, u).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("asistencia previa = %d renglones, quiere 1", n)
	}
}

// La lista de bajas no alcanza a las de otra empresa, ni con una conexión reciclada ni sin empresa.
func TestInactiveUsersAreIsolated(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	dueña := makeCompany(t, owner, "bajas-duena")
	ajena := makeCompany(t, owner, "bajas-ajena")
	baja := makeUserIn(t, owner, dueña, "baja_de_la_duena", "cajero")
	if _, err := owner.Pool.Exec(context.Background(), `update users set is_active = false where id = $1`, baja); err != nil {
		t.Fatal(err)
	}
	inTheThreeCases(t, dueña, ajena, func(t *testing.T, st *store.Store, ctx context.Context) {
		got, err := app.NewUsersService(st, nil, false, "").List(ctx, domain.UsersInactive)
		if err != nil {
			return // sin empresa la consulta puede negarse: lo que no puede es devolver filas ajenas
		}
		for _, u := range got {
			if u.ID == baja {
				t.Fatalf("la lista de bajas trae a %d, de otra empresa", baja)
			}
		}
	})
}
