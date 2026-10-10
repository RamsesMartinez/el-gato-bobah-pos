//go:build integration

package integration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// LA FILA SE PIDE CADA 30 SEGUNDOS POR TABLETA: SU CONSULTA TIENE TECHO (research R-7).
//
// 30 mil pedidos entregados en dos empresas, la mitad sin cobrar. Sin la ventana de 90 días el
// lateral de pagos recorre todo el histórico (medido: ~175 ms). Con ella tiene que quedar en 30 ms y
// entrar por un índice que arranque por empresa: RLS agrega ese predicado a toda consulta del rol de
// la app. (El plan decía `orders_company_date_status`; con el OR de la rama de cocina —de cualquier
// fecha— Postgres entra por el índice de empresa y filtra, y así se midió: lo que se topa es el
// tiempo, no el nombre del índice.) El modo `olderDebts` se anota, no se topa: es el techo del
// `ponytail` de la consulta y solo lo piden acciones del operador, no un intervalo.
func TestLiveAccountsStaysFast(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	other := makeCompany(t, st, "otra-rapida")
	user := makeUser(t, st, "cajero_rapido", "cajero")
	otherUser := makeUserIn(t, st, other, "cajero_rapido_b", "cajero")
	for _, c := range []struct{ company, user int64 }{{defaultCompanyID, user}, {other, otherUser}} {
		if _, err := st.Pool.Exec(ctx, `
			insert into orders (company_id, client_uuid, business_date, daily_number, service_type, opened_by,
			                    subtotal, total, status, opened_at)
			select $1, gen_random_uuid(), $3::date - (g % 400), g, 'mostrador', $2, 100, 100, 'entregada',
			       $3::timestamptz - make_interval(days => g % 400)
			from generate_series(1, 15000) g`, c.company, c.user, fixedNow); err != nil {
			t.Fatalf("sembrar pedidos: %v", err)
		}
		if _, err := st.Pool.Exec(ctx, `
			insert into order_payments (company_id, order_id, payment_method_id, amount, created_at)
			select o.company_id, o.id, (select id from payment_methods where company_id = $1 limit 1), 100, o.opened_at
			from orders o where o.company_id = $1 and o.daily_number % 2 = 0`, c.company); err != nil {
			t.Fatalf("sembrar pagos: %v", err)
		}
	}
	if _, err := st.Pool.Exec(ctx, `analyze orders; analyze order_payments`); err != nil {
		t.Fatal(err)
	}

	query := liveOrdersSQL(t)
	conn := conexionDeEmpresa(t, appRoleStore(t), defaultCompanyID)
	since90 := pgtype.Date{Time: fixedNow.AddDate(0, 0, -90), Valid: true}
	sinceAll := pgtype.Date{Time: time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	best := func(since pgtype.Date) time.Duration {
		var b time.Duration
		for i := range 5 {
			start := time.Now()
			rows, err := conn.Query(ctx, query, since)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
			}
			rows.Close()
			if d := time.Since(start); i == 0 || d < b {
				b = d
			}
		}
		return b
	}
	window := best(since90)
	all := best(sinceAll)
	t.Logf("ListLiveOrders con 30 mil pedidos: ventana de 90 días %v, olderDebts %v (techo del ponytail)", window, all)
	if window > 30*time.Millisecond {
		t.Errorf("la consulta de cada 30 s tardó %v con 30 mil pedidos; el techo es 30 ms", window)
	}

	var plan strings.Builder
	// La fecha va como literal: un EXPLAIN con parámetro por el protocolo extendido no devuelve plan.
	literal := strings.ReplaceAll(query, "$1::date", "'"+since90.Time.Format("2006-01-02")+"'::date")
	rows, err := conn.Query(ctx, "explain "+literal)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line + "\n")
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("explain: %v", err)
	}
	// Ni recorrido secuencial de `orders` ni un índice que no arranque por empresa: RLS pone el
	// predicado de empresa en toda consulta del rol de la app, y un índice que no lo usa se queda
	// descartando filas de otras empresas.
	if strings.Contains(plan.String(), "Seq Scan on orders") || !strings.Contains(plan.String(), "company_id = ") {
		t.Errorf("la consulta no entra a orders por un índice de empresa:\n%s", plan.String())
	}
}

// liveOrdersSQL lee la consulta del archivo de sqlc, para medir la MISMA que corre en producción y
// no una copia que se queda atrás.
func liveOrdersSQL(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../queries/orders.sql")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	i := strings.Index(s, "-- name: ListLiveOrders")
	if i < 0 {
		t.Fatal("no está ListLiveOrders en queries/orders.sql")
	}
	s = s[i:]
	s = s[strings.Index(s, "\n")+1:]
	if j := strings.Index(s, "-- name:"); j >= 0 {
		s = s[:j]
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		if k := strings.Index(line, "--"); k >= 0 {
			line = line[:k]
		}
		out = append(out, line)
	}
	q := strings.TrimSpace(strings.Join(out, "\n"))
	q = strings.TrimSuffix(q, ";")
	return strings.ReplaceAll(q, "@since::date", "$1::date")
}
