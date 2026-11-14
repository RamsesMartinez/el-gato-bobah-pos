//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// openMCPShift abre el turno de la caja con la hora real, como lo abre la API: la API usa el reloj de
// verdad para decidir a qué corte va un pago en efectivo. El día es el de la zona de la empresa, nunca
// `current_date` (la zona de la sesión de Postgres).
func openMCPShift(t *testing.T, q queryRower, company, admin, register int64) int64 {
	t.Helper()
	var id int64
	if err := q.QueryRow(context.Background(), `insert into register_sessions (business_date, opening_cash, opened_by, register_id)
	     values (business_day_of($1, now()), 1000, $2, $3) returning id`, company, admin, register).Scan(&id); err != nil {
		t.Fatalf("abrir el turno: %v", err)
	}
	return id
}

// TestMCPSeedShiftIsOnTheBusinessDay: el turno de la semilla del MCP abría con `current_date`, el día
// de la zona de la SESIÓN de Postgres (UTC en CI). De 18:00 a 24:00 del centro ese ya es mañana: el
// gasto tomaba el día del turno y la entrega el día del negocio, quedaba «entregado antes de comprarse»
// y TestMCPExpenseEditFixesAPurchase moría en la vista previa de la edición. La API abre el turno con
// el día de la zona de la empresa (OpenSession); la semilla tiene que hacer lo mismo a cualquier hora.
func TestMCPSeedShiftIsOnTheBusinessDay(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	ctx := context.Background()
	var register int64
	if err := w.owner.Pool.QueryRow(ctx, `select id from cash_registers where company_id = $1 and is_primary and is_active limit 1`,
		w.company).Scan(&register); err != nil {
		t.Fatal(err)
	}

	tx, err := w.owner.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Una zona de sesión cuyo día NO es el del negocio, ahora mismo. Entre UTC+14 y UTC−12 hay 26 horas:
	// a cualquier hora, al menos una de las dos cae en otro día que la zona de la empresa. Así el test
	// falla sin depender de la hora en que corra el CI.
	var businessDay time.Time
	if err := tx.QueryRow(ctx, `select business_day_of($1, now())`, w.company).Scan(&businessDay); err != nil {
		t.Fatal(err)
	}
	zone := ""
	for _, z := range []string{"Etc/GMT-14", "Etc/GMT+12"} {
		var differs bool
		if err := tx.QueryRow(ctx, `select (now() at time zone $1)::date <> $2::date`, z, businessDay).Scan(&differs); err != nil {
			t.Fatal(err)
		}
		if differs {
			zone = z
			break
		}
	}
	if zone == "" {
		t.Fatalf("ninguna zona de prueba cae en otro día que %s", businessDay.Format(time.DateOnly))
	}
	if _, err := tx.Exec(ctx, `select set_config('TimeZone', $1, true)`, zone); err != nil {
		t.Fatal(err)
	}

	session := openMCPShift(t, tx, w.company, w.admin, register)
	var shiftDay time.Time
	if err := tx.QueryRow(ctx, `select business_date from register_sessions where id = $1`, session).Scan(&shiftDay); err != nil {
		t.Fatal(err)
	}
	if !shiftDay.Equal(businessDay) {
		t.Fatalf("con la sesión de Postgres en %s el turno abrió el %s y el día del negocio es el %s: "+
			"un gasto pagado en ese turno queda después de su entrega y ya no se puede editar",
			zone, shiftDay.Format(time.DateOnly), businessDay.Format(time.DateOnly))
	}
}
