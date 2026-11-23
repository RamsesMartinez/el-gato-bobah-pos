//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

const incomingOrdersPermissionVersion = 117

// LA 0117 (permiso de pedidos entrantes, spec 093) CONTRA UN RESPALDO REAL CON DOS EMPRESAS. Solo
// agrega filas, pero dos de ellas son la parte delicada: el permiso llega por herencia a los
// sugeridos, y a los roles PROPIOS que ya existían se les da una vez —hasta hoy cualquier sesión
// veía y aceptaba pedidos de plataforma, y un rol propio no hereda permisos nuevos—. Sin eso, sus
// usuarios dejarían de oír los pedidos al subir.
func TestMigration0117OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	exigeDosEmpresas(t, st)
	migrarAbajoHasta(t, st.Pool, incomingOrdersPermissionVersion-1)
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	// Un rol propio por empresa: con cero, el insert de la migración no hace nada y la prueba pasaría
	// en blanco.
	if _, err := st.Pool.Exec(ctx, `
		insert into roles (company_id, name)
		select c.id, 'Repartidor 093' from companies c
		 where not exists (select 1 from roles r where r.company_id = c.id and r.name = 'Repartidor 093')`); err != nil {
		t.Fatal(err)
	}
	type rol struct{ company, id int64 }
	propios := func() []rol {
		var out []rol
		rows, err := st.Pool.Query(ctx, `select company_id, id from roles where system_role_id is null order by id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var r rol
			if err := rows.Scan(&r.company, &r.id); err != nil {
				t.Fatal(err)
			}
			out = append(out, r)
		}
		return out
	}
	antes := propios()
	empresas := map[int64]bool{}
	for _, r := range antes {
		empresas[r.company] = true
	}
	if len(empresas) < 2 {
		t.Fatalf("roles propios en %d empresas; la prueba necesita dos", len(empresas))
	}

	up := func() {
		t.Helper()
		if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", incomingOrdersPermissionVersion); err != nil {
			t.Fatal(err)
		}
	}
	up()

	t.Run("llega por herencia a los cuatro sugeridos", func(t *testing.T) {
		var n int
		if err := st.Pool.QueryRow(ctx, `
			select count(*) from system_role_permissions sp join permissions p on p.id = sp.permission_id
			 where p.code = 'orders.accept_incoming'`).Scan(&n); err != nil || n != 4 {
			t.Fatalf("orders.accept_incoming en %d roles sugeridos: %v", n, err)
		}
	})

	t.Run("cada rol propio que ya existía lo conserva, en su empresa", func(t *testing.T) {
		for _, r := range antes {
			var company int64
			var granted bool
			err := st.Pool.QueryRow(ctx, `
				select company_id, granted from role_permission_overrides where role_id = $1 and permission_id = 39`,
				r.id).Scan(&company, &granted)
			if err != nil {
				t.Fatalf("el rol propio %d se quedó sin el permiso: sus usuarios dejarían de oír los pedidos (%v)", r.id, err)
			}
			if company != r.company || !granted {
				t.Fatalf("rol %d: override con empresa %d (debía %d) y granted %v", r.id, company, r.company, granted)
			}
		}
	})

	t.Run("un mesero de cualquier empresa lo tiene", func(t *testing.T) {
		var user int64
		if err := st.Pool.QueryRow(ctx, `
			select u.id from users u join roles r on r.id = u.role_id and r.system_role_id = 4 limit 1`).Scan(&user); err != nil {
			t.Skip("el respaldo no trae meseros")
		}
		perms, err := db.New(st.Pool).UserEffectivePermissions(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(perms, "orders.accept_incoming") {
			t.Fatalf("permisos efectivos del mesero sin orders.accept_incoming: %v", perms)
		}
	})

	t.Run("un rol propio creado después no lo hereda", func(t *testing.T) {
		var id int64
		if err := st.Pool.QueryRow(ctx, `
			insert into roles (company_id, name) select min(id), 'Nuevo 093' from companies returning id`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from role_permission_overrides where role_id = $1`, id).Scan(&n); err != nil || n != 0 {
			t.Fatalf("el rol propio nuevo trae %d ajustes: %v", n, err)
		}
		if _, err := st.Pool.Exec(ctx, `delete from roles where id = $1`, id); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("bajar y volver a subir deja lo mismo", func(t *testing.T) {
		var n1, n2 int
		contar := `select count(*) from role_permission_overrides where permission_id = 39`
		if err := st.Pool.QueryRow(ctx, contar).Scan(&n1); err != nil {
			t.Fatal(err)
		}
		if err := goose.DownContext(ctx, prepararGoose(t, st.Pool), "."); err != nil {
			t.Fatalf("el Down se negó sin que ninguna empresa quitara el permiso: %v", err)
		}
		var existe int
		if err := st.Pool.QueryRow(ctx, `select count(*) from permissions where id = 39`).Scan(&existe); err != nil || existe != 0 {
			t.Fatalf("el permiso 39 sigue tras el Down: %d %v", existe, err)
		}
		up()
		if err := st.Pool.QueryRow(ctx, contar).Scan(&n2); err != nil || n1 != n2 {
			t.Fatalf("ajustes del 39: %d antes y %d tras bajar y subir (%v)", n1, n2, err)
		}
	})

	t.Run("el Down se niega si una empresa se lo quitó a un rol, y su limpieza lo deja bajar", func(t *testing.T) {
		if _, err := st.Pool.Exec(ctx, `
			insert into role_permission_overrides (company_id, role_id, permission_id, granted)
			select company_id, id, 39, false from roles where system_role_id = 4 limit 1`); err != nil {
			t.Fatal(err)
		}
		if err := goose.DownContext(ctx, prepararGoose(t, st.Pool), "."); err == nil {
			t.Fatal("el Down bajó y borró la decisión de una empresa de quitarle el permiso a un rol")
		}
		migrarAbajoHasta(t, st.Pool, incomingOrdersPermissionVersion-1)
	})
}
