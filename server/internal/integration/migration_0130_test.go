//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

const expectedWasteDeductsVersion = 130

// LA 0129 Y LA 0130 (spec 096, subida 2) CONTRA UN RESPALDO REAL CON DOS EMPRESAS. Agregan el tipo de
// movimiento `merma_esperada` y la opción de descontarla (apagada), y quitan `ingredients.waste_pct`,
// que ninguna versión lee desde la subida 1. Lo que importa: subir no enciende nada ni cambia un
// costo, y bajar le regresa la columna a una versión vieja con lo capturado.
func TestMigration0130OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	migrarAbajoHasta(t, st.Pool, expectedWasteDeductsVersion-2)
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	var conPct int64
	if err := st.Pool.QueryRow(ctx, `select id from ingredients order by id limit 1`).Scan(&conPct); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, `update ingredients set expected_waste_pct = 6.5 where id = $1`, conPct); err != nil {
		t.Fatal(err)
	}

	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", expectedWasteDeductsVersion); err != nil {
		t.Fatalf("la 0129/0130 no subieron sobre el respaldo (la API nueva no arrancaría): %v", err)
	}

	t.Run("ninguna empresa nace descontando la merma", func(t *testing.T) {
		var encendidas, empresas int
		if err := st.Pool.QueryRow(ctx, `select count(*) filter (where expected_waste_deducts), count(*) from business_settings`).Scan(&encendidas, &empresas); err != nil {
			t.Fatal(err)
		}
		if empresas < 2 {
			t.Fatalf("el respaldo trae %d empresas; la prueba necesita dos", empresas)
		}
		if encendidas != 0 {
			t.Fatalf("%d empresas nacieron descontando la merma: el inventario bajaría sin que nadie lo pidiera", encendidas)
		}
	})

	t.Run("la columna vieja y su espejo ya no están", func(t *testing.T) {
		var columnas, triggers int
		_ = st.Pool.QueryRow(ctx, `select count(*) from information_schema.columns where table_name = 'ingredients' and column_name = 'waste_pct'`).Scan(&columnas)
		_ = st.Pool.QueryRow(ctx, `select count(*) from pg_trigger where tgname like 'ingredients_mirror_expected_waste%'`).Scan(&triggers)
		if columnas != 0 || triggers != 0 {
			t.Fatalf("quedaron %d columnas waste_pct y %d triggers espejo", columnas, triggers)
		}
	})

	t.Run("el tipo de movimiento existe", func(t *testing.T) {
		var ok bool
		if err := st.Pool.QueryRow(ctx, `select 'merma_esperada' = any(enum_range(null::stock_movement_type)::text[])`).Scan(&ok); err != nil || !ok {
			t.Fatalf("no existe merma_esperada en el enum (%v)", err)
		}
	})

	t.Run("bajar le regresa la columna con lo capturado", func(t *testing.T) {
		if err := goose.DownToContext(ctx, prepararGoose(t, st.Pool), ".", expectedWasteDeductsVersion-1); err != nil {
			t.Fatalf("el Down de la 0130 falló: %v", err)
		}
		var viejo float64
		if err := st.Pool.QueryRow(ctx, `select waste_pct from ingredients where id = $1`, conPct).Scan(&viejo); err != nil || viejo != 6.5 {
			t.Fatalf("waste_pct = %v (%v); quiere 6.5", viejo, err)
		}
	})
}
