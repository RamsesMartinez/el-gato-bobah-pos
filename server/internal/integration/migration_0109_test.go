//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

// LA 0109 CONTRA UN RESPALDO REAL CON DOS EMPRESAS (principio IV): borra users.role y deja role_id
// obligatorio, rellenando antes lo que la API anterior pudo dejar a medias durante la subida de la
// 0108 (una empresa sin sus sugeridos, un usuario sin role_id).
func TestMigration0109OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	exigeDosEmpresas(t, st)
	migrarAbajoHasta(t, st.Pool, 108)

	// Lo que dejaría la API anterior: una empresa nueva sin sus roles y un usuario sin role_id (el
	// trigger de convivencia se apaga para dejarlo así a propósito).
	var nueva int64
	if err := st.Pool.QueryRow(ctx, `insert into companies (slug, name) values ('sin-roles-0109', 'Sin roles') returning id`).Scan(&nueva); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`alter table users disable trigger users_sync_role`,
		`insert into users (company_id, name, username, role) values (` + itoa(int(nueva)) + `, 'Huérfana', 'huerfana_0109', 'cajero')`,
		`alter table users enable trigger users_sync_role`,
	} {
		if _, err := st.Pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	type key struct {
		company int64
		code    string
	}
	before := map[key]int{}
	rows, err := st.Pool.Query(ctx, `select company_id, role::text, count(*) from users group by 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k key
		var n int
		_ = rows.Scan(&k.company, &k.code, &n)
		before[k] = n
	}
	rows.Close()

	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", 109); err != nil {
		t.Fatalf("subir a la 109: %v", err)
	}
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	var hasRole, hasTrigger bool
	var nullable string
	_ = st.Pool.QueryRow(ctx, `select exists (select 1 from information_schema.columns where table_name = 'users' and column_name = 'role')`).Scan(&hasRole)
	_ = st.Pool.QueryRow(ctx, `select exists (select 1 from pg_trigger where tgname = 'users_sync_role')`).Scan(&hasTrigger)
	_ = st.Pool.QueryRow(ctx, `select is_nullable from information_schema.columns where table_name = 'users' and column_name = 'role_id'`).Scan(&nullable)
	if hasRole || hasTrigger || nullable != "NO" {
		t.Fatalf("users.role sigue = %v, trigger = %v, role_id nullable = %s", hasRole, hasTrigger, nullable)
	}
	after := map[key]int{}
	rows, err = st.Pool.Query(ctx, `select u.company_id, s.code, count(*) from users u
		join roles r on r.id = u.role_id join system_roles s on s.id = r.system_role_id group by 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var k key
		var n int
		_ = rows.Scan(&k.company, &k.code, &n)
		after[k] = n
	}
	rows.Close()
	for k, n := range before {
		if after[k] != n {
			t.Errorf("empresa %d, %s: %d antes, %d después", k.company, k.code, n, after[k])
		}
	}
	var enUso bool
	_ = st.Pool.QueryRow(ctx, `select exists (select 1 from information_schema.columns where table_name = 'usage_daily' and column_name = 'role' and udt_name = 'user_role')`).Scan(&enUso)
	if !enUso {
		t.Error("la medición de uso perdió su dimensión de rol sugerido")
	}

	t.Run("el Down devuelve users.role con el código del sugerido", func(t *testing.T) {
		if err := goose.DownToContext(ctx, prepararGoose(t, st.Pool), ".", 108); err != nil {
			t.Fatalf("bajar: %v", err)
		}
		var mal int
		if err := st.Pool.QueryRow(ctx, `select count(*) from users u join roles r on r.id = u.role_id
			left join system_roles s on s.id = r.system_role_id where u.role::text <> coalesce(s.code, 'mesero')`).Scan(&mal); err != nil {
			t.Fatal(err)
		}
		if mal != 0 {
			t.Fatalf("%d usuarios con users.role distinto de su rol", mal)
		}
	})
}
