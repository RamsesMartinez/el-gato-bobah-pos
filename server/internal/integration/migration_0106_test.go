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

	// Un renglón real puesto como lo deja una pesada con reparto: es sobre el que actúan la API anterior
	// y el Down.
	var line struct{ count, ingredient int64 }
	if lines == 0 {
		t.Fatal("el respaldo no trae renglones de conteo: las pruebas de abajo no probarían nada")
	}
	weigh := func(t *testing.T) {
		t.Helper()
		if err := st.Pool.QueryRow(ctx, `update stock_count_lines set gross_g = 100, gross_containers = 1, tare_g_used = 10,
			closed_packs = '[]' where (count_id, ingredient_id) = (select count_id, ingredient_id from stock_count_lines order by count_id, ingredient_id limit 1)
			returning count_id, ingredient_id`).Scan(&line.count, &line.ingredient); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("la API anterior recaptura sin pesar y el reparto se va con la pesada, sin tronar", func(t *testing.T) {
		weigh(t)
		if _, err := st.Pool.Exec(ctx, `update stock_count_lines set gross_g = null, gross_containers = null, tare_g_used = null, density_used = null
			where count_id = $1 and ingredient_id = $2`, line.count, line.ingredient); err != nil {
			t.Fatalf("el upsert de la API anterior truena: %v", err)
		}
		var packs *string
		if err := st.Pool.QueryRow(ctx, `select closed_packs::text from stock_count_lines where count_id = $1 and ingredient_id = $2`,
			line.count, line.ingredient).Scan(&packs); err != nil || packs != nil {
			t.Fatalf("quedó el reparto %v sin pesada (%v)", packs, err)
		}
	})

	t.Run("bajar con un reparto guardado quita la columna y deja los renglones", func(t *testing.T) {
		weigh(t)
		migrarAbajoHasta(t, st.Pool, closedPacksVersion-1)
		var ln int64
		if err := st.Pool.QueryRow(ctx, `select count(*) from stock_count_lines`).Scan(&ln); err != nil || ln != lines {
			t.Fatalf("renglones %d→%d, %v", lines, ln, err)
		}
		migrarArriba(t, st.Pool)
	})
}
