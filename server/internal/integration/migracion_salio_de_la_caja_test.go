//go:build integration

package integration

import (
	"context"
	"testing"
)

// drawerExtraVersion es la migración de lo que salió del cajón además del importe (spec 050).
// PROVISIONAL: se mueve al siguiente número libre de develop al fusionar, y esta constante con ella.
const drawerExtraVersion = 102

// LA 0102 CON DOS EMPRESAS (constitución IV). Solo agrega: los pagos que ya existían quedan con
// extra 0 (salió de la caja = su importe), un extra negativo se rechaza, la salida de un pago se
// registra por importe + extra aunque el INSERT mande otro monto (la API vieja durante la subida),
// y el Down se niega en cuanto un pago lleva extra.
func TestMigrationDrawerExtra(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	coB := makeCompany(t, st, "mig-salio-de-la-caja-b")
	migrarAbajoHasta(t, st.Pool, drawerExtraVersion-1)

	userA := makeUser(t, st, "gerente_mig50_a", "gerente")
	regA := principalRegister(t, st)
	userB := makeUserIn(t, st, coB, "gerente_mig50_b", "gerente")
	var regB int64
	if err := st.Pool.QueryRow(ctx, `insert into cash_registers (company_id, name, is_primary) values ($1, 'Principal B', true) returning id`, coB).Scan(&regB); err != nil {
		t.Fatalf("caja principal de B: %v", err)
	}
	sessA := turnoDeCaja(t, st, defaultCompanyID, regA, userA, "abierta")
	sessB := turnoDeCaja(t, st, coB, regB, userB, "abierta")
	payA := pagoEnEfectivoViejo(t, st, defaultCompanyID, regA, userA, nil, "2026-10-01")
	payB := pagoEnEfectivoViejo(t, st, coB, regB, userB, nil, "2026-10-01")

	migrarArriba(t, st.Pool)

	for _, pay := range []int64{payA, payB} {
		var extra string
		if err := st.Pool.QueryRow(ctx, `select drawer_extra::text from expense_payments where id = $1`, pay).Scan(&extra); err != nil {
			t.Fatal(err)
		}
		if extra != "0.00" {
			t.Fatalf("el pago viejo %d quedó con extra %s: un pago ya guardado cambió lo que salió del cajón", pay, extra)
		}
	}
	if _, err := st.Pool.Exec(ctx, `update expense_payments set drawer_extra = -1 where id = $1`, payA); err == nil {
		t.Fatal("un extra negativo se aceptó")
	}

	// La salida la fija la base con importe + extra, también si el INSERT trae otro monto.
	if _, err := st.Pool.Exec(ctx, `update expense_payments set drawer_extra = 20 where id = $1`, payB); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		company, sess, pay, user int64
		want                     string
	}{{defaultCompanyID, sessA, payA, userA, "100.00"}, {coB, sessB, payB, userB, "120.00"}} {
		var amount string
		if err := st.Pool.QueryRow(ctx, `insert into register_cash_movements (company_id, session_id, kind, amount, concept, user_id, expense_payment_id)
			values ($1, $2, 'salida', 1, 'Gasto', $3, $4) returning amount::text`, c.company, c.sess, c.user, c.pay).Scan(&amount); err != nil {
			t.Fatalf("salida del pago %d: %v", c.pay, err)
		}
		if amount != c.want {
			t.Fatalf("la salida del pago %d quedó en %s; quería %s (importe + extra)", c.pay, amount, c.want)
		}
	}
	// Un movimiento que no es salida de un pago no se toca.
	var free string
	if err := st.Pool.QueryRow(ctx, `insert into register_cash_movements (company_id, session_id, kind, amount, concept, user_id)
		values ($1, $2, 'entrada', 7, 'Cambio', $3) returning amount::text`, defaultCompanyID, sessA, userA).Scan(&free); err != nil || free != "7.00" {
		t.Fatalf("una entrada a mano quedó en %s (%v)", free, err)
	}

	if err := migrarAbajoHastaErr(t, st.Pool, drawerExtraVersion-1); err == nil {
		t.Fatal("el Down corrió con un pago con extra: perdería lo que salió del cajón")
	}
	if _, err := st.Pool.Exec(ctx, `update expense_payments set drawer_extra = 0`); err != nil {
		t.Fatal(err)
	}
	migrarAbajoHasta(t, st.Pool, drawerExtraVersion-1)
	migrarArriba(t, st.Pool)
}
