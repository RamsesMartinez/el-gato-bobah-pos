//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

const companyRolesVersion = 108

// LA 0108 CONTRA UN RESPALDO REAL CON DOS EMPRESAS (principio IV). Con una sola empresa, el relleno
// «por cada empresa» sería un no-op y pasaría verde para romper en producción.
func TestMigration0108OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	exigeDosEmpresas(t, st)
	migrarAbajoHasta(t, st.Pool, companyRolesVersion-1)

	// Lo que había antes: cuántos usuarios de cada rol por empresa.
	type key struct {
		company int64
		role    string
	}
	before := map[key]int{}
	rows, err := st.Pool.Query(ctx, `select company_id, role::text, count(*) from users group by 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k key
		var n int
		if err := rows.Scan(&k.company, &k.role, &n); err != nil {
			t.Fatal(err)
		}
		before[k] = n
	}
	rows.Close()
	if len(before) == 0 {
		t.Fatal("el respaldo no trae usuarios")
	}

	migrarArriba(t, st.Pool)

	t.Run("cada empresa tiene sus cuatro sugeridos activos", func(t *testing.T) {
		var faltan int
		if err := st.Pool.QueryRow(ctx, `
			select count(*) from companies c
			 where (select count(*) from roles r where r.company_id = c.id and r.system_role_id is not null and r.active) <> 4`,
		).Scan(&faltan); err != nil {
			t.Fatal(err)
		}
		if faltan != 0 {
			t.Fatalf("%d empresas sin sus cuatro sugeridos", faltan)
		}
	})

	t.Run("cada usuario tiene el sugerido equivalente a su rol, de su empresa", func(t *testing.T) {
		var mal int
		if err := st.Pool.QueryRow(ctx, `
			select count(*) from users u
			  left join roles r on r.id = u.role_id
			  left join system_roles s on s.id = r.system_role_id
			 where r.id is null or r.company_id <> u.company_id or s.code <> u.role::text`).Scan(&mal); err != nil {
			t.Fatal(err)
		}
		if mal != 0 {
			t.Fatalf("%d usuarios sin el rol equivalente", mal)
		}
		after := map[key]int{}
		rows, err := st.Pool.Query(ctx, `
			select u.company_id, s.code, count(*) from users u
			  join roles r on r.id = u.role_id join system_roles s on s.id = r.system_role_id group by 1, 2`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var k key
			var n int
			_ = rows.Scan(&k.company, &k.role, &n)
			after[k] = n
		}
		rows.Close()
		for k, n := range before {
			if after[k] != n {
				t.Errorf("empresa %d, rol %s: %d antes, %d después", k.company, k.role, n, after[k])
			}
		}
	})

	t.Run("el Down se niega con un rol propio y su limpieza lo deja bajar", func(t *testing.T) {
		var co int64
		if err := st.Pool.QueryRow(ctx, `select id from companies order by id limit 1`).Scan(&co); err != nil {
			t.Fatal(err)
		}
		var propio int64
		if err := st.Pool.QueryRow(ctx,
			`insert into roles (company_id, name) values ($1, 'Propio de prueba') returning id`, co).Scan(&propio); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Pool.Exec(ctx,
			`update users set role_id = $2 where id = (select id from users where company_id = $1 order by id limit 1)`, co, propio); err != nil {
			t.Fatal(err)
		}
		// Sin la limpieza de prueba: es la guarda de producción la que se mira.
		if err := goose.DownContext(ctx, prepararGoose(t, st.Pool), "."); err == nil {
			t.Fatal("el Down bajó con un rol propio: lo habría perdido sin avisar")
		}
		migrarAbajoHasta(t, st.Pool, companyRolesVersion-1)
		migrarArriba(t, st.Pool)
	})
}
