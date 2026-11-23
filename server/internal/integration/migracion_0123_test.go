//go:build integration

package integration

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

// publicMenuVersion es la migración del menú público (spec 097).
// PROVISIONAL: se mueve al siguiente número libre de develop al fusionar, y esta constante con ella.
const publicMenuVersion = 123

// LA 0121 SOBRE UN RESPALDO REAL CON DOS EMPRESAS (constitución IV).
//
// Lo que solo se ve con datos reales: que cada empresa amanece con un menú y un nombre derivado del
// NOMBRE del negocio —no del slug con el que se inicia sesión—, que dos nombres empatados no tumban
// el relleno, y que la vista que resuelve nombres sin sesión no deja ver nada más.
func TestMigrationPublicMenuOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start < publicMenuVersion-1 {
		migrarArriba(t, st.Pool)
	}
	migrarAbajoHasta(t, st.Pool, publicMenuVersion-1)
	t.Cleanup(func() {
		if versionDeEsquema(t, st) < start {
			migrarArriba(t, st.Pool)
		}
	})

	// Dos empresas cuyo nombre da el mismo nombre de menú, una con nombre de 45 caracteres (el sufijo
	// no debe desbordar el check de 40) y una cuyo nombre no deja ni una letra.
	long := strings.Repeat("Taquería Los Primos ", 3)
	for _, n := range []string{long, long, "🍕🍕", "Ñ"} {
		if _, err := st.Pool.Exec(ctx, `insert into companies (slug, name) values ('mig0121-' || substr(md5(random()::text), 1, 8), $1)`, n); err != nil {
			t.Fatal(err)
		}
	}

	migrarArriba(t, st.Pool)

	shape := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

	t.Run("toda empresa tiene menú, código QR y un solo nombre vigente válido", func(t *testing.T) {
		var missing int
		if err := st.Pool.QueryRow(ctx, `
			select count(*) from companies c
			 where not exists (select 1 from public_menus m where m.company_id = c.id)
			    or (select count(*) from public_menu_names n where n.company_id = c.id and n.retired_at is null) <> 1`).Scan(&missing); err != nil {
			t.Fatal(err)
		}
		if missing != 0 {
			t.Fatalf("%d empresas sin menú o sin exactamente un nombre vigente", missing)
		}
		rows, err := st.Pool.Query(ctx, `select n.name, m.qr_token from public_menu_names n join public_menus m on m.id = n.menu_id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		tok := regexp.MustCompile(`^[0-9a-f]{12}$`)
		for rows.Next() {
			var name, token string
			if err := rows.Scan(&name, &token); err != nil {
				t.Fatal(err)
			}
			if len(name) < 3 || len(name) > 40 || !shape.MatchString(name) {
				t.Fatalf("nombre inválido %q: el relleno dejó algo que la Function nunca podría servir", name)
			}
			if !tok.MatchString(token) {
				t.Fatalf("código QR con forma inesperada: %q", token)
			}
		}
	})

	t.Run("el nombre sale del nombre del negocio, no del slug de inicio de sesión", func(t *testing.T) {
		var name, slug string
		if err := st.Pool.QueryRow(ctx, `
			select n.name, c.slug from companies c join public_menu_names n on n.company_id = c.id and n.retired_at is null
			 where c.slug = 'gatobobah'`).Scan(&name, &slug); err != nil {
			t.Fatal(err)
		}
		if name == slug {
			t.Fatalf("el menú de El Gato Bobah se llama %q, igual que su slug: publicaría la mitad de la credencial de inicio de sesión", name)
		}
		if name != "el-gato-bobah" {
			t.Fatalf("se esperaba el-gato-bobah, quedó %q", name)
		}
	})

	t.Run("free_name nunca devuelve algo que viole el check", func(t *testing.T) {
		var id int64
		if err := st.Pool.QueryRow(ctx, `select id from companies order by id limit 1`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		cases := map[string]func(string) bool{
			"":                              func(s string) bool { return strings.HasPrefix(s, "menu-") },
			"n":                             func(s string) bool { return s == "menu-n" || strings.HasPrefix(s, "menu-n-") },
			"admin":                         func(s string) bool { return s == "admin-2" || strings.HasPrefix(s, "admin-") },
			"el-gato-bobah":                 func(s string) bool { return s != "el-gato-bobah" },
			strings.Repeat("a", 40):         func(s string) bool { return true },
			strings.Repeat("ab-", 13) + "a": func(s string) bool { return true },
		}
		if _, err := st.Pool.Exec(ctx, `insert into public_menu_names (company_id, menu_id, name, retired_at)
			select m.company_id, m.id, repeat('a', 40), now() from public_menus m where m.company_id = $1`, id); err != nil {
			t.Fatal(err)
		}
		for in, ok := range cases {
			var out string
			if err := st.Pool.QueryRow(ctx, `select public_menu_free_name($1, $2)`, in, id).Scan(&out); err != nil {
				t.Fatalf("free_name(%q): %v", in, err)
			}
			if len(out) < 3 || len(out) > 40 || !shape.MatchString(out) || !ok(out) {
				t.Fatalf("free_name(%q) = %q", in, out)
			}
		}
	})

	t.Run("la vista que resuelve sin sesión no ve slug ni nada más del negocio", func(t *testing.T) {
		var canLogin, slugGrant, idGrant bool
		if err := st.Pool.QueryRow(ctx, `
			select r.rolcanlogin,
			       has_column_privilege('gatobobah_public_menu', 'companies', 'slug', 'select'),
			       has_column_privilege('gatobobah_public_menu', 'companies', 'id', 'select')
			  from pg_roles r where r.rolname = 'gatobobah_public_menu'`).Scan(&canLogin, &slugGrant, &idGrant); err != nil {
			t.Fatal(err)
		}
		if canLogin || slugGrant || !idGrant {
			t.Fatalf("rol del menú: login=%v slug=%v id=%v; debe ser nologin y ver solo id e is_active", canLogin, slugGrant, idGrant)
		}
		rows, err := st.Pool.Query(ctx, `select column_name from information_schema.columns where table_name = 'public_menu_routes' order by ordinal_position`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var cols []string
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				t.Fatal(err)
			}
			cols = append(cols, c)
		}
		if strings.Join(cols, ",") != "name,company_id,is_current,current_name,qr_token,company_active" {
			t.Fatalf("la vista proyecta %v: cada columna de más la puede leer cualquier petición del rol de la aplicación", cols)
		}
	})

	t.Run("bajar y volver a subir", func(t *testing.T) {
		migrarAbajoHasta(t, st.Pool, publicMenuVersion-1)
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from pg_roles where rolname = 'gatobobah_public_menu'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatal("el Down no debe borrar el rol: es del clúster y otra base puede usarlo")
		}
		migrarArriba(t, st.Pool)
	})
}
