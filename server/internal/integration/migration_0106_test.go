//go:build integration

package integration

import (
	"context"
	"testing"
)

// closedPacksVersion guarda cuántos cerrados de cada empaque lleva una pesada del conteo (#204).
const closedPacksVersion = 106

// LA 0106 SOBRE UN RESPALDO REAL CON DOS EMPRESAS (constitución IV).
//
// Solo agrega una columna nula a los renglones del conteo. Lo que solo se ve sobre datos reales: que
// ningún renglón existente viola el check nuevo ni se reescribe, y que bajar no pierde nada más.
func TestMigrationCountClosedPacksOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start < closedPacksVersion-1 {
		migrarArriba(t, st.Pool)
	}
	migrarAbajoHasta(t, st.Pool, closedPacksVersion-1)
	t.Cleanup(func() {
		if versionDeEsquema(t, st) < start {
			migrarArriba(t, st.Pool)
		}
	})
	var lines, weighed int64
	if err := st.Pool.QueryRow(ctx, `select count(*), count(gross_g) from stock_count_lines`).Scan(&lines, &weighed); err != nil {
		t.Fatal(err)
	}
	migrarArriba(t, st.Pool)

	t.Run("ningún renglón cambia y la columna nace vacía", func(t *testing.T) {
		var ln, wd, withPacks int64
		if err := st.Pool.QueryRow(ctx, `select count(*), count(gross_g), count(closed_packs) from stock_count_lines`).Scan(&ln, &wd, &withPacks); err != nil {
			t.Fatal(err)
		}
		if ln != lines || wd != weighed || withPacks != 0 {
			t.Fatalf("renglones %d→%d, pesados %d→%d, con cerrados por empaque %d", lines, ln, weighed, wd, withPacks)
		}
	})

	t.Run("bajar quita la columna y deja los renglones", func(t *testing.T) {
		migrarAbajoHasta(t, st.Pool, closedPacksVersion-1)
		var ln int64
		if err := st.Pool.QueryRow(ctx, `select count(*) from stock_count_lines`).Scan(&ln); err != nil || ln != lines {
			t.Fatalf("renglones %d→%d, %v", lines, ln, err)
		}
		migrarArriba(t, st.Pool)
	})
}
