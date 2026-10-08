//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// splitBillVersion es la migración de dividir la cuenta. Su número se mueve al siguiente libre de
// develop al fusionar (data-model.md), y esta constante con él.
const splitBillVersion = 79

// LA MIGRACIÓN DE DIVIDIR LA CUENTA SOBRE UN RESPALDO REAL CON DOS EMPRESAS.
//
// Cubre lo que un esquema sembrado desde cero no ve: pagos y pedidos que ya existen con las columnas
// nuevas en nulo, el rol de la aplicación leyendo y escribiendo las tablas nuevas, que una fila de
// una empresa no pueda colgarse de un renglón, pago o pedido de otra (los chequeos de FK saltan RLS),
// y que el Down se niegue cada vez que perdería algo sin rastro.
func TestMigrationSplitBillOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start >= splitBillVersion {
		migrarAbajoHasta(t, st.Pool, splitBillVersion-1)
	} else if start < splitBillVersion-1 {
		migrarArriba(t, st.Pool)
		migrarAbajoHasta(t, st.Pool, splitBillVersion-1)
	}
	t.Cleanup(func() {
		if v := versionDeEsquema(t, st); v != start && start < splitBillVersion {
			migrarAbajoHasta(t, st.Pool, start)
		}
	})

	var paymentsBefore, ordersBefore int
	if err := st.Pool.QueryRow(ctx, `select (select count(*) from order_payments), (select count(*) from orders)`).
		Scan(&paymentsBefore, &ordersBefore); err != nil {
		t.Fatal(err)
	}
	migrarArriba(t, st.Pool)

	t.Run("lo que ya existía queda en nulo", func(t *testing.T) {
		var payments, withNumber, withSplit, orders, merged int
		if err := st.Pool.QueryRow(ctx, `
			select (select count(*) from order_payments),
			       (select count(*) from order_payments where payment_number is not null),
			       (select count(*) from order_payments where split_part is not null or split_of is not null),
			       (select count(*) from orders),
			       (select count(*) from orders where merged_into_order_id is not null)`).
			Scan(&payments, &withNumber, &withSplit, &orders, &merged); err != nil {
			t.Fatal(err)
		}
		if payments != paymentsBefore || orders != ordersBefore || withNumber+withSplit+merged != 0 {
			t.Fatalf("pagos %d→%d, pedidos %d→%d, con número %d, con parte %d, juntados %d: la migración no rellena nada",
				paymentsBefore, payments, ordersBefore, orders, withNumber, withSplit, merged)
		}
	})

	// Dos empresas reales: A con un pedido que tiene pago y renglón, B con un renglón cualquiera.
	var companyA, orderA, lineA, paymentA, sessionA, methodA, userA int64
	if err := st.Pool.QueryRow(ctx, `
		select o.company_id, o.id, l.id, p.id, p.register_session_id, p.payment_method_id, o.opened_by
		  from orders o
		  join order_lines l on l.order_id = o.id
		  join order_payments p on p.order_id = o.id and p.register_session_id is not null
		 order by o.id desc limit 1`).
		Scan(&companyA, &orderA, &lineA, &paymentA, &sessionA, &methodA, &userA); err != nil {
		t.Fatalf("un pedido con pago y renglón: %v", err)
	}
	var companyB, orderB, lineB, paymentB int64
	if err := st.Pool.QueryRow(ctx, `
		select o.company_id, o.id, l.id, coalesce((select max(p.id) from order_payments p where p.company_id = o.company_id), 0)
		  from orders o join order_lines l on l.order_id = o.id
		 where o.company_id <> $1 order by o.id desc limit 1`, companyA).
		Scan(&companyB, &orderB, &lineB, &paymentB); err != nil {
		t.Fatalf("un renglón de la otra empresa: %v", err)
	}

	t.Run("el rol de la aplicación lee y escribe las cuatro tablas", func(t *testing.T) {
		conn := conexionDeEmpresa(t, restoredAppRoleStore(t), companyA)
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		key := uuid.New()
		for _, s := range []struct {
			name string
			sql  string
			args []any
		}{
			{"order_payment_lines", `insert into order_payment_lines (order_payment_id, order_line_id, qty, amount) values ($1, $2, 1, 0)`,
				[]any{paymentA, lineA}},
			{"order_payment_voids", `insert into order_payment_voids (order_id, original_payment_id, payment_number, payment_method_id,
				amount, tip_amount, register_session_id, paid_at, voided_by, reason)
				values ($1, -1, 1, $2, 1, 0, $3, now(), $4, 'prueba')`, []any{orderA, methodA, sessionA, userA}},
			{"order_line_move_batches", `insert into order_line_move_batches (client_uuid, from_order_id, to_order_id, moved_by)
				values ($1, $2, (select id from orders where id <> $2 order by id desc limit 1), $3)`, []any{key, orderA, userA}},
			{"order_line_moves", `insert into order_line_moves (client_uuid, order_line_id, qty) values ($1, $2, 1)`, []any{key, lineA}},
		} {
			if _, err := tx.Exec(ctx, s.sql, s.args...); err != nil {
				t.Fatalf("%s: insertar como gatobobah_app: %v", s.name, err)
			}
			var n int
			if err := tx.QueryRow(ctx, `select count(*) from `+s.name).Scan(&n); err != nil || n == 0 {
				t.Fatalf("%s: leer como gatobobah_app: n=%d err=%v", s.name, n, err)
			}
		}
	})

	t.Run("una fila de una empresa no se cuelga de otra", func(t *testing.T) {
		cases := []struct {
			name string
			sql  string
			args []any
		}{
			{"cobertura de un renglón ajeno", `insert into order_payment_lines (company_id, order_payment_id, order_line_id, qty, amount)
				values ($1, $2, $3, 1, 0)`, []any{companyA, paymentA, lineB}},
			{"bitácora de un pedido ajeno", `insert into order_payment_voids (company_id, order_id, original_payment_id, payment_number,
				payment_method_id, amount, tip_amount, register_session_id, paid_at, voided_by, reason)
				values ($1, $2, -2, 1, $3, 1, 0, $4, now(), $5, 'prueba')`, []any{companyA, orderB, methodA, sessionA, userA}},
			{"lote hacia un pedido ajeno", `insert into order_line_move_batches (company_id, client_uuid, from_order_id, to_order_id, moved_by)
				values ($1, $2, $3, $4, $5)`, []any{companyA, uuid.New(), orderA, orderB, userA}},
			{"pedido juntado con uno ajeno", `update orders set status = 'cancelada', cancelled_at = now(), cancelled_by = opened_by,
				cancel_reason = 'Se juntó con otro pedido', merged_into_order_id = $2 where id = $1`, []any{orderA, orderB}},
		}
		if paymentB != 0 {
			cases = append(cases, struct {
				name string
				sql  string
				args []any
			}{"cobertura con el pago de otra empresa", `insert into order_payment_lines (company_id, order_payment_id, order_line_id, qty, amount)
				values ($1, $2, $3, 1, 0)`, []any{companyA, paymentB, lineA}})
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				err := inRolledBackTx(t, st.Pool, func(tx pgx.Tx) error { _, err := tx.Exec(ctx, c.sql, c.args...); return err })
				exigeViolacionDeRestriccion(t, err, c.name)
			})
		}
	})

	t.Run("solo un pedido cancelado se puede juntar", func(t *testing.T) {
		var other int64
		if err := st.Pool.QueryRow(ctx, `select id from orders where company_id = $1 and id <> $2 order by id desc limit 1`,
			companyA, orderA).Scan(&other); err != nil {
			t.Fatal(err)
		}
		err := inRolledBackTx(t, st.Pool, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `update orders set status = 'abierta', merged_into_order_id = $2 where id = $1`, orderA, other)
			return err
		})
		exigeViolacionDeRestriccion(t, err, "juntar un pedido que no está cancelado")
	})

	t.Run("el Down se niega si perdería algo", func(t *testing.T) {
		guards := []struct {
			name     string
			setup    string
			undo     string
			args     []any
			undoArgs []any
		}{
			{"un pago devuelto", `insert into order_payment_voids (company_id, order_id, original_payment_id, payment_number, payment_method_id,
				amount, tip_amount, register_session_id, paid_at, voided_by, reason)
				values ($1, $2, -3, 1, $3, 1, 0, $4, now(), $5, 'prueba')`,
				`delete from order_payment_voids where original_payment_id = -3`, []any{companyA, orderA, methodA, sessionA, userA}, nil},
			{"una cobertura", `insert into order_payment_lines (company_id, order_payment_id, order_line_id, qty, amount) values ($1, $2, $3, 1, 0)`,
				`delete from order_payment_lines where order_payment_id = $1`, []any{companyA, paymentA, lineA}, []any{paymentA}},
			{"un lote de pasar", `insert into order_line_move_batches (company_id, client_uuid, from_order_id, to_order_id, moved_by)
				values ($1, '00000000-0000-0000-0000-0000000000a1', $2, (select id from orders where company_id = $1 and id <> $2 order by id desc limit 1), $3)`,
				`delete from order_line_move_batches where client_uuid = '00000000-0000-0000-0000-0000000000a1'`, []any{companyA, orderA, userA}, nil},
			{"un renglón pasado", `with b as (insert into order_line_move_batches (company_id, client_uuid, from_order_id, to_order_id, moved_by)
				values ($1, '00000000-0000-0000-0000-0000000000a2', $2, (select id from orders where company_id = $1 and id <> $2 order by id desc limit 1), $3)
				returning client_uuid)
				insert into order_line_moves (company_id, client_uuid, order_line_id, qty) select $1, client_uuid, $4, 1 from b`,
				`with m as (delete from order_line_moves where client_uuid = '00000000-0000-0000-0000-0000000000a2' returning client_uuid)
				 delete from order_line_move_batches where client_uuid = '00000000-0000-0000-0000-0000000000a2'`, []any{companyA, orderA, userA, lineA}, nil},
			{"un pedido juntado", `update orders set merged_into_order_id = (select id from orders where company_id = $1 and id <> $2 order by id desc limit 1),
				status = 'cancelada', cancelled_at = coalesce(cancelled_at, now()), cancelled_by = coalesce(cancelled_by, opened_by),
				cancel_reason = coalesce(cancel_reason, 'Se juntó con otro pedido') where id = $2`,
				`update orders set merged_into_order_id = null where id = $1`, []any{companyA, orderA}, []any{orderA}},
			{"un pago por partes", `update order_payments set split_part = 1, split_of = 2 where id = $1`,
				`update order_payments set split_part = null, split_of = null where id = $1`, []any{paymentA}, []any{paymentA}},
			{"un pago con número", `update order_payments set payment_number = 1 where id = $1`,
				`update order_payments set payment_number = null where id = $1`, []any{paymentA}, []any{paymentA}},
		}
		for _, g := range guards {
			t.Run(g.name, func(t *testing.T) {
				// El estado del pedido se guarda y se repone: la guarda del pedido juntado lo cancela.
				var status string
				if err := st.Pool.QueryRow(ctx, `select status::text from orders where id = $1`, orderA).Scan(&status); err != nil {
					t.Fatal(err)
				}
				if _, err := st.Pool.Exec(ctx, g.setup, g.args...); err != nil {
					t.Fatalf("preparar %s: %v", g.name, err)
				}
				err := goose.DownToContext(ctx, prepararGoose(t, st.Pool), ".", splitBillVersion-1)
				if _, uerr := st.Pool.Exec(ctx, g.undo, g.undoArgs...); uerr != nil {
					t.Fatalf("deshacer %s: %v", g.name, uerr)
				}
				if g.name == "un pedido juntado" && status != "cancelada" {
					if _, uerr := st.Pool.Exec(ctx, `update orders set status = $2::order_status, cancelled_at = null, cancelled_by = null,
						cancel_reason = null where id = $1`, orderA, status); uerr != nil {
						t.Fatalf("reponer el estado: %v", uerr)
					}
				}
				if err == nil {
					migrarArriba(t, st.Pool)
					t.Fatalf("el Down corrió con %s: lo habría perdido sin rastro", g.name)
				}
				if want := downGuardText[g.name]; !strings.Contains(err.Error(), want) {
					t.Fatalf("el Down se negó, pero no por su guarda (quería «%s»): %v", want, err)
				}
				if v := versionDeEsquema(t, st); v < splitBillVersion {
					t.Fatalf("el Down se negó pero dejó la base en %d", v)
				}
			})
		}
	})

	t.Run("sin nada que perder, baja y vuelve a subir", func(t *testing.T) {
		migrarAbajoHasta(t, st.Pool, splitBillVersion-1)
		migrarArriba(t, st.Pool)
	})
}

// downGuardText es el pedazo del mensaje de cada guarda del Down: sin él, un Down que truena por
// otra razón (una dependencia, un typo) pasaría por guarda funcionando.
var downGuardText = map[string]string{
	"un pago devuelto":   "pagos devueltos",
	"una cobertura":      "pagos con productos",
	"un lote de pasar":   "productos pasados",
	"un renglón pasado":  "productos pasados",
	"un pedido juntado":  "pedidos juntados",
	"un pago por partes": "parte o número",
	"un pago con número": "parte o número",
}

// inRolledBackTx corre `f` como dueño en una transacción que siempre se deshace.
func inRolledBackTx(t *testing.T, pool *pgxpool.Pool, f func(pgx.Tx) error) error {
	t.Helper()
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	return f(tx)
}
