//go:build integration

package integration

import (
	"context"
	"testing"
)

// draftVersionVersion es la migración que le da versión a la cuenta entera. Su número se mueve al
// siguiente libre de develop al fusionar, y esta constante con él.
const draftVersionVersion = 83

// LA VERSIÓN DE LA CUENTA SOBRE UN RESPALDO REAL CON DOS EMPRESAS.
//
// Las cuentas que ya se estaban capturando al desplegar tienen que nacer con una versión (si no, la
// tableta que las tiene abiertas no podría descartarlas), el rol de la aplicación tiene que poder
// avanzarla bajo RLS, y el Down no puede llevarse nada más que la columna.
func TestMigrationDraftVersionOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start >= draftVersionVersion {
		migrarAbajoHasta(t, st.Pool, draftVersionVersion-1)
	} else if start < draftVersionVersion-1 {
		migrarArriba(t, st.Pool)
		migrarAbajoHasta(t, st.Pool, draftVersionVersion-1)
	}
	t.Cleanup(func() {
		if v := versionDeEsquema(t, st); v != start && start < draftVersionVersion {
			migrarAbajoHasta(t, st.Pool, start)
		}
	})

	count := func(t *testing.T) (drafts, live int) {
		t.Helper()
		if err := st.Pool.QueryRow(ctx,
			`select count(*), count(*) filter (where status = 'capturando') from order_drafts`).Scan(&drafts, &live); err != nil {
			t.Fatal(err)
		}
		return drafts, live
	}
	draftsBefore, liveBefore := count(t)
	migrarArriba(t, st.Pool)

	t.Run("las cuentas que ya existían quedan en la versión 1 y ninguna se mueve", func(t *testing.T) {
		drafts, live := count(t)
		if drafts != draftsBefore || live != liveBefore {
			t.Fatalf("cuentas %d→%d, vivas %d→%d: la migración solo agrega una columna", draftsBefore, drafts, liveBefore, live)
		}
		var otherThanOne int
		if err := st.Pool.QueryRow(ctx, `select count(*) from order_drafts where version <> 1`).Scan(&otherThanOne); err != nil {
			t.Fatal(err)
		}
		if otherThanOne != 0 {
			t.Fatalf("%d cuentas nacieron con otra versión", otherThanOne)
		}
	})

	t.Run("el rol de la aplicación avanza la versión de SU empresa y no ve la de otra", func(t *testing.T) {
		var a, b int64
		if err := st.Pool.QueryRow(ctx, `select min(id), max(id) from companies`).Scan(&a, &b); err != nil {
			t.Fatal(err)
		}
		conn := conexionDeEmpresa(t, restoredAppRoleStore(t), a)
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, `update order_drafts set version = version + 1`); err != nil {
			t.Fatalf("avanzar la versión como gatobobah_app: %v", err)
		}
		var foreign int
		if err := tx.QueryRow(ctx, `select count(*) from order_drafts where company_id = $1`, b).Scan(&foreign); err != nil {
			t.Fatal(err)
		}
		if foreign != 0 {
			t.Fatalf("la empresa %d ve %d cuentas de la %d", a, foreign, b)
		}
	})

	t.Run("el Down quita la columna y nada más", func(t *testing.T) {
		migrarAbajoHasta(t, st.Pool, draftVersionVersion-1)
		drafts, live := count(t)
		if drafts != draftsBefore || live != liveBefore {
			t.Fatalf("cuentas %d→%d, vivas %d→%d tras el Down", draftsBefore, drafts, liveBefore, live)
		}
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from information_schema.columns
			where table_name = 'order_drafts' and column_name = 'version'`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("la columna sigue tras el Down (n=%d, err=%v)", n, err)
		}
		migrarArriba(t, st.Pool)
	})
}
