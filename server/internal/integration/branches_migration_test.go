//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

// LA MIGRACIÓN DE SUCURSALES (0076) SOBRE UN RESPALDO REAL CON DOS EMPRESAS.
//
// Lo que solo se ve con datos reales: pedidos viejos sin turno, existencias acumuladas, dos
// empresas a las que el relleno tiene que darles su propia matriz y no la de la otra. Y que el
// dinero no se mueva: la migración no toca un solo total.

func TestBranchesMigrationOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start >= 76 {
		migrarAbajoHasta(t, st.Pool, 75)
	}
	t.Cleanup(func() {
		if versionDeEsquema(t, st) != start {
			if start >= 76 {
				migrarArriba(t, st.Pool)
			} else {
				migrarAbajoHasta(t, st.Pool, start)
			}
		}
	})

	snapshot := func() string {
		var out string
		if err := st.Pool.QueryRow(ctx, `
			select coalesce(string_agg(x, ';' order by x), '') from (
			  select format('o|%s|%s|%s|%s|%s', company_id, business_date, status, count(*), sum(total)) as x
			    from orders group by company_id, business_date, status
			  union all
			  select format('p|%s|%s|%s', o.company_id, p.payment_method_id, sum(p.amount))
			    from order_payments p join orders o on o.id = p.order_id group by o.company_id, p.payment_method_id
			  union all
			  select format('s|%s|%s|%s|%s', company_id, coalesce(ingredient_id, 0), coalesce(product_id, 0), on_hand)
			    from stock_levels
			) t`).Scan(&out); err != nil {
			t.Fatalf("resumen de dinero y existencias: %v", err)
		}
		return out
	}
	var updatedBefore string
	if err := st.Pool.QueryRow(ctx, `select coalesce(max(updated_at)::text, '') from orders`).Scan(&updatedBefore); err != nil {
		t.Fatal(err)
	}
	before := snapshot()

	migrarArriba(t, st.Pool)

	if after := snapshot(); after != before {
		t.Fatalf("la migración movió dinero o existencias (SC-005): antes y después difieren")
	}
	var updatedAfter string
	if err := st.Pool.QueryRow(ctx, `select coalesce(max(updated_at)::text, '') from orders`).Scan(&updatedAfter); err != nil {
		t.Fatal(err)
	}
	if updatedAfter != updatedBefore {
		t.Fatalf("el relleno reescribió updated_at de los pedidos (%s → %s): es la hora de la última edición de quien opera", updatedBefore, updatedAfter)
	}

	var companies, withOneHQ int
	if err := st.Pool.QueryRow(ctx, `
		select (select count(*) from companies),
		       (select count(*) from companies c where (select count(*) from branches b
		         where b.company_id = c.id and b.is_headquarters and b.branch_number = 1) = 1)`).Scan(&companies, &withOneHQ); err != nil {
		t.Fatal(err)
	}
	if companies < 2 || withOneHQ != companies {
		t.Fatalf("cada empresa debe tener exactamente una matriz número 1: %d de %d la tienen", withOneHQ, companies)
	}

	for _, table := range []string{"cash_registers", "platform_connections", "orders", "stock_movements", "stock_levels"} {
		var orphans, crossed int
		if err := st.Pool.QueryRow(ctx,
			`select count(*) filter (where t.branch_id is null),
			        count(*) filter (where b.company_id <> t.company_id)
			   from `+table+` t left join branches b on b.id = t.branch_id`).Scan(&orphans, &crossed); err != nil {
			t.Fatalf("revisar %s: %v", table, err)
		}
		if orphans != 0 || crossed != 0 {
			t.Fatalf("%s: %d filas sin sucursal y %d en la sucursal de otra empresa", table, orphans, crossed)
		}
	}

	t.Run("bajar se niega con dos sucursales", func(t *testing.T) {
		var company int64
		if err := st.Pool.QueryRow(ctx, `select min(id) from companies`).Scan(&company); err != nil {
			t.Fatal(err)
		}
		var second int64
		if err := st.Pool.QueryRow(ctx,
			`insert into branches (company_id, code, name) values ($1, 'PRUEBA2', 'Prueba') returning id`, company).Scan(&second); err != nil {
			t.Fatal(err)
		}
		db := prepararGoose(t, st.Pool)
		err := goose.DownToContext(ctx, db, ".", 75)
		if err == nil || !strings.Contains(err.Error(), "más de una sucursal") {
			t.Fatalf("bajar la 0076 con dos sucursales mezclaría sus datos y debe negarse, fue: %v", err)
		}
		if _, err := st.Pool.Exec(ctx, `delete from branches where id = $1`, second); err != nil {
			t.Fatal(err)
		}
		migrarAbajoHasta(t, st.Pool, 75)
		if after := snapshot(); after != before {
			t.Fatalf("bajar la 0076 movió dinero o existencias")
		}
	})
}
