//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
)

const expectedWasteVersion = 126

// LA 0126 (merma esperada, spec 096) CONTRA UN RESPALDO REAL CON DOS EMPRESAS. Solo agrega tres
// columnas nullable y copia el % que hoy tienen los artículos. Lo que importa es que ningún costo
// cambie al subir (SC-001): el motor nuevo usa `coalesce(expected_waste_pct, 0)` por artículo donde el
// viejo usaba `waste_pct`, con el general y el del producto vacíos, así que basta con que esas dos
// cifras coincidan en cada artículo. Y que el Down le regrese a la API anterior lo que se capturó
// después de subir (R14 H3): sin eso, al volver a la versión anterior su primer recálculo cambiaría
// costos con el % viejo.
func TestMigration0126OnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()
	migrarAbajoHasta(t, st.Pool, expectedWasteVersion-1)
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	var companies int
	if err := st.Pool.QueryRow(ctx, `select count(distinct company_id) from ingredients`).Scan(&companies); err != nil {
		t.Fatal(err)
	}
	if companies < 2 {
		t.Fatalf("el respaldo trae artículos de %d empresas; la prueba necesita dos", companies)
	}

	if err := goose.UpToContext(ctx, prepararGoose(t, st.Pool), ".", expectedWasteVersion); err != nil {
		t.Fatalf("la 0126 no subió sobre el respaldo (la API nueva no arrancaría): %v", err)
	}

	t.Run("cada artículo conserva su % y los que estaban en 0 quedan sin captura", func(t *testing.T) {
		var distintos, ceroCapturado int
		if err := st.Pool.QueryRow(ctx, `
			select count(*) filter (where coalesce(expected_waste_pct, 0) <> waste_pct),
			       count(*) filter (where expected_waste_pct = 0)
			  from ingredients`).Scan(&distintos, &ceroCapturado); err != nil {
			t.Fatal(err)
		}
		if distintos != 0 {
			t.Fatalf("%d artículos cambiaron de %% al subir: su costo cambiaría sin que nadie lo pidiera", distintos)
		}
		if ceroCapturado != 0 {
			t.Fatalf("%d artículos quedaron con 0 %% capturado; un 0 de antes significa «nadie lo capturó»", ceroCapturado)
		}
	})

	t.Run("productos y general nacen vacíos", func(t *testing.T) {
		var productos, generales int
		if err := st.Pool.QueryRow(ctx, `select (select count(*) from products where expected_waste_pct is not null),
		       (select count(*) from business_settings where expected_waste_pct is not null)`).Scan(&productos, &generales); err != nil {
			t.Fatal(err)
		}
		if productos != 0 || generales != 0 {
			t.Fatalf("productos con %% = %d, empresas con general = %d: los costos subirían al subir", productos, generales)
		}
	})

	t.Run("el check rechaza un 100 %", func(t *testing.T) {
		if _, err := st.Pool.Exec(ctx, `update business_settings set expected_waste_pct = 100`); err == nil {
			t.Fatal("un general de 100 % entró: el costo se duplicaría")
		}
	})

	t.Run("el Down le regresa a la API anterior lo capturado después de subir", func(t *testing.T) {
		var id int64
		if err := st.Pool.QueryRow(ctx, `select id from ingredients order by id limit 1`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Pool.Exec(ctx, `update ingredients set expected_waste_pct = 8 where id = $1`, id); err != nil {
			t.Fatal(err)
		}
		if err := goose.DownToContext(ctx, prepararGoose(t, st.Pool), ".", expectedWasteVersion-1); err != nil {
			t.Fatalf("el Down de la 0126 falló: %v", err)
		}
		var viejo float64
		if err := st.Pool.QueryRow(ctx, `select waste_pct from ingredients where id = $1`, id).Scan(&viejo); err != nil {
			t.Fatal(err)
		}
		if viejo != 8 {
			t.Fatalf("waste_pct = %v tras el Down; la API anterior recalcularía con el %% de antes de subir", viejo)
		}
	})
}
