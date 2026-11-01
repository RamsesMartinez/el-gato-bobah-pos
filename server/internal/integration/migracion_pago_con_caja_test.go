//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// paymentRegisterVersion es la migración de la caja del pago de gasto (spec 047). PROVISIONAL: se
// mueve al siguiente número libre de develop al fusionar, y esta constante con ella.
const paymentRegisterVersion = 99

// pagoViejo siembra como owner un gasto de `company` pagado en efectivo desde un turno de `reg`,
// con el esquema de antes de la 0099 (sin register_id). Devuelve el pago.
func pagoViejo(t *testing.T, st *store.Store, company, reg, user int64) int64 {
	t.Helper()
	ctx := context.Background()
	var sess, cat, exp, method, pay int64
	if err := st.Pool.QueryRow(ctx, `insert into register_sessions (company_id, register_id, opened_by, business_date, status, closed_at, closed_by)
		values ($1, $2, $3, current_date, 'cerrada', now(), $3) returning id`, company, reg, user).Scan(&sess); err != nil {
		t.Fatalf("turno: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into expense_categories (company_id, name, financial_group) values ($1, 'Mig 47 ' || gen_random_uuid()::text, 'operacional') returning id`, company).Scan(&cat); err != nil {
		t.Fatalf("categoría: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into expenses (company_id, category_id, amount, expense_date, created_by) values ($1, $2, 100, current_date, $3) returning id`, company, cat, user).Scan(&exp); err != nil {
		t.Fatalf("gasto: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, `select id from payment_methods where company_id = $1 and affects_cash_drawer limit 1`, company).Scan(&method); err != nil {
		t.Fatalf("método en efectivo: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into expense_payments (company_id, expense_id, payment_method_id, amount, paid_on, register_session_id, paid_by)
		values ($1, $2, $3, 100, current_date, $4, $5) returning id`, company, exp, method, sess, user).Scan(&pay); err != nil {
		t.Fatalf("pago: %v", err)
	}
	return pay
}

// LA 0099 CON DOS EMPRESAS (constitución IV). Solo agrega: rellena la caja de los pagos que ya
// tenían turno, la deriva del turno también para la API vieja que no la manda, no deja que un pago
// apunte a una caja de otra empresa (la FK salta RLS), no deja a la principal sin arqueo, y su Down
// se niega en cuanto existe un pago en efectivo fuera de corte, que perdería su caja.
func TestMigrationPaymentRegister(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	coB := makeCompany(t, st, "mig-pago-caja-b")
	migrarAbajoHasta(t, st.Pool, paymentRegisterVersion-1)

	userA := makeUser(t, st, "gerente_mig47_a", "gerente")
	regA := principalRegister(t, st)
	userB := makeUserIn(t, st, coB, "gerente_mig47_b", "gerente")
	var regB int64
	if err := st.Pool.QueryRow(ctx, `insert into cash_registers (company_id, name, is_primary) values ($1, 'Principal B', true) returning id`, coB).Scan(&regB); err != nil {
		t.Fatalf("caja principal de B: %v", err)
	}
	payA := pagoViejo(t, st, defaultCompanyID, regA, userA)
	payB := pagoViejo(t, st, coB, regB, userB)

	migrarArriba(t, st.Pool)

	cajaDe := func(pay int64) *int64 {
		var r *int64
		if err := st.Pool.QueryRow(ctx, `select register_id from expense_payments where id = $1`, pay).Scan(&r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := cajaDe(payA); r == nil || *r != regA {
		t.Fatalf("el pago viejo de A quedó con caja %v, esperaba la de su turno %d", r, regA)
	}
	if r := cajaDe(payB); r == nil || *r != regB {
		t.Fatalf("el pago viejo de B quedó con caja %v, esperaba la de su turno %d", r, regB)
	}

	// La API vieja, durante la subida, inserta sin register_id: el trigger lo toma del turno.
	if r := cajaDe(pagoViejo(t, st, defaultCompanyID, regA, userA)); r == nil || *r != regA {
		t.Fatalf("un pago con turno insertado sin caja quedó con %v: la API vieja deja pagos sin caja", r)
	}

	var cat, exp, method int64
	_ = st.Pool.QueryRow(ctx, `select category_id, id from expenses where company_id = $1 limit 1`, defaultCompanyID).Scan(&cat, &exp)
	_ = st.Pool.QueryRow(ctx, `select id from payment_methods where company_id = $1 and affects_cash_drawer limit 1`, defaultCompanyID).Scan(&method)
	if _, err := st.Pool.Exec(ctx, `insert into expense_payments (company_id, expense_id, payment_method_id, amount, paid_on, register_id, paid_by)
		values ($1, $2, $3, 10, current_date, $4, $5)`, defaultCompanyID, exp, method, regB, userA); err == nil {
		t.Fatal("un pago de la empresa A apuntando a una caja de B se aceptó: la FK cruza empresas")
	}
	if _, err := st.Pool.Exec(ctx, `update cash_registers set counts_cash = false where id = $1`, regA); err == nil {
		t.Fatal("la caja principal quedó sin arqueo: sus ventas en efectivo no tendrían corte")
	}

	// Un pago en efectivo fuera de corte: el Down lo perdería, y se niega.
	if _, err := st.Pool.Exec(ctx, `insert into expense_payments (company_id, expense_id, payment_method_id, amount, paid_on, register_id, paid_by)
		values ($1, $2, $3, 10, current_date, $4, $5)`, defaultCompanyID, exp, method, regA, userA); err != nil {
		t.Fatalf("pago fuera de corte: %v", err)
	}
	if err := migrarAbajoHastaErr(t, st.Pool, paymentRegisterVersion-1); err == nil {
		t.Fatal("el Down corrió con un pago fuera de corte: ese pago perdió su caja")
	}
}
