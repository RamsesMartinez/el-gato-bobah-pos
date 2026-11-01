//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// excludedItemsVersion es la migración de los renglones excluidos y el total del ticket (spec 049).
// PROVISIONAL: se mueve al siguiente número libre de develop al fusionar, y esta constante con ella.
const excludedItemsVersion = 101

// gastoConRenglon siembra como owner un gasto de `company` con un renglón no inventariable, con el
// esquema de antes de la 0101. Devuelve el gasto y el renglón.
func gastoConRenglon(t *testing.T, st *store.Store, company, user int64) (exp, item int64) {
	t.Helper()
	ctx := context.Background()
	var cat int64
	if err := st.Pool.QueryRow(ctx, `insert into expense_categories (company_id, name, financial_group) values ($1, 'Mig 49 ' || gen_random_uuid()::text, 'operacional') returning id`, company).Scan(&cat); err != nil {
		t.Fatalf("categoría: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into expenses (company_id, category_id, amount, expense_date, created_by) values ($1, $2, 100, current_date, $3) returning id`, company, cat, user).Scan(&exp); err != nil {
		t.Fatalf("gasto: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, `insert into expense_items (company_id, expense_id, description, quantity, unit_cost, amount, position)
		values ($1, $2, 'Bolsa', 1, 50, 50, 0) returning id`, company, exp).Scan(&item); err != nil {
		t.Fatalf("renglón: %v", err)
	}
	return exp, item
}

// LA 0101 CON DOS EMPRESAS (constitución IV). Solo agrega: los renglones y gastos que ya existían
// quedan incluidos y sin total del ticket, un total negativo se rechaza, y el Down se niega en
// cuanto hay un excluido (volvería a contar en silencio) o un total del ticket (no se reconstruye).
func TestMigrationExcludedItems(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	coB := makeCompany(t, st, "mig-excluidos-b")
	migrarAbajoHasta(t, st.Pool, excludedItemsVersion-1)

	userA := makeUser(t, st, "gerente_mig49_a", "gerente")
	userB := makeUserIn(t, st, coB, "gerente_mig49_b", "gerente")
	expA, itemA := gastoConRenglon(t, st, defaultCompanyID, userA)
	expB, itemB := gastoConRenglon(t, st, coB, userB)

	migrarArriba(t, st.Pool)

	for _, c := range []struct{ exp, item int64 }{{expA, itemA}, {expB, itemB}} {
		var excluded bool
		var total *string
		if err := st.Pool.QueryRow(ctx, `select ei.excluded, e.document_total::text from expense_items ei join expenses e on e.id = ei.expense_id
			where ei.id = $1`, c.item).Scan(&excluded, &total); err != nil {
			t.Fatal(err)
		}
		if excluded || total != nil {
			t.Fatalf("el gasto viejo %d quedó con excluido=%v y total del ticket %v: un gasto ya guardado cambió", c.exp, excluded, total)
		}
	}

	if _, err := st.Pool.Exec(ctx, `update expenses set document_total = -1 where id = $1`, expA); err == nil {
		t.Fatal("un total del ticket negativo se aceptó")
	}
	// Un excluido con importe 0 se guarda igual (spec 049, bordes): es evidencia, no gasto.
	if _, err := st.Pool.Exec(ctx, `insert into expense_items (company_id, expense_id, description, quantity, unit_cost, amount, position, excluded)
		values ($1, $2, 'Muestra', 1, 0, 0, 1, true)`, defaultCompanyID, expA); err != nil {
		t.Fatalf("un excluido con importe 0 se rechazó: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `insert into expense_items (company_id, expense_id, description, quantity, unit_cost, amount, position)
		values ($1, $2, 'Cero', 1, 0, 0, 2)`, defaultCompanyID, expA); err == nil {
		t.Fatal("un renglón incluido con importe 0 y sin ser regalo se aceptó")
	}

	// Con un excluido, el Down se niega.
	if err := migrarAbajoHastaErr(t, st.Pool, excludedItemsVersion-1); err == nil {
		t.Fatal("el Down corrió con un renglón excluido: volvería a contar en silencio")
	}
	if _, err := st.Pool.Exec(ctx, `delete from expense_items where excluded`); err != nil {
		t.Fatal(err)
	}
	// Con un total del ticket, también.
	if _, err := st.Pool.Exec(ctx, `update expenses set document_total = 500 where id = $1`, expB); err != nil {
		t.Fatalf("total del ticket: %v", err)
	}
	if err := migrarAbajoHastaErr(t, st.Pool, excludedItemsVersion-1); err == nil {
		t.Fatal("el Down corrió con un total del ticket: ese dato no se reconstruye")
	}
	if _, err := st.Pool.Exec(ctx, `update expenses set document_total = null`); err != nil {
		t.Fatal(err)
	}
	// Sin ninguno de los dos, baja limpio.
	migrarAbajoHasta(t, st.Pool, excludedItemsVersion-1)
}
