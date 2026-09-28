//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
)

// TODA TABLA CON company_id ESTÁ AISLADA, Y SE COMPRUEBA RECORRIENDO EL CATÁLOGO, NO UNA LISTA.
//
// Una lista escrita a mano se queda corta en silencio el día que alguien agrega una tabla. Este
// test la encuentra solo: la tabla nueva sin RLS, sin su política con `nullif` o sin su GRANT
// truena aquí, antes de producción.
//
// Estructura, por tabla: RLS encendido, una política que compara contra `app.company_id` protegido
// con `nullif` (el defecto de la 0074: tras un `reset` el ajuste es cadena vacía y el cast revienta)
// y `select` para gatobobah_app (sin él, 42501 en el primer request; el GRANT no se hereda).
//
// Comportamiento, por tabla, en dos de los tres casos (el tercero, «sin empresa», lo cubre
// TestElRolDeAppNoVeNadaSinEmpresa):
//   - otra empresa: la sesión de B no ve ninguna fila cuyo company_id no sea B.
//   - conexión reciclada: tras soltar una sesión, la misma conexión sin empresa no ve nada y NO
//     revienta (la 0074).
func TestEveryCompanyTableIsIsolated(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	a := makeCompany(t, st, "catalogo-a")
	b := makeCompany(t, st, "catalogo-b")
	_ = a

	rows, err := st.Pool.Query(ctx, `
		select c.relname,
		       c.relrowsecurity,
		       exists (select 1 from pg_policies p
		                where p.schemaname = 'public' and p.tablename = c.relname
		                  and (coalesce(p.qual, '') || coalesce(p.with_check, '')) ilike '%current_setting%app.company_id%'
		                  and (coalesce(p.qual, '') || coalesce(p.with_check, '')) ilike '%nullif%'),
		       has_table_privilege('gatobobah_app', c.oid, 'select')
		  from pg_class c
		  join pg_namespace n on n.oid = c.relnamespace and n.nspname = 'public'
		 where c.relkind in ('r', 'p')
		   and exists (select 1 from pg_attribute at
		                where at.attrelid = c.oid and at.attname = 'company_id' and not at.attisdropped)
		 order by c.relname`)
	if err != nil {
		t.Fatal(err)
	}
	type table struct {
		name                   string
		rls, policy, canSelect bool
	}
	var tables []table
	for rows.Next() {
		var x table
		if err := rows.Scan(&x.name, &x.rls, &x.policy, &x.canSelect); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, x)
	}
	rows.Close()
	if len(tables) < 50 {
		t.Fatalf("solo %d tablas con company_id: la consulta al catálogo dejó de encontrarlas", len(tables))
	}

	var wrong []string
	for _, x := range tables {
		switch {
		case !x.rls:
			wrong = append(wrong, x.name+": RLS apagado")
		case !x.policy:
			wrong = append(wrong, x.name+": sin política de empresa protegida con nullif")
		case !x.canSelect:
			wrong = append(wrong, x.name+": gatobobah_app sin select")
		}
	}
	if len(wrong) > 0 {
		t.Fatalf("tablas con company_id sin aislar (%d):\n  %s", len(wrong), strings.Join(wrong, "\n  "))
	}

	appSt := appRoleStore(t)
	asB := conexionDeEmpresa(t, appSt, b)
	recycled := appRoleStoreWithOneConn(t)
	_, release, err := recycled.AcquireTenant(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	release()

	var leaks []string
	for _, x := range tables {
		// pgx no interpola identificadores; el nombre viene del catálogo de Postgres, no de fuera.
		var n int
		if err := asB.QueryRow(ctx, `select count(*) from "`+x.name+`" where company_id <> $1`, b).Scan(&n); err != nil {
			leaks = append(leaks, x.name+" (otra empresa, error: "+err.Error()+")")
		} else if n > 0 {
			leaks = append(leaks, x.name+" (la sesión de B ve "+itoa(n)+" filas ajenas)")
		}
		if err := recycled.Pool.QueryRow(ctx, `select count(*) from "`+x.name+`"`).Scan(&n); err != nil {
			leaks = append(leaks, x.name+" (conexión reciclada, revienta: "+err.Error()+")")
		} else if n > 0 {
			leaks = append(leaks, x.name+" (conexión reciclada ve "+itoa(n)+" filas)")
		}
	}
	if len(leaks) > 0 {
		t.Fatalf("RLS no aísla en %d casos:\n  %s", len(leaks), strings.Join(leaks, "\n  "))
	}
}
