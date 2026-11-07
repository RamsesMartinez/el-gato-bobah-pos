//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// ADMINISTRAR ROLES POR LA RUTA (spec 068, US2–US5).

type rolesAPI struct {
	t      *testing.T
	st     *store.Store
	r      http.Handler
	jm     *auth.Manager
	admin  string
	adminU int64
}

func newRolesAPI(t *testing.T) *rolesAPI {
	t.Helper()
	st := newTestStore(t)
	jm := auth.NewManager(secretoDelNegocioEnPruebas, nil)
	h := httpapi.NewHandlers(httpapi.Deps{JWT: jm, Broker: realtime.NewBroker(),
		Users: app.NewUsersService(st, nil, false, "pepper-de-prueba")})
	api := &rolesAPI{t: t, st: st, r: httpapi.Router(config.Config{}, jm, h, st), jm: jm}
	api.adminU = makeUser(t, st, "roles_admin", "admin")
	api.admin = api.token(api.adminU)
	return api
}

func (a *rolesAPI) token(id int64) string {
	a.t.Helper()
	tok, err := a.jm.Issue(domain.User{ID: id, CompanyID: defaultCompanyID, Name: "u"})
	if err != nil {
		a.t.Fatal(err)
	}
	return tok
}

func (a *rolesAPI) call(method, path, tok string, body any) (int, map[string]any) {
	a.t.Helper()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	w := do(a.t, a.r, method, path, tok, raw, "application/json")
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

type roleJSON struct {
	ID          int64    `json:"id"`
	SystemCode  string   `json:"systemCode"`
	Name        string   `json:"name"`
	Active      bool     `json:"active"`
	Version     int32    `json:"version"`
	ActiveUsers int64    `json:"activeUsers"`
	AllUsers    int64    `json:"allUsers"`
	Permissions []string `json:"permissions"`
	Added       []string `json:"added"`
	Removed     []string `json:"removed"`
}

func (a *rolesAPI) roles() map[string]roleJSON {
	a.t.Helper()
	w := do(a.t, a.r, http.MethodGet, "/api/v1/roles", a.admin, nil, "")
	if w.Code != http.StatusOK {
		a.t.Fatalf("GET /roles = %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Items []roleJSON `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		a.t.Fatal(err)
	}
	out := map[string]roleJSON{}
	for _, r := range body.Items {
		if r.Permissions == nil || r.Added == nil || r.Removed == nil {
			a.t.Fatalf("el rol %s trae una lista null", r.Name)
		}
		out[r.Name] = r
	}
	return out
}

func errCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func (a *rolesAPI) auditCount(action string) int {
	a.t.Helper()
	var n int
	if err := a.st.Pool.QueryRow(context.Background(),
		`select count(*) from audit_events where action = $1`, action).Scan(&n); err != nil {
		a.t.Fatal(err)
	}
	return n
}

// US2: editar lo que puede un sugerido aplica a la siguiente petición, sin renovar sesión.
func TestEditingASuggestedRoleAppliesToTheNextRequest(t *testing.T) {
	t.Parallel()
	a := newRolesAPI(t)
	cajeroU := makeUser(t, a.st, "roles_cajera", "cajero")
	cajeraTok := a.token(cajeroU)
	cajero := a.roles()["Cajero"]

	if code, _ := a.call(http.MethodGet, "/api/v1/sales", cajeraTok, nil); code != http.StatusForbidden {
		t.Fatalf("antes: ventas = %d, quiere 403", code)
	}
	code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/roles/%d", cajero.ID), a.admin, map[string]any{
		"version": cajero.Version, "add": []string{"sales.view"}, "remove": []string{"orders.delivered_view"}})
	if code != http.StatusOK {
		t.Fatalf("PATCH = %d: %v", code, body)
	}
	if code, _ := a.call(http.MethodGet, "/api/v1/sales", cajeraTok, nil); code == http.StatusForbidden {
		t.Fatal("con ventas agregado al cajero, su siguiente petición sigue negada")
	}
	if code, _ := a.call(http.MethodGet, "/api/v1/orders/delivered", cajeraTok, nil); code != http.StatusForbidden {
		t.Fatalf("con entregados quitado, la siguiente petición = %d, quiere 403", code)
	}
	c := a.roles()["Cajero"]
	if !slices.Contains(c.Added, "sales.view") || !slices.Contains(c.Removed, "orders.delivered_view") {
		t.Errorf("agregados %v, quitados %v", c.Added, c.Removed)
	}
	if a.auditCount("role.permissions_changed") != 1 {
		t.Error("el cambio de permisos no dejó su evento role.permissions_changed")
	}

	t.Run("con la versión vieja se rechaza", func(t *testing.T) {
		code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/roles/%d", cajero.ID), a.admin, map[string]any{
			"version": cajero.Version, "add": []string{"reports.view"}})
		if code != http.StatusConflict || errCode(body) != "ROLE_CHANGED" {
			t.Fatalf("= %d %v, quiere 409 ROLE_CHANGED", code, body)
		}
	})
	t.Run("volver a lo sugerido", func(t *testing.T) {
		c := a.roles()["Cajero"]
		if code, body := a.call(http.MethodPost, fmt.Sprintf("/api/v1/roles/%d/reset", c.ID), a.admin, map[string]any{"version": c.Version}); code != http.StatusOK {
			t.Fatalf("reset = %d %v", code, body)
		}
		c = a.roles()["Cajero"]
		if len(c.Added)+len(c.Removed) != 0 {
			t.Fatalf("después de volver a lo sugerido quedan ajustes: +%v −%v", c.Added, c.Removed)
		}
		if a.auditCount("role.reset") != 1 {
			t.Error("sin evento role.reset")
		}
	})
	t.Run("el admin no pierde la administración de roles", func(t *testing.T) {
		ad := a.roles()["Admin"]
		code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/roles/%d", ad.ID), a.admin, map[string]any{
			"version": ad.Version, "remove": []string{"roles.manage"}})
		if code != http.StatusConflict || errCode(body) != "ROLE_ADMIN_KEEPS_ROLES" {
			t.Fatalf("= %d %v", code, body)
		}
	})
	t.Run("un cajero no administra roles", func(t *testing.T) {
		if code, _ := a.call(http.MethodGet, "/api/v1/roles", cajeraTok, nil); code != http.StatusForbidden {
			t.Fatalf("= %d, quiere 403", code)
		}
	})
}

// HERENCIA: un permiso que el producto le da a un sugerido DESPUÉS llega a las empresas que no lo
// quitaron, y una pantalla que guardó sin conocerlo no lo quita.
func TestANewProductPermissionReachesTheSuggestedRole(t *testing.T) {
	t.Parallel()
	a := newRolesAPI(t)
	ctx := context.Background()
	cajeroU := makeUser(t, a.st, "herencia_cajero", "cajero")
	cajero := a.roles()["Cajero"]
	// La pantalla se abrió antes del cambio de producto y guarda solo lo que tocó.
	if code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/roles/%d", cajero.ID), a.admin, map[string]any{
		"version": cajero.Version, "add": []string{"sales.view"}}); code != http.StatusOK {
		t.Fatalf("PATCH = %d %v", code, body)
	}
	// El producto le da «reportes» al cajero (lo que haría una migración futura).
	if _, err := a.st.Pool.Exec(ctx, `insert into system_role_permissions (system_role_id, permission_id)
		select 3, id from permissions where code = 'reports.view'`); err != nil {
		t.Fatal(err)
	}
	perms, err := app.NewRolesService(a.st).EffectivePermissions(ctx, cajeroU)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(perms, domain.PermReportsView) || !slices.Contains(perms, domain.PermSalesView) {
		t.Fatalf("el cajero tiene %v: le falta lo que el producto agregó o lo que la empresa agregó", perms)
	}
}

// US3: un rol propio, su nombre y su asignación.
func TestACustomRoleIsCreatedAndAssigned(t *testing.T) {
	t.Parallel()
	a := newRolesAPI(t)
	code, body := a.call(http.MethodPost, "/api/v1/roles", a.admin, map[string]any{
		"name": "  Encargado   de turno ", "permissions": []string{"cash.operate", "sales.view"}})
	if code != http.StatusCreated {
		t.Fatalf("POST = %d %v", code, body)
	}
	enc := a.roles()["Encargado de turno"]
	if enc.ID == 0 || enc.SystemCode != "" || !enc.Active {
		t.Fatalf("rol creado: %+v", enc)
	}
	if a.auditCount("role.created") != 1 {
		t.Error("sin evento role.created")
	}
	for _, name := range []string{"encargado de turno", "CÁJERO"} {
		code, body := a.call(http.MethodPost, "/api/v1/roles", a.admin, map[string]any{"name": name, "permissions": []string{}})
		if code != http.StatusConflict || errCode(body) != "ROLE_NAME_TAKEN" {
			t.Errorf("crear %q = %d %v, quiere ROLE_NAME_TAKEN", name, code, body)
		}
	}

	u := makeUser(t, a.st, "encargada", "cajero")
	code, body = a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", u), a.admin, map[string]any{
		"name": "Encargada", "roleId": enc.ID, "isActive": true})
	if code != http.StatusOK {
		t.Fatalf("asignar = %d %v", code, body)
	}
	if a.auditCount("user.role_changed") != 1 {
		t.Error("sin evento user.role_changed")
	}
	tok := a.token(u)
	if code, _ := a.call(http.MethodGet, "/api/v1/sales", tok, nil); code == http.StatusForbidden {
		t.Error("el rol propio trae ventas y se le niega")
	}
	if code, _ := a.call(http.MethodGet, "/api/v1/orders/delivered", tok, nil); code != http.StatusForbidden {
		t.Error("el rol propio no trae entregados y se le permite")
	}

	t.Run("renombrar un propio, y no a uno repetido", func(t *testing.T) {
		enc := a.roles()["Encargado de turno"]
		if code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/roles/%d", enc.ID), a.admin, map[string]any{
			"version": enc.Version, "name": "Encargada de turno"}); code != http.StatusOK {
			t.Fatalf("renombrar = %d %v", code, body)
		}
		enc = a.roles()["Encargada de turno"]
		code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/roles/%d", enc.ID), a.admin, map[string]any{
			"version": enc.Version, "name": "Mesero"})
		if code != http.StatusConflict || errCode(body) != "ROLE_NAME_TAKEN" {
			t.Fatalf("renombrar a Mesero = %d %v", code, body)
		}
		if a.auditCount("role.renamed") != 1 {
			t.Error("sin evento role.renamed")
		}
	})
	t.Run("un sugerido no se renombra", func(t *testing.T) {
		m := a.roles()["Mesero"]
		code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/roles/%d", m.ID), a.admin, map[string]any{
			"version": m.Version, "name": "Mesera"})
		if code != http.StatusConflict || errCode(body) != "ROLE_SYSTEM" {
			t.Fatalf("= %d %v", code, body)
		}
	})
	t.Run("un rol de otra empresa no se asigna", func(t *testing.T) {
		otra := makeCompany(t, a.st, "roles-http-otra")
		ajeno := roleIDOf(t, a.st, otra, "admin")
		code, _ := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", u), a.admin, map[string]any{
			"name": "Encargada", "roleId": ajeno, "isActive": true})
		if code == http.StatusOK {
			t.Fatal("asignó el rol admin de otra empresa")
		}
	})
}

// US4: inactivar, reactivar y borrar.
func TestDeactivatingAndDeletingRoles(t *testing.T) {
	t.Parallel()
	a := newRolesAPI(t)
	_, _ = a.call(http.MethodPost, "/api/v1/roles", a.admin, map[string]any{"name": "Barista", "permissions": []string{"orders.move_lines"}})
	_, _ = a.call(http.MethodPost, "/api/v1/roles", a.admin, map[string]any{"name": "Repartidor", "permissions": []string{}})
	roles := a.roles()
	barista, repartidor := roles["Barista"], roles["Repartidor"]
	u := makeUser(t, a.st, "barista", "cajero")
	if code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", u), a.admin, map[string]any{
		"name": "Barista", "roleId": barista.ID, "isActive": true}); code != http.StatusOK {
		t.Fatalf("asignar = %d %v", code, body)
	}

	deact := func(r roleJSON) (int, map[string]any) {
		r = a.roles()[r.Name]
		return a.call(http.MethodPost, fmt.Sprintf("/api/v1/roles/%d/deactivate", r.ID), a.admin, map[string]any{"version": r.Version})
	}
	if code, body := deact(barista); code != http.StatusConflict || errCode(body) != "ROLE_IN_USE" {
		t.Fatalf("inactivar con una persona activa = %d %v", code, body)
	}
	if code, body := deact(a.roles()["Admin"]); code != http.StatusConflict || errCode(body) != "ROLE_ADMIN_KEEPS_ROLES" {
		t.Fatalf("inactivar al admin = %d %v", code, body)
	}
	// Se da de baja a la persona: ya se puede inactivar, pero no borrar (la tuvo).
	if code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", u), a.admin, map[string]any{
		"name": "Barista", "roleId": barista.ID, "isActive": false}); code != http.StatusOK {
		t.Fatalf("dar de baja = %d %v", code, body)
	}
	if code, body := deact(barista); code != http.StatusOK {
		t.Fatalf("inactivar sin activos = %d %v", code, body)
	}
	if a.auditCount("role.deactivated") != 1 {
		t.Error("sin evento role.deactivated")
	}
	if code, body := a.call(http.MethodDelete, fmt.Sprintf("/api/v1/roles/%d", barista.ID), a.admin, nil); code != http.StatusConflict || errCode(body) != "ROLE_IN_USE" {
		t.Fatalf("borrar uno que tuvo usuario = %d %v", code, body)
	}
	t.Run("reactivar a la persona con un rol inactivo se rechaza", func(t *testing.T) {
		code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", u), a.admin, map[string]any{
			"name": "Barista", "roleId": barista.ID, "isActive": true})
		if code != http.StatusConflict || errCode(body) != "ROLE_INACTIVE" {
			t.Fatalf("= %d %v", code, body)
		}
	})
	t.Run("reactivar el rol conserva sus permisos", func(t *testing.T) {
		b := a.roles()["Barista"]
		if code, body := a.call(http.MethodPost, fmt.Sprintf("/api/v1/roles/%d/reactivate", b.ID), a.admin, map[string]any{"version": b.Version}); code != http.StatusOK {
			t.Fatalf("= %d %v", code, body)
		}
		if b := a.roles()["Barista"]; !b.Active || !slices.Equal(b.Permissions, []string{"orders.move_lines"}) {
			t.Fatalf("después de reactivar: %+v", b)
		}
		if a.auditCount("role.reactivated") != 1 {
			t.Error("sin evento role.reactivated")
		}
	})
	if code, body := a.call(http.MethodDelete, fmt.Sprintf("/api/v1/roles/%d", a.roles()["Cajero"].ID), a.admin, nil); code != http.StatusConflict || errCode(body) != "ROLE_SYSTEM" {
		t.Fatalf("borrar un sugerido = %d %v", code, body)
	}
	if code, body := a.call(http.MethodDelete, fmt.Sprintf("/api/v1/roles/%d", repartidor.ID), a.admin, nil); code != http.StatusNoContent {
		t.Fatalf("borrar un propio sin usuarios = %d %v", code, body)
	}
	if a.auditCount("role.deleted") != 1 {
		t.Error("sin evento role.deleted")
	}
}

// NUNCA SIN QUIEN ADMINISTRE ROLES (FR-012): ni cambiando de rol ni dando de baja al último, ni con
// dos bajas a la vez de los dos únicos.
func TestTheLastRoleAdminCannotBeRemoved(t *testing.T) {
	t.Parallel()
	a := newRolesAPI(t)
	ctx := context.Background()
	// En la empresa de prueba puede haber otros admins sembrados: se dejan solo los de esta prueba.
	if _, err := a.st.Pool.Exec(ctx, `update users set is_active = false where company_id = $1 and id <> $2`, defaultCompanyID, a.adminU); err != nil {
		t.Fatal(err)
	}
	cajero := a.roles()["Cajero"]
	code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", a.adminU), a.admin, map[string]any{
		"name": "Admin", "roleId": cajero.ID, "isActive": true})
	if code != http.StatusConflict || errCode(body) != "LAST_ROLE_ADMIN" {
		t.Fatalf("cambiar de rol al último = %d %v", code, body)
	}
	code, body = a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", a.adminU), a.admin, map[string]any{
		"name": "Admin", "roleId": a.roles()["Admin"].ID, "isActive": false})
	if code != http.StatusConflict || errCode(body) != "LAST_ROLE_ADMIN" {
		t.Fatalf("dar de baja al último = %d %v", code, body)
	}

	segunda := makeUser(t, a.st, "segunda_admin", "admin")
	adminRole := a.roles()["Admin"].ID
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i, id := range []int64{a.adminU, segunda} {
		wg.Add(1)
		go func(i int, id int64) {
			defer wg.Done()
			w := do(t, a.r, http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", id), a.admin,
				[]byte(fmt.Sprintf(`{"name":"x","roleId":%d,"isActive":false}`, adminRole)), "application/json")
			codes[i] = w.Code
		}(i, id)
	}
	wg.Wait()
	var activos int
	if err := a.st.Pool.QueryRow(ctx, `select count(*) from users u join roles r on r.id = u.role_id
		where u.is_active and r.system_role_id = 1 and u.company_id = $1`, defaultCompanyID).Scan(&activos); err != nil {
		t.Fatal(err)
	}
	if activos < 1 {
		t.Fatalf("dos bajas a la vez dejaron a la empresa sin administradores (respuestas %v)", codes)
	}
}

// Editar el nombre de alguien no depende de que quede un administrador si no se tocó rol ni alta:
// una empresa que ya estaba sin administradores (alta a mano) no se queda sin poder corregir nada.
func TestEditingANameDoesNotRequireARoleAdminIfNothingChanged(t *testing.T) {
	t.Parallel()
	a := newRolesAPI(t)
	ctx := context.Background()
	u := makeUser(t, a.st, "solo_nombre", "cajero")
	// La empresa se queda sin nadie que administre roles (como la dejaría un alta a mano).
	if _, err := a.st.Pool.Exec(ctx, `update users set is_active = false where company_id = $1 and id <> $2`, defaultCompanyID, u); err != nil {
		t.Fatal(err)
	}
	svc := app.NewUsersService(a.st, nil, false, "pepper-de-prueba")
	tctx, release, err := appRoleStore(t).AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	_ = svc
	appSvc := app.NewUsersService(appRoleStore(t), nil, false, "pepper-de-prueba")
	cajero := roleIDOf(t, a.st, defaultCompanyID, "cajero")
	if _, err := appSvc.Update(tctx, app.UpdateUserInput{ID: u, Name: "Nombre nuevo", RoleID: &cajero, IsActive: true, ActorPerms: domain.AllPermissions()}); err != nil {
		t.Fatalf("cambiar solo el nombre: %v", err)
	}
}

// DURANTE LA SUBIDA, una tableta con la pantalla anterior manda `role` (el código que ve) al editar a
// alguien con rol propio; ese `role` es «mesero» (lo que el trigger le puso). No se le reasigna el
// sugerido: se conserva su rol propio.
func TestTheOldScreenDoesNotReassignACustomRole(t *testing.T) {
	t.Parallel()
	a := newRolesAPI(t)
	ctx := context.Background()
	_, _ = a.call(http.MethodPost, "/api/v1/roles", a.admin, map[string]any{"name": "Barista", "permissions": []string{"orders.move_lines"}})
	barista := a.roles()["Barista"].ID
	u := makeUser(t, a.st, "barista_vieja", "cajero")
	if _, err := a.st.Pool.Exec(ctx, `update users set role_id = $2 where id = $1`, u, barista); err != nil {
		t.Fatal(err)
	}
	code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", u), a.admin, map[string]any{
		"name": "Barista vieja", "role": "mesero", "isActive": true})
	if code != http.StatusOK {
		t.Fatalf("= %d %v", code, body)
	}
	var rid int64
	_ = a.st.Pool.QueryRow(ctx, `select role_id from users where id = $1`, u).Scan(&rid)
	if rid != barista {
		t.Fatalf("role_id = %d: la pantalla anterior le quitó su rol propio", rid)
	}
}

// NADIE ADMINISTRA A QUIEN PUEDE MÁS QUE ÉL (auditoría de seguridad de la 068). Un rol propio con solo
// «administrar empleados» podía asignarse el rol admin o resetear la contraseña del admin, y uno con
// «administrar roles» darse cualquier permiso. Se rechaza dar, quitar o asignar lo que uno no tiene.
func TestNobodyManagesSomeoneMorePowerful(t *testing.T) {
	t.Parallel()
	a := newRolesAPI(t)
	_, _ = a.call(http.MethodPost, "/api/v1/roles", a.admin, map[string]any{"name": "Personal", "permissions": []string{"employees.manage"}})
	_, _ = a.call(http.MethodPost, "/api/v1/roles", a.admin, map[string]any{"name": "Roles", "permissions": []string{"roles.manage", "sales.view"}})
	roles := a.roles()
	personal := makeUser(t, a.st, "enc_personal", "cajero")
	rolesU := makeUser(t, a.st, "enc_roles", "cajero")
	for u, r := range map[int64]int64{personal: roles["Personal"].ID, rolesU: roles["Roles"].ID} {
		if code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", u), a.admin, map[string]any{
			"name": "x", "roleId": r, "isActive": true}); code != http.StatusOK {
			t.Fatalf("asignar = %d %v", code, body)
		}
	}
	pt, rt := a.token(personal), a.token(rolesU)
	cajeroU := makeUser(t, a.st, "otro_cajero", "cajero")

	t.Run("no se asigna el rol admin a sí mismo", func(t *testing.T) {
		if code, _ := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", personal), pt, map[string]any{
			"name": "x", "roleId": roles["Admin"].ID, "isActive": true}); code != http.StatusForbidden {
			t.Fatalf("= %d, quiere 403", code)
		}
	})
	t.Run("ni se lo asigna a otro", func(t *testing.T) {
		if code, _ := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", cajeroU), pt, map[string]any{
			"name": "x", "roleId": roles["Admin"].ID, "isActive": true}); code != http.StatusForbidden {
			t.Fatalf("= %d, quiere 403", code)
		}
	})
	t.Run("ni resetea la contraseña del admin", func(t *testing.T) {
		if code, _ := a.call(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/password", a.adminU), pt, map[string]any{
			"password": "Otra-Contrasena-Larga-9!"}); code != http.StatusForbidden {
			t.Fatalf("= %d, quiere 403", code)
		}
	})
	t.Run("ni da de baja al admin", func(t *testing.T) {
		if code, _ := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", a.adminU), pt, map[string]any{
			"name": "x", "roleId": roles["Admin"].ID, "isActive": false}); code != http.StatusForbidden {
			t.Fatalf("= %d, quiere 403", code)
		}
	})
	t.Run("quien administra roles no se da lo que no tiene", func(t *testing.T) {
		r := a.roles()["Roles"]
		if code, _ := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/roles/%d", r.ID), rt, map[string]any{
			"version": r.Version, "add": []string{"payments.void"}}); code != http.StatusForbidden {
			t.Fatalf("= %d, quiere 403", code)
		}
	})
	t.Run("ni le quita al admin", func(t *testing.T) {
		ad := a.roles()["Admin"]
		if code, _ := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/roles/%d", ad.ID), rt, map[string]any{
			"version": ad.Version, "remove": []string{"sales.view"}}); code != http.StatusForbidden {
			t.Fatalf("= %d, quiere 403", code)
		}
	})
	t.Run("pero sí da lo que tiene a un rol que no lo supera", func(t *testing.T) {
		_, _ = a.call(http.MethodPost, "/api/v1/roles", a.admin, map[string]any{"name": "Vacío", "permissions": []string{}})
		m := a.roles()["Vacío"]
		if code, body := a.call(http.MethodPatch, fmt.Sprintf("/api/v1/roles/%d", m.ID), rt, map[string]any{
			"version": m.Version, "add": []string{"sales.view"}}); code != http.StatusOK {
			t.Fatalf("= %d %v", code, body)
		}
	})
}
