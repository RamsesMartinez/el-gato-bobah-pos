//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// ROLES POR EMPRESA EN LA BASE (spec 068, 0108).

// roleIDOf es el id del sugerido `code` en la empresa.
func roleIDOf(t *testing.T, st *store.Store, company int64, code string) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(), `
		select r.id from roles r join system_roles s on s.id = r.system_role_id
		 where r.company_id = $1 and s.code = $2`, company, code).Scan(&id); err != nil {
		t.Fatalf("rol %s de la empresa %d: %v", code, company, err)
	}
	return id
}

// El catálogo de la base y el del dominio dicen lo mismo: códigos, áreas y lo que el producto le da
// a cada sugerido. Un permiso nuevo en Go sin su fila en la migración (o al revés) truena aquí.
func TestThePermissionCatalogMatchesTheDomain(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()

	rows, err := st.Pool.Query(ctx, `select code, area from permissions order by id`)
	if err != nil {
		t.Fatal(err)
	}
	var codes []domain.Permission
	for rows.Next() {
		var code, area string
		if err := rows.Scan(&code, &area); err != nil {
			t.Fatal(err)
		}
		codes = append(codes, domain.Permission(code))
		if area != domain.PermissionArea(domain.Permission(code)) {
			t.Errorf("%s: área %q en la base, %q en el dominio", code, area, domain.PermissionArea(domain.Permission(code)))
		}
	}
	rows.Close()
	if !slices.Equal(codes, domain.AllPermissions()) {
		t.Errorf("la base tiene %v\nel dominio %v", codes, domain.AllPermissions())
	}

	for _, r := range []domain.Role{domain.RoleAdmin, domain.RoleGerente, domain.RoleCajero, domain.RoleMesero} {
		var got []domain.Permission
		rows, err := st.Pool.Query(ctx, `
			select p.code from system_role_permissions sp
			  join system_roles s on s.id = sp.system_role_id
			  join permissions p on p.id = sp.permission_id
			 where s.code = $1`, string(r))
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var c string
			_ = rows.Scan(&c)
			got = append(got, domain.Permission(c))
		}
		rows.Close()
		want := domain.PermissionsFor(r)
		if len(got) != len(want) {
			t.Errorf("%s: la base le da %d permisos, el dominio %d", r, len(got), len(want))
		}
		for _, p := range want {
			if !slices.Contains(got, p) {
				t.Errorf("%s: el dominio le da %s y la base no", r, p)
			}
		}
	}
}

// Toda empresa tiene sus cuatro sugeridos, y una empresa nueva nace con ellos.
func TestEveryCompanyHasItsFourSuggestedRoles(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	nueva := makeCompany(t, st, "roles-nueva")
	for _, co := range []int64{defaultCompanyID, nueva} {
		var n int
		if err := st.Pool.QueryRow(ctx,
			`select count(*) from roles where company_id = $1 and system_role_id is not null and active`, co).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 4 {
			t.Errorf("la empresa %d tiene %d sugeridos activos, quiere 4", co, n)
		}
	}
}

// CONVIVENCIA CON LA API ANTERIOR (0108 hasta la 0109): la anterior escribe `role`, la nueva
// `role_id`, y el trigger mantiene los dos coherentes. Como owner y como la aplicación.
func TestTheUserRoleColumnsStayInSync(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	co := int64(defaultCompanyID)
	cajero := roleIDOf(t, st, co, "cajero")
	gerente := roleIDOf(t, st, co, "gerente")
	var propio int64
	if err := st.Pool.QueryRow(ctx,
		`insert into roles (company_id, name) values ($1, 'Encargado de turno') returning id`, co).Scan(&propio); err != nil {
		t.Fatal(err)
	}

	read := func(t *testing.T, id int64) (string, int64) {
		t.Helper()
		var role string
		var roleID int64
		if err := st.Pool.QueryRow(ctx, `select role::text, role_id from users where id = $1`, id).Scan(&role, &roleID); err != nil {
			t.Fatal(err)
		}
		return role, roleID
	}

	t.Run("la API anterior inserta solo role", func(t *testing.T) {
		id := makeUserIn(t, st, co, "sync_viejo", "cajero")
		if role, rid := read(t, id); role != "cajero" || rid != cajero {
			t.Fatalf("= %s/%d, quiere cajero/%d", role, rid, cajero)
		}
	})
	t.Run("la API nueva inserta solo role_id", func(t *testing.T) {
		var id int64
		if err := st.Pool.QueryRow(ctx,
			`insert into users (company_id, name, username, role_id) values ($1, 'Nueva', 'sync_nuevo', $2) returning id`,
			co, gerente).Scan(&id); err != nil {
			t.Fatalf("insert sin role: %v", err)
		}
		if role, _ := read(t, id); role != "gerente" {
			t.Fatalf("role = %s, quiere gerente", role)
		}
	})
	t.Run("un rol propio se ve como mesero para la API anterior", func(t *testing.T) {
		var id int64
		if err := st.Pool.QueryRow(ctx,
			`insert into users (company_id, name, username, role_id) values ($1, 'Propio', 'sync_propio', $2) returning id`,
			co, propio).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if role, _ := read(t, id); role != "mesero" {
			t.Fatalf("role = %s, quiere mesero (lo mínimo)", role)
		}
	})
	t.Run("la API anterior cambia role y role_id lo sigue", func(t *testing.T) {
		id := makeUserIn(t, st, co, "sync_cambia", "cajero")
		if _, err := st.Pool.Exec(ctx, `update users set role = 'gerente' where id = $1`, id); err != nil {
			t.Fatal(err)
		}
		if _, rid := read(t, id); rid != gerente {
			t.Fatalf("role_id = %d, quiere %d", rid, gerente)
		}
	})
	t.Run("si cambian los dos, gana role_id", func(t *testing.T) {
		id := makeUserIn(t, st, co, "sync_dos", "cajero")
		if _, err := st.Pool.Exec(ctx, `update users set role = 'mesero', role_id = $2 where id = $1`, id, gerente); err != nil {
			t.Fatal(err)
		}
		if role, rid := read(t, id); role != "gerente" || rid != gerente {
			t.Fatalf("= %s/%d, quiere gerente/%d", role, rid, gerente)
		}
	})
	t.Run("una empresa sin roles los recibe al dar de alta a alguien", func(t *testing.T) {
		var sinRoles int64
		if err := st.Pool.QueryRow(ctx,
			`insert into companies (slug, name) values ('sin-roles', 'Sin roles') returning id`).Scan(&sinRoles); err != nil {
			t.Fatal(err)
		}
		id := makeUserIn(t, st, sinRoles, "sync_sin_roles", "admin")
		if _, rid := read(t, id); rid != roleIDOf(t, st, sinRoles, "admin") {
			t.Fatalf("role_id = %d: el admin de una empresa creada sin roles se quedaría sin permisos", rid)
		}
	})
	t.Run("con search_path vacío, como pg_restore", func(t *testing.T) {
		conn, err := st.Pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Release()
		if _, err := conn.Exec(ctx, `select set_config('search_path', '', false)`); err != nil {
			t.Fatal(err)
		}
		defer func() { _, _ = conn.Exec(ctx, `reset search_path`) }()
		if _, err := conn.Exec(ctx,
			`insert into public.users (company_id, name, username, role) values ($1, 'Restaurada', 'sync_restore', 'mesero')`, co); err != nil {
			t.Fatalf("el trigger truena sin search_path: %v", err)
		}
	})
	t.Run("como la aplicación, bajo RLS", func(t *testing.T) {
		conn := conexionDeEmpresa(t, appRoleStore(t), co)
		var id int64
		if err := conn.QueryRow(ctx,
			`insert into users (name, username, role) values ('App', 'sync_app', 'cajero') returning id`).Scan(&id); err != nil {
			t.Fatalf("insert como la aplicación: %v", err)
		}
		if _, rid := read(t, id); rid != cajero {
			t.Fatalf("role_id = %d, quiere %d", rid, cajero)
		}
	})
}

// Una empresa no puede asignar a su empleado un rol de otra, ni siquiera como owner: la llave
// foránea compuesta lo impide aunque RLS fallara (las llaves foráneas saltan RLS).
func TestARoleOfAnotherCompanyCannotBeAssigned(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	otra := makeCompany(t, st, "roles-ajena")
	id := makeUserIn(t, st, defaultCompanyID, "fk_ajena", "cajero")
	_, err := st.Pool.Exec(ctx, `update users set role_id = $2 where id = $1`, id, roleIDOf(t, st, otra, "admin"))
	exigeViolacionDeRestriccion(t, err, "asignar el rol admin de otra empresa")
}

// El admin no se inactiva, tampoco escribiendo directo en la base.
func TestTheAdminRoleCannotBeDeactivatedInTheDatabase(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	_, err := st.Pool.Exec(context.Background(),
		`update roles set active = false where id = $1`, roleIDOf(t, st, defaultCompanyID, "admin"))
	exigeViolacionDeRestriccion(t, err, "inactivar al admin")
}

// AISLAMIENTO: los roles, sus ajustes y lo que puede cada usuario no cruzan empresas, en los tres
// casos donde RLS ya falló.
func TestRolesAreIsolated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	owner := int64(defaultCompanyID)
	other := makeCompany(t, st, "roles-otra")
	var propio int64
	if err := st.Pool.QueryRow(ctx,
		`insert into roles (company_id, name) values ($1, 'Secreto') returning id`, owner).Scan(&propio); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into role_permission_overrides (company_id, role_id, permission_id, granted)
		select $1, $2, id, true from permissions where code = 'sales.view'`, owner, propio); err != nil {
		t.Fatal(err)
	}
	u := makeUserIn(t, st, owner, "aislado", "cajero")

	inTheThreeCases(t, owner, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		roles, err := st.QC(ctx).ListRoles(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range roles {
			if r.CompanyID == owner {
				t.Errorf("ve el rol %d de otra empresa", r.ID)
			}
		}
		ovs, err := st.QC(ctx).ListRoleOverrides(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range ovs {
			if o.CompanyID == owner {
				t.Errorf("ve un ajuste de otra empresa: %+v", o)
			}
		}
		perms, err := st.QC(ctx).UserEffectivePermissions(ctx, u)
		if err != nil {
			t.Fatal(err)
		}
		if len(perms) != 0 {
			t.Errorf("resuelve %d permisos de un usuario de otra empresa", len(perms))
		}
	})
}

// Un usuario cuyo rol está inactivo no puede nada que pida permiso (FR-017).
func TestAUserWithAnInactiveRoleHasNoPermissions(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	u := makeUserIn(t, st, defaultCompanyID, "rol_inactivo", "cajero")
	if _, err := st.Pool.Exec(ctx, `update roles set active = false where id = $1`,
		roleIDOf(t, st, defaultCompanyID, "cajero")); err != nil {
		t.Fatal(err)
	}
	app := appRoleStore(t)
	tctx, release, err := app.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	perms, err := app.QC(tctx).UserEffectivePermissions(tctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if len(perms) != 0 {
		t.Fatalf("con el rol inactivo tiene %v", perms)
	}
}
