//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// LO PERDIDO NUNCA SUPERA EL TOTAL DEL PEDIDO (dueño, 2026-10-09; migración 0086).
//
// El tope vivía solo en el servidor. Una escritura por otro camino que lo pasara dejaba
// `total - written_off_amount` negativo en el pendiente de Ventas y en los pedidos que bloquean el
// cierre: un «debe» negativo que resta de lo que sí se debe.
func TestWriteOffCannotExceedTheOrderTotal(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	o := k.order(t, "1", "40", true)

	_, err := k.st.Pool.Exec(k.ctx, `update orders set written_off_amount = total + 1, written_off_reason = 'x',
		written_off_by = $2, written_off_at = now(), written_off_business_date = current_date where id = $1`, o.ID, k.user)
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "23514" {
		t.Fatalf("dar por perdido más que el total = %v, quiere violación de check (23514)", err)
	}

	// El que dio algo por perdido no se borra: el rastro del dinero perdido no desaparece por arrastre.
	var regla string
	if err := k.st.Pool.QueryRow(k.ctx, `select confdeltype from pg_constraint
		where conrelid = 'orders'::regclass and contype = 'f'
		  and conkey = array[(select attnum from pg_attribute where attrelid = 'orders'::regclass and attname = 'written_off_by')]`).Scan(&regla); err != nil {
		t.Fatal(err)
	}
	if regla != "r" {
		t.Fatalf("written_off_by on delete = %q, quiere restrict (r)", regla)
	}

	// Ventas filtra lo perdido por día; el índice empieza por company_id porque RLS agrega ese predicado.
	var def string
	if err := k.st.Pool.QueryRow(k.ctx, `select indexdef from pg_indexes where indexname = 'orders_company_written_off_day'`).Scan(&def); err != nil {
		t.Fatalf("falta el índice del día de lo perdido: %v", err)
	}
}
