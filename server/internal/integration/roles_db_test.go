//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
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

// La convivencia con la API anterior (el trigger sync_user_role) la probó la entrega 2 y se borró
// con la 0109: users.role ya no existe.

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

// La aplicación no crea roles en otra empresa: ensure_company_roles salta RLS (security definer) y
// solo acepta la empresa de la sesión cuando la llama gatobobah_app.
func TestEnsureCompanyRolesOnlyForTheSessionCompany(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	var otra int64
	if err := st.Pool.QueryRow(ctx, `insert into companies (slug, name) values ('ajena-sin-roles', 'Ajena') returning id`).Scan(&otra); err != nil {
		t.Fatal(err)
	}
	conn := conexionDeEmpresa(t, appRoleStore(t), defaultCompanyID)
	if _, err := conn.Exec(ctx, `select ensure_company_roles($1)`, otra); err == nil {
		t.Fatal("la aplicación creó los roles de otra empresa")
	}
	var n int
	_ = st.Pool.QueryRow(ctx, `select count(*) from roles where company_id = $1`, otra).Scan(&n)
	if n != 0 {
		t.Fatalf("la otra empresa quedó con %d roles", n)
	}
	if _, err := conn.Exec(ctx, `select ensure_company_roles($1)`, int64(defaultCompanyID)); err != nil {
		t.Fatalf("la propia empresa: %v", err)
	}
}

// Un sugerido no se borra, tampoco por la consulta directa.
func TestASuggestedRoleCannotBeDeletedByTheQuery(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	conn := conexionDeEmpresa(t, appRoleStore(t), defaultCompanyID)
	q := db.New(conn)
	n, err := q.DeleteRole(ctx, roleIDOf(t, st, defaultCompanyID, "mesero"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("borró el sugerido mesero")
	}
}
