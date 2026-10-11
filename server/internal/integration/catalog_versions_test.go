//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// La versión de catálogo existe porque el 2026-10-10 un cambio hecho con SQL directo no llegó a la
// tableta: el menú guardado solo se invalidaba desde las pantallas de administración. Estas pruebas
// fallan nombrando el camino por el que el cambio se escaparía.

func catalogVersion(t *testing.T, st *store.Store, company int64) int64 {
	t.Helper()
	var v int64
	if err := st.Pool.QueryRow(context.Background(),
		`select coalesce((select version from catalog_versions where company_id = $1), 0)`, company).Scan(&v); err != nil {
		t.Fatalf("leer la versión de %d: %v", company, err)
	}
	return v
}

// Un script como owner que NO declara empresa y toca dos a la vez: cada una sube, sin depender de
// app.company_id.
func TestADirectSQLChangeBumpsEveryTouchedCompany(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	a := makeCompany(t, st, "version-a")
	b := makeCompany(t, st, "version-b")
	c := makeCompany(t, st, "version-c")
	for _, co := range []int64{a, b, c} {
		if _, err := st.Pool.Exec(ctx, `insert into modifier_groups (company_id, name) values ($1, 'Perlas')`, co); err != nil {
			t.Fatalf("grupo de %d: %v", co, err)
		}
	}
	antesA, antesB, antesC := catalogVersion(t, st, a), catalogVersion(t, st, b), catalogVersion(t, st, c)

	tx, err := st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `select set_config('app.company_id', '', true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `update modifier_groups set is_active = false where company_id in ($1, $2)`, a, b); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if catalogVersion(t, st, a) <= antesA || catalogVersion(t, st, b) <= antesB {
		t.Fatalf("un update directo sin empresa no subió la versión de las empresas que tocó: el menú guardado se seguiría sirviendo")
	}
	if catalogVersion(t, st, c) != antesC {
		t.Fatalf("la versión de una empresa que nadie tocó cambió: cada cambio ajeno le tiraría el menú guardado")
	}
}

// Cada una de las tablas que lee el menú sube la versión en alta, cambio y baja. Una tabla sin su
// trigger es exactamente el camino por el que volvería el incidente.
func TestEveryMenuTableBumpsTheVersion(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	co := makeCompany(t, st, "version-tablas")
	ex := func(sql string, args ...any) {
		t.Helper()
		if _, err := st.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	var cat, prod, grp, opt int64
	var plat int16
	if err := st.Pool.QueryRow(ctx, `insert into categories (company_id, name) values ($1, 'C') returning id`, co).Scan(&cat); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into products (company_id, name, category_id, price) values ($1, 'P', $2, 10) returning id`, co, cat).Scan(&prod); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into modifier_groups (company_id, name) values ($1, 'G') returning id`, co).Scan(&grp); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into modifier_options (company_id, group_id, name) values ($1, $2, 'O') returning id`, co, grp).Scan(&opt); err != nil {
		t.Fatal(err)
	}
	if err := st.Pool.QueryRow(ctx, `select id from delivery_platforms where company_id = $1 and name <> 'Propio' limit 1`, co).Scan(&plat); err != nil {
		t.Fatal(err)
	}
	var canal int64
	if err := st.Pool.QueryRow(ctx, `select id from channels where company_id = $1 limit 1`, co).Scan(&canal); err != nil {
		if _, err := st.Pool.Exec(ctx, `insert into channels (company_id, code, name) values ($1, 'pos', 'POS')`, co); err != nil {
			t.Fatalf("canal: %v", err)
		}
		if err := st.Pool.QueryRow(ctx, `select id from channels where company_id = $1 limit 1`, co).Scan(&canal); err != nil {
			t.Fatal(err)
		}
	}

	quien := makeUserIn(t, st, co, "version-quien", "admin")

	cases := []struct {
		table string
		run   func()
	}{
		{"categories", func() { ex(`update categories set name = 'C2' where id = $1`, cat) }},
		{"products", func() { ex(`update products set price = 11 where id = $1`, prod) }},
		{"modifier_groups", func() { ex(`update modifier_groups set name = 'G2' where id = $1`, grp) }},
		{"modifier_options", func() { ex(`update modifier_options set name = 'O2' where id = $1`, opt) }},
		{"product_modifier_groups insert", func() {
			ex(`insert into product_modifier_groups (company_id, product_id, group_id) values ($1, $2, $3)`, co, prod, grp)
		}},
		{"product_modifier_groups delete", func() { ex(`delete from product_modifier_groups where product_id = $1`, prod) }},
		{"product_channels", func() {
			ex(`insert into product_channels (company_id, product_id, channel_id, visibility) values ($1, $2, $3, 'oculto')`, co, prod, canal)
		}},
		{"channels", func() { ex(`update channels set name = name where id = $1`, canal) }},
		{"delivery_platforms", func() { ex(`update delivery_platforms set price_markup_pct = 20 where id = $1`, plat) }},
		{"product_platform_prices", func() {
			ex(`insert into product_platform_prices (company_id, product_id, platform_id, price, updated_by) values ($1, $2, $3, 15, $4)`, co, prod, plat, quien)
		}},
		{"modifier_option_platform_prices", func() {
			ex(`insert into modifier_option_platform_prices (company_id, option_id, platform_id, price_delta, updated_by) values ($1, $2, $3, 2, $4)`, co, opt, plat, quien)
		}},
	}
	for _, c := range cases {
		antes := catalogVersion(t, st, co)
		c.run()
		if catalogVersion(t, st, co) <= antes {
			t.Errorf("%s: el cambio no subió la versión del catálogo; la tableta seguiría con el menú viejo", c.table)
		}
	}

	// Una sentencia que no toca filas no es un cambio.
	antes := catalogVersion(t, st, co)
	ex(`update products set price = 1 where id = -1`)
	if catalogVersion(t, st, co) != antes {
		t.Errorf("una sentencia sin filas subió la versión: cada script vacío tiraría el menú guardado")
	}
}

// La API escribe catálogo como gatobobah_app bajo RLS: el trigger tiene que poder subir el contador
// sin que el rol tenga permiso de escribirlo.
func TestTheAppRoleBumpsOnlyItsOwnCompany(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	a := makeCompany(t, st, "version-app-a")
	b := makeCompany(t, st, "version-app-b")
	antesB := catalogVersion(t, st, b)

	var cat int64
	if err := st.Pool.QueryRow(ctx, `insert into categories (company_id, name) values ($1, 'Desde la app') returning id`, a).Scan(&cat); err != nil {
		t.Fatal(err)
	}
	antesA := catalogVersion(t, st, a)

	appSt := appRoleStore(t)
	actx, release, err := appSt.AcquireTenant(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	admin := app.NewAdminService(appSt)
	if _, err := admin.CreateProduct(actx, "Frappé", cat, decimal.NewFromInt(50), false, false, nil); err != nil {
		release()
		t.Fatalf("CreateProduct como app: %v", err)
	}
	release()

	if catalogVersion(t, st, a) <= antesA {
		t.Fatalf("un alta por la API (rol de la app, bajo RLS) no subió la versión")
	}
	if catalogVersion(t, st, b) != antesB {
		t.Fatalf("un alta de la empresa A subió la versión de B")
	}
}

// Borrar una empresa arrastra su catálogo; el trigger no puede apuntar un contador a la empresa
// que se está borrando y tumbar el borrado por la llave foránea.
func TestDeletingACompanyIsNotBlockedByItsCatalogVersion(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	co := makeCompany(t, st, "version-borrada")
	makeProductIn(t, st, co)
	if _, err := st.Pool.Exec(ctx, `delete from companies where id = $1`, co); err != nil {
		t.Fatalf("borrar la empresa falló por el contador de catálogo: %v", err)
	}
}

func makeProductIn(t *testing.T, st *store.Store, co int64) {
	t.Helper()
	ctx := context.Background()
	var cat int64
	if err := st.Pool.QueryRow(ctx, `insert into categories (company_id, name) values ($1, 'C-' || gen_random_uuid()) returning id`, co).Scan(&cat); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into products (company_id, name, category_id, price) values ($1, 'P-' || gen_random_uuid(), $2, 10)`, co, cat); err != nil {
		t.Fatal(err)
	}
}

// La lectura de la versión que usa el menú, en los tres casos donde RLS ya falló: nunca ve la de
// otra empresa.
func TestCatalogVersionIsIsolated(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	owner := makeCompany(t, st, "version-duena")
	other := makeCompany(t, st, "version-ajena")
	// La dueña queda con una versión que ninguna otra empresa puede tener a la vez.
	for range 20 {
		makeProductIn(t, st, owner)
	}
	ownerV := catalogVersion(t, st, owner)
	if ownerV <= catalogVersion(t, st, other) {
		t.Fatal("la versión de la dueña no se distingue de la ajena: la prueba no probaría nada")
	}
	inTheThreeCases(t, owner, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		v, err := app.NewMenuService(st, nil).Version(ctx)
		if err != nil {
			t.Fatalf("Version: %v", err)
		}
		if v.Catalog == ownerV {
			t.Fatalf("se leyó la versión de la empresa dueña (%d) desde fuera", v.Catalog)
		}
	})
}
