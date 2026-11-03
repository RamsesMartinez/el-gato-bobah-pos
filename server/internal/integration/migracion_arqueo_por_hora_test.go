//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// countWindowVersion es la migración del arqueo por hora (spec 048).
const countWindowVersion = 100

// pagoEnEfectivoViejo siembra como owner, con el esquema de la 0099, un pago en efectivo de
// `company` desde la caja `reg`, del día `paidOn`; con `sess` lleva su salida de caja como la
// dejaba la API vieja (sin decir de qué pago es). Devuelve el pago.
func pagoEnEfectivoViejo(t *testing.T, st *store.Store, company, reg, user int64, sess *int64, paidOn string) int64 {
	t.Helper()
	ctx := context.Background()
	var cat, exp, method, pay int64
	if err := st.Pool.QueryRow(ctx, `insert into expense_categories (company_id, name, financial_group) values ($1, 'Mig 48 ' || gen_random_uuid()::text, 'operacional') returning id`, company).Scan(&cat); err != nil {
		t.Fatalf("categoría: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into expenses (company_id, category_id, amount, expense_date, created_by) values ($1, $2, 100, $3::date, $4) returning id`, company, cat, paidOn, user).Scan(&exp); err != nil {
		t.Fatalf("gasto: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, `select id from payment_methods where company_id = $1 and affects_cash_drawer limit 1`, company).Scan(&method); err != nil {
		t.Fatalf("método en efectivo: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into expense_payments (company_id, expense_id, payment_method_id, amount, paid_on, register_session_id, register_id, paid_by)
		values ($1, $2, $3, 100, $4::date, $5, $6, $7) returning id`, company, exp, method, paidOn, sess, reg, user).Scan(&pay); err != nil {
		t.Fatalf("pago: %v", err)
	}
	if sess != nil {
		if _, err := st.Pool.Exec(ctx, `insert into register_cash_movements (company_id, session_id, kind, amount, concept, expense_id, user_id)
			values ($1, $2, 'salida', 100, 'Gasto', $3, $4)`, company, *sess, exp, user); err != nil {
			t.Fatalf("salida del pago: %v", err)
		}
	}
	return pay
}

func turnoDeCaja(t *testing.T, st *store.Store, company, reg, user int64, status string) int64 {
	t.Helper()
	var id int64
	q := `insert into register_sessions (company_id, register_id, opened_by, business_date, status) values ($1, $2, $3, current_date, 'abierta') returning id`
	if status == "cerrada" {
		q = `insert into register_sessions (company_id, register_id, opened_by, business_date, status, closed_at, closed_by) values ($1, $2, $3, current_date, 'cerrada', now(), $3) returning id`
	}
	if err := st.Pool.QueryRow(context.Background(), q, company, reg, user).Scan(&id); err != nil {
		t.Fatalf("turno: %v", err)
	}
	return id
}

// LA 0100 CON DOS EMPRESAS (constitución IV). Solo agrega: rellena `include_in_count` (con corte =
// incluido) y `paid_at` (la hora de captura si se pagó ese día local; si no, su medianoche), liga
// las salidas viejas con su pago, sigue aceptando los inserts de la API vieja durante la subida, no
// deja un pago con corte «sin incluir» ni dos salidas del mismo pago, la liga no cruza empresas, y
// el Down se niega en cuanto hay un pendiente.
func TestMigrationCountWindow(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	coB := makeCompany(t, st, "mig-arqueo-hora-b")
	migrarAbajoHasta(t, st.Pool, countWindowVersion-1)

	userA := makeUser(t, st, "gerente_mig48_a", "gerente")
	regA := principalRegister(t, st)
	userB := makeUserIn(t, st, coB, "gerente_mig48_b", "gerente")
	var regB int64
	if err := st.Pool.QueryRow(ctx, `insert into cash_registers (company_id, name, is_primary) values ($1, 'Principal B', true) returning id`, coB).Scan(&regB); err != nil {
		t.Fatalf("caja principal de B: %v", err)
	}
	var hoy string
	if err := st.Pool.QueryRow(ctx, `select (now() at time zone 'America/Mexico_City')::date::text`).Scan(&hoy); err != nil {
		t.Fatal(err)
	}
	sessA := turnoDeCaja(t, st, defaultCompanyID, regA, userA, "cerrada")
	sessB := turnoDeCaja(t, st, coB, regB, userB, "cerrada")
	enCorteA := pagoEnEfectivoViejo(t, st, defaultCompanyID, regA, userA, &sessA, hoy)
	enCorteB := pagoEnEfectivoViejo(t, st, coB, regB, userB, &sessB, hoy)
	fueraA := pagoEnEfectivoViejo(t, st, defaultCompanyID, regA, userA, nil, "2026-07-01")

	migrarArriba(t, st.Pool)

	type pago struct {
		include bool
		paidAt  time.Time
		created time.Time
	}
	leer := func(id int64) pago {
		var p pago
		if err := st.Pool.QueryRow(ctx, `select include_in_count, paid_at, created_at from expense_payments where id = $1`, id).Scan(&p.include, &p.paidAt, &p.created); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for _, id := range []int64{enCorteA, enCorteB} {
		p := leer(id)
		if !p.include {
			t.Fatalf("el pago %d con corte quedó «sin incluir»: el check lo prohíbe y el relleno lo dejó así", id)
		}
		if !p.paidAt.Equal(p.created) {
			t.Fatalf("el pago %d pagado el día de su captura quedó a las %s; quería su hora de captura %s", id, p.paidAt, p.created)
		}
	}
	p := leer(fueraA)
	if p.include {
		t.Fatal("un pago viejo fuera de corte quedó incluido: el siguiente corte lo descontaría")
	}
	if want := time.Date(2026, 7, 1, 6, 0, 0, 0, time.UTC); !p.paidAt.Equal(want) {
		t.Fatalf("el pago de otro día quedó a las %s; quería la medianoche local de su día (%s)", p.paidAt.UTC(), want)
	}

	// La salida vieja quedó ligada a su pago, en las dos empresas.
	for _, c := range []struct{ pay, sess int64 }{{enCorteA, sessA}, {enCorteB, sessB}} {
		var liga *int64
		if err := st.Pool.QueryRow(ctx, `select expense_payment_id from register_cash_movements where session_id = $1`, c.sess).Scan(&liga); err != nil {
			t.Fatal(err)
		}
		if liga == nil || *liga != c.pay {
			t.Fatalf("la salida del turno %d quedó ligada a %v; quería el pago %d", c.sess, liga, c.pay)
		}
	}

	// La API vieja inserta sin las columnas nuevas: sin corte queda fuera, con corte incluido.
	abierto := turnoDeCaja(t, st, defaultCompanyID, regA, userA, "abierta")
	viejoConCorte := pagoEnEfectivoViejo(t, st, defaultCompanyID, regA, userA, &abierto, hoy)
	if !leer(viejoConCorte).include {
		t.Fatal("un pago de la API vieja con corte quedó «sin incluir»")
	}
	viejoSinCorte := pagoEnEfectivoViejo(t, st, defaultCompanyID, regA, userA, nil, hoy)
	if leer(viejoSinCorte).include {
		t.Fatal("un pago de la API vieja sin corte quedó pendiente: la tableta vieja dijo «fuera»")
	}

	if _, err := st.Pool.Exec(ctx, `update expense_payments set include_in_count = false where id = $1`, enCorteA); err == nil {
		t.Fatal("un pago con corte quedó «sin incluir»: su salida descuenta del corte algo que dice no contar")
	}
	if _, err := st.Pool.Exec(ctx, `insert into register_cash_movements (session_id, kind, amount, concept, user_id, expense_payment_id)
		values ($1, 'salida', 100, 'Gasto', $2, $3)`, sessA, userA, enCorteA); err == nil {
		t.Fatal("un pago tuvo dos salidas de caja: el doble descuento pasó la base")
	}
	if _, err := st.Pool.Exec(ctx, `insert into register_cash_movements (session_id, kind, amount, concept, user_id, expense_payment_id)
		values ($1, 'entrada', 100, 'Gasto', $2, $3)`, sessA, userA, enCorteB); err == nil {
		t.Fatal("un movimiento de la empresa A quedó ligado al pago de B: la FK cruza empresas")
	}

	// Un pendiente: el Down perdería que debe entrar a un corte, y se niega.
	if _, err := st.Pool.Exec(ctx, `update expense_payments set include_in_count = true where id = $1`, viejoSinCorte); err != nil {
		t.Fatal(err)
	}
	if err := migrarAbajoHastaErr(t, st.Pool, countWindowVersion-1); err == nil {
		t.Fatal("el Down corrió con un pago pendiente de corte: ese gasto ya no entraría a ninguno")
	}
}
