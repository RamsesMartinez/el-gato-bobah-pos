//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

const cashToleranceVersion = 114

// LA 0114 (tolerancia de diferencia, #290 fase 4) CONTRA UN RESPALDO REAL CON DOS EMPRESAS. Es una
// columna con default: cada empresa queda en $0, la API anterior sigue leyendo los ajustes, y un
// negativo se rechaza en la base.
func TestMigration0114OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	migrarAbajoHasta(t, st.Pool, cashToleranceVersion-1)
	t.Cleanup(func() { migrarArriba(t, st.Pool) })
	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", cashToleranceVersion); err != nil {
		t.Fatal(err)
	}
	var empresas, enCero int
	if err := st.Pool.QueryRow(ctx, `select count(*), count(*) filter (where cash_difference_tolerance = 0) from business_settings`).Scan(&empresas, &enCero); err != nil {
		t.Fatal(err)
	}
	if empresas < 2 || enCero != empresas {
		t.Fatalf("%d empresas, %d con tolerancia en $0: toda empresa arranca sin aceptar diferencias", empresas, enCero)
	}
	_, err := st.Pool.Exec(ctx, `update business_settings set cash_difference_tolerance = -1`)
	exigeViolacionDeRestriccion(t, err, "una tolerancia negativa")
	if err := goose.DownContext(ctx, prepararGoose(t, st.Pool), "."); err != nil {
		t.Fatalf("el Down de la 0114: %v", err)
	}
}
