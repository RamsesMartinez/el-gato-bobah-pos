//go:build integration

package integration

import (
	"context"
	"testing"
)

// LA MIGRACIÓN DEL ALMACÉN (0078) SOBRE UN RESPALDO REAL CON DOS EMPRESAS.
func TestStockCompositionMigrationOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start >= 78 {
		migrarAbajoHasta(t, st.Pool, 77)
	} else if start < 77 {
		migrarArriba(t, st.Pool)
		migrarAbajoHasta(t, st.Pool, 77)
	}
	t.Cleanup(func() {
		if v := versionDeEsquema(t, st); v != start && start < 78 {
			migrarAbajoHasta(t, st.Pool, start)
		}
	})

	type counts struct{ confirmedProducts, estimatedProducts, estimatedOptions, estimatedPreps int }
	var want counts
	if err := st.Pool.QueryRow(ctx, `
		select (select count(*) from products where track_stock or type = 'combo'),
		       (select count(*) from products where not track_stock and type <> 'combo' and recipe_id is not null),
		       (select count(*) from modifier_options where recipe_id is not null or linked_product_id is not null),
		       (select count(*) from ingredients where is_prep)`).
		Scan(&want.confirmedProducts, &want.estimatedProducts, &want.estimatedOptions, &want.estimatedPreps); err != nil {
		t.Fatal(err)
	}

	migrarArriba(t, st.Pool)

	var got counts
	var confirmedByPerson int
	if err := st.Pool.QueryRow(ctx, `
		select (select count(*) from products where composition_status = 'confirmed'),
		       (select count(*) from products where composition_status = 'estimated'),
		       (select count(*) from modifier_options where composition_status = 'estimated'),
		       (select count(*) from ingredients where composition_status = 'estimated'),
		       (select count(*) from products where composition_confirmed_by is not null)`).
		Scan(&got.confirmedProducts, &got.estimatedProducts, &got.estimatedOptions, &got.estimatedPreps, &confirmedByPerson); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("el relleno del estado no corresponde a lo que había: quería %+v, quedó %+v", want, got)
	}
	if confirmedByPerson != 0 {
		t.Fatalf("lo que confirmó la migración no lleva confirmador, y hay %d con uno", confirmedByPerson)
	}

	t.Run("un componente no se cuelga de un renglón de otra empresa", func(t *testing.T) {
		var line, lineCompany, otherCompany, product int64
		if err := st.Pool.QueryRow(ctx, `
			select l.id, l.company_id, c.id
			  from order_lines l join companies c on c.id <> l.company_id
			 order by l.id limit 1`).Scan(&line, &lineCompany, &otherCompany); err != nil {
			t.Skipf("el respaldo necesita renglones y dos empresas: %v", err)
		}
		if err := st.Pool.QueryRow(ctx, `select id from products where company_id = $1 limit 1`, otherCompany).Scan(&product); err != nil {
			t.Fatal(err)
		}
		_, err := st.Pool.Exec(ctx, `
			insert into order_line_components (order_line_id, product_id, quantity, company_id)
			values ($1, $2, 1, $3)`, line, product, otherCompany)
		exigeViolacionDeRestriccion(t, err, "un componente de la empresa B colgado de un renglón de la empresa A")
	})

	t.Run("confirmada sin fecha se rechaza", func(t *testing.T) {
		_, err := st.Pool.Exec(ctx, `
			update products set composition_status = 'confirmed', composition_confirmed_at = null
			 where id = (select id from products order by id limit 1)`)
		exigeViolacionDeRestriccion(t, err, "una composición confirmada sin cuándo")
	})

	t.Run("baja y vuelve a subir", func(t *testing.T) {
		migrarAbajoHasta(t, st.Pool, 77)
		var n int
		if err := st.Pool.QueryRow(ctx, `
			select count(*) from information_schema.columns
			 where column_name in ('composition_status', 'component_of_product_id')
			    or (table_name = 'platform_incoming_order_lines' and column_name = 'modifier_option_id')`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("la bajada dejó %d columnas de la 0078", n)
		}
		migrarArriba(t, st.Pool)
	})
}
