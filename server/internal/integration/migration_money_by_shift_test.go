//go:build integration

package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pressly/goose/v3"
	"github.com/shopspring/decimal"
)

// moneyByShiftVersion es la migración que le da turno, día y propina a cada devolución (0080), y
// dayOfEachPaymentVersion la que le pone su día a cada pago anterior (0081).
const (
	moneyByShiftVersion     = 80
	dayOfEachPaymentVersion = 81
)

// EL TURNO Y EL DÍA DE CADA DEVOLUCIÓN Y DE CADA PAGO, SOBRE UN RESPALDO REAL CON DOS EMPRESAS.
//
// Lo que un esquema sembrado desde cero no ve: devoluciones ya registradas que tienen que quedar en
// el turno de su salida de caja y no en otro; las de tarjeta hechas durante el turno que sigue
// abierto al migrar, que sin turno harían cerrar ese corte sin restarlas; el día de una devolución
// cerca de la medianoche en la zona del negocio; una devolución que no se puede colgar del turno de
// otra empresa (los chequeos de FK saltan RLS), y un Down que se niega cuando perdería dinero.
func TestMigrationMoneyByShiftOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	before := int64(moneyByShiftVersion - 1)
	if start >= moneyByShiftVersion {
		migrarAbajoHasta(t, st.Pool, before)
	} else if start < before {
		migrarArriba(t, st.Pool)
		migrarAbajoHasta(t, st.Pool, before)
	}
	t.Cleanup(func() {
		if v := versionDeEsquema(t, st); v != start && start < moneyByShiftVersion {
			migrarAbajoHasta(t, st.Pool, start)
		}
	})

	// Las dos empresas del respaldo, cada una con un pedido cobrado, su método y quien lo cobró.
	type empresa struct{ id, order, user, method, branch, register int64 }
	pick := func(not int64) empresa {
		var e empresa
		if err := st.Pool.QueryRow(ctx, `
			select o.company_id, o.id, o.opened_by, p.payment_method_id, o.branch_id,
			       (select r.id from cash_registers r where r.company_id = o.company_id and r.is_primary
			          and r.branch_id = o.branch_id order by r.id limit 1)
			  from orders o join order_payments p on p.order_id = o.id
			 where o.company_id <> $1 and o.branch_id is not null
			 order by o.id desc limit 1`, not).
			Scan(&e.id, &e.order, &e.user, &e.method, &e.branch, &e.register); err != nil {
			t.Fatalf("un pedido cobrado de una empresa distinta de %d: %v", not, err)
		}
		return e
	}
	a := pick(0)
	b := pick(a.id)

	var refundsBefore int
	var refundedBefore decimal.Decimal
	if err := st.Pool.QueryRow(ctx, `select count(*), coalesce(sum(amount), 0) from order_refunds`).
		Scan(&refundsBefore, &refundedBefore); err != nil {
		t.Fatal(err)
	}

	// Fixtures, como owner y con company_id explícito. Se borran antes de bajar la migración: el Down
	// se niega con una devolución de solo propina, y el respaldo tiene que quedar como llegó.
	var created struct {
		sessions, movements, refunds []int64
	}
	t.Cleanup(func() {
		for _, id := range created.refunds {
			_, _ = st.Pool.Exec(ctx, `delete from order_refunds where id = $1`, id)
		}
		for _, id := range created.movements {
			_, _ = st.Pool.Exec(ctx, `delete from register_cash_movements where id = $1`, id)
		}
		for _, id := range created.sessions {
			_, _ = st.Pool.Exec(ctx, `delete from register_sessions where id = $1`, id)
		}
	})
	newSession := func(e empresa, status string, openedAt time.Time) int64 {
		var id int64
		if err := st.Pool.QueryRow(ctx, `
			insert into register_sessions (company_id, business_date, opening_cash, opened_by, register_id, status, opened_at, closed_at)
			values ($1, $2::timestamptz::date, 0, $3, $4, $5::session_status, $2,
			        case when $5 = 'cerrada' then $2::timestamptz + interval '8 hours' end)
			returning id`, e.id, openedAt, e.user, e.register, status).Scan(&id); err != nil {
			t.Fatalf("turno %s de la empresa %d: %v", status, e.id, err)
		}
		created.sessions = append(created.sessions, id)
		return id
	}
	newRefund := func(e empresa, at time.Time, movement *int64) int64 {
		var id int64
		if err := st.Pool.QueryRow(ctx, `
			insert into order_refunds (company_id, order_id, payment_method_id, amount, reason, refunded_by, cash_movement_id, created_at)
			values ($1, $2, $3, 10, 'prueba 0080', $4, $5, $6) returning id`,
			e.id, e.order, e.method, e.user, movement, at).Scan(&id); err != nil {
			t.Fatalf("devolución de la empresa %d: %v", e.id, err)
		}
		created.refunds = append(created.refunds, id)
		return id
	}

	// A: un turno cerrado con una devolución en efectivo (con su salida) y otra por tarjeta (sin ella).
	closedAt := time.Date(2026, 9, 30, 14, 0, 0, 0, time.UTC)
	closedA := newSession(a, "cerrada", closedAt)
	var movement int64
	if err := st.Pool.QueryRow(ctx, `
		insert into register_cash_movements (company_id, session_id, kind, amount, concept, user_id)
		values ($1, $2, 'salida', 10, 'Devolución: prueba 0080', $3) returning id`, a.id, closedA, a.user).Scan(&movement); err != nil {
		t.Fatal(err)
	}
	created.movements = append(created.movements, movement)
	cashRefund := newRefund(a, closedAt.Add(time.Hour), &movement)
	cardInClosed := newRefund(a, closedAt.Add(2*time.Hour), nil)
	// 23:30 del 30 de septiembre en la Ciudad de México son las 05:30 UTC del 1 de octubre.
	nearMidnight := newRefund(a, time.Date(2026, 10, 1, 5, 30, 0, 0, time.UTC), nil)

	// B: el turno principal que sigue ABIERTO al migrar, con una devolución por tarjeta adentro y otra
	// de antes de que abriera. Si ya hay uno abierto en el respaldo se usa ése: es el caso real.
	var openB int64
	var openedB time.Time
	err := st.Pool.QueryRow(ctx, `
		select s.id, s.opened_at from register_sessions s join cash_registers r on r.id = s.register_id
		 where s.company_id = $1 and s.status = 'abierta' and r.is_primary and r.branch_id = $2`, b.id, b.branch).
		Scan(&openB, &openedB)
	if err == pgx.ErrNoRows {
		openedB = time.Now().Add(-3 * time.Hour)
		openB = newSession(b, "abierta", openedB)
	} else if err != nil {
		t.Fatal(err)
	}
	cardInOpen := newRefund(b, openedB.Add(time.Minute), nil)
	cardBeforeOpen := newRefund(b, openedB.Add(-time.Hour), nil)

	migrarArriba(t, st.Pool)
	if v := versionDeEsquema(t, st); v < dayOfEachPaymentVersion {
		t.Fatalf("versión %d después de migrar, quiere al menos %d", v, dayOfEachPaymentVersion)
	}

	t.Run("el dinero devuelto no cambia", func(t *testing.T) {
		var n int
		var sum, tips decimal.Decimal
		if err := st.Pool.QueryRow(ctx, `select count(*), coalesce(sum(amount), 0), coalesce(sum(tip_amount), 0)
			from order_refunds where id <> all($1)`, created.refunds).Scan(&n, &sum, &tips); err != nil {
			t.Fatal(err)
		}
		if n != refundsBefore || !sum.Equal(refundedBefore) || !tips.IsZero() {
			t.Fatalf("devoluciones %d→%d, monto %s→%s, propina %s: la migración no mueve dinero",
				refundsBefore, n, refundedBefore, sum, tips)
		}
	})

	sessionOf := func(id int64) *int64 {
		var s *int64
		if err := st.Pool.QueryRow(ctx, `select register_session_id from order_refunds where id = $1`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	t.Run("cada devolución queda en su turno", func(t *testing.T) {
		if s := sessionOf(cashRefund); s == nil || *s != closedA {
			t.Errorf("la de efectivo quedó en %v, quiere %d: el turno de su salida de caja", s, closedA)
		}
		if s := sessionOf(cardInClosed); s != nil {
			t.Errorf("la de tarjeta de un turno cerrado quedó en %d: su corte ya guardó su cifra", *s)
		}
		if s := sessionOf(cardInOpen); s == nil || *s != openB {
			t.Errorf("la de tarjeta del turno abierto quedó en %v, quiere %d: sin él ese corte no la resta", s, openB)
		}
		if s := sessionOf(cardBeforeOpen); s != nil {
			t.Errorf("la de antes de abrir quedó en %d: no es de ese turno", *s)
		}
	})

	t.Run("el día es el de la zona del negocio", func(t *testing.T) {
		var day time.Time
		if err := st.Pool.QueryRow(ctx, `select business_date from order_refunds where id = $1`, nearMidnight).Scan(&day); err != nil {
			t.Fatal(err)
		}
		if got := day.Format("2006-01-02"); got != "2026-09-30" {
			t.Fatalf("la devolución de las 23:30 del 30 de septiembre quedó el %s", got)
		}
		var sinDia int
		if err := st.Pool.QueryRow(ctx, `select count(*) from order_refunds where business_date is null`).Scan(&sinDia); err != nil {
			t.Fatal(err)
		}
		if sinDia != 0 {
			t.Fatalf("%d devoluciones sin día", sinDia)
		}
	})

	t.Run("cada pago anterior queda en el día de su pedido", func(t *testing.T) {
		var distinto, sinDia int
		if err := st.Pool.QueryRow(ctx, `
			select count(*) filter (where op.business_date <> o.business_date), count(*) filter (where op.business_date is null)
			  from order_payments op join orders o on o.id = op.order_id`).Scan(&distinto, &sinDia); err != nil {
			t.Fatal(err)
		}
		if distinto+sinDia != 0 {
			t.Fatalf("%d pagos en otro día y %d sin día: lo histórico tiene que reportarse como hasta hoy", distinto, sinDia)
		}
	})

	t.Run("una devolución no se cuelga del turno de otra empresa", func(t *testing.T) {
		err := inRolledBackTx(t, st.Pool, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `update order_refunds set register_session_id = $2 where id = $1`, cardBeforeOpen, closedA)
			return err
		})
		exigeViolacionDeRestriccion(t, err, "ligar una devolución al turno de otra empresa")
	})

	t.Run("el rol de la aplicación escribe las columnas nuevas", func(t *testing.T) {
		conn := conexionDeEmpresa(t, restoredAppRoleStore(t), a.id)
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, `
			insert into order_refunds (order_id, payment_method_id, amount, tip_amount, reason, refunded_by, register_session_id, business_date)
			values ($1, $2, 0, 5, 'solo propina', $3, $4, '2026-10-01')`, a.order, a.method, a.user, closedA); err != nil {
			t.Fatalf("insertar como gatobobah_app una devolución de solo propina: %v", err)
		}
		if _, err := tx.Exec(ctx, `update order_refunds set register_session_id = $2 where id = $1`, cardInClosed, closedA); err != nil {
			t.Fatalf("reclamar una devolución como gatobobah_app: %v", err)
		}
		if _, err := tx.Exec(ctx, `update order_payments set business_date = business_date where order_id = $1`, a.order); err != nil {
			t.Fatalf("escribir el día del pago como gatobobah_app: %v", err)
		}
	})

	t.Run("una devolución de cero no entra", func(t *testing.T) {
		err := inRolledBackTx(t, st.Pool, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `insert into order_refunds (company_id, order_id, payment_method_id, amount, tip_amount, reason, refunded_by)
				values ($1, $2, $3, 0, 0, 'nada', $4)`, a.id, a.order, a.method, a.user)
			return err
		})
		exigeViolacionDeRestriccion(t, err, "una devolución sin monto ni propina")
	})

	t.Run("el Down se niega si perdería una propina devuelta", func(t *testing.T) {
		var tipOnly int64
		if err := st.Pool.QueryRow(ctx, `
			insert into order_refunds (company_id, order_id, payment_method_id, amount, tip_amount, reason, refunded_by)
			values ($1, $2, $3, 0, 5, 'solo propina', $4) returning id`, a.id, a.order, a.method, a.user).Scan(&tipOnly); err != nil {
			t.Fatal(err)
		}
		db := prepararGoose(t, st.Pool)
		if err := gooseDownTo(db, before); err == nil {
			t.Fatal("el Down borró una devolución de solo propina: ese dinero salió del cajón y ya no habría rastro")
		}
		if _, err := st.Pool.Exec(ctx, `delete from order_refunds where id = $1`, tipOnly); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("el Down limpio deja el esquema como estaba", func(t *testing.T) {
		for _, id := range created.refunds {
			if _, err := st.Pool.Exec(ctx, `delete from order_refunds where id = $1`, id); err != nil {
				t.Fatal(err)
			}
		}
		created.refunds = nil
		migrarAbajoHasta(t, st.Pool, before)
		var cols int
		if err := st.Pool.QueryRow(ctx, `select count(*) from information_schema.columns
			where (table_name = 'order_refunds' and column_name in ('register_session_id', 'business_date', 'tip_amount'))
			   or (table_name = 'order_payments' and column_name = 'business_date')`).Scan(&cols); err != nil {
			t.Fatal(err)
		}
		if cols != 0 {
			t.Fatalf("quedan %d columnas de la 0080 después del Down", cols)
		}
		migrarArriba(t, st.Pool)
	})
}

// gooseDownTo es migrarAbajoHasta devolviendo el error en vez de fallar: un Down que DEBE negarse
// se prueba viendo ese error.
func gooseDownTo(db *sql.DB, version int64) error {
	return goose.DownToContext(context.Background(), db, ".", version)
}
