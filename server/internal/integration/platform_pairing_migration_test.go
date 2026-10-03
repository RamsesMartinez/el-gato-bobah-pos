//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LA MIGRACIÓN DEL EMPAREJAMIENTO (0077) SOBRE UN RESPALDO REAL CON DOS EMPRESAS.

// testStoreIn da de alta una tienda de prueba en la primera empresa del respaldo y devuelve un
// producto suyo. Los respaldos todavía no traen tiendas conectadas, y sin una las pruebas de las
// parejas se omitirían en silencio.
func testStoreIn(t *testing.T, ctx context.Context, st *store.Store) (company, product, connection int64) {
	t.Helper()
	if err := st.Pool.QueryRow(ctx, `
		insert into platform_connections (company_id, delivery_platform_id, external_store_id, label)
		select dp.company_id, dp.id, 'tienda-prueba-0077', 'Prueba 0077'
		  from delivery_platforms dp order by dp.company_id, dp.id limit 1
		returning company_id, id`).Scan(&company, &connection); err != nil {
		t.Fatalf("crear la tienda de prueba: %v", err)
	}
	t.Cleanup(func() {
		_, _ = st.Pool.Exec(context.Background(), `delete from platform_connections where id = $1`, connection)
	})
	if err := st.Pool.QueryRow(ctx, `select id from products where company_id = $1 order by id limit 1`, company).Scan(&product); err != nil {
		t.Fatalf("un producto de la empresa %d: %v", company, err)
	}
	return company, product, connection
}
func TestPlatformPairingMigrationOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start >= 77 {
		migrarAbajoHasta(t, st.Pool, 76)
	} else if start < 76 {
		migrarArriba(t, st.Pool) // hasta la última; abajo se regresa a 76
		migrarAbajoHasta(t, st.Pool, 76)
	}
	t.Cleanup(func() {
		if v := versionDeEsquema(t, st); v != start && start < 77 {
			migrarAbajoHasta(t, st.Pool, start)
		}
	})

	var manualBefore int
	if err := st.Pool.QueryRow(ctx,
		`select (select count(*) from product_platform_prices) + (select count(*) from modifier_option_platform_prices)`).Scan(&manualBefore); err != nil {
		t.Fatal(err)
	}

	migrarArriba(t, st.Pool)

	var companies, generics, manualAfter int
	if err := st.Pool.QueryRow(ctx, `
		select (select count(*) from companies),
		       (select count(*) from products where system_kind = 'platform_unpaired' and not is_active and needs_prep),
		       (select count(*) from product_platform_prices where source = 'manual')
		     + (select count(*) from modifier_option_platform_prices where source = 'manual')`).Scan(&companies, &generics, &manualAfter); err != nil {
		t.Fatal(err)
	}
	if generics != companies {
		t.Fatalf("cada empresa debe tener un producto genérico inactivo y que sale en comanda: %d de %d", generics, companies)
	}
	if manualAfter != manualBefore {
		t.Fatalf("los precios capturados a mano deben quedar como manuales: %d antes, %d después", manualBefore, manualAfter)
	}

	t.Run("una opción ligada a un producto se rechaza", func(t *testing.T) {
		company, product, connection := testStoreIn(t, ctx, st)
		_, err := st.Pool.Exec(ctx, `
			insert into platform_item_links (company_id, connection_id, external_id, kind, local_kind, product_id)
			values ($1, $2, 'opcion-mal-ligada', 'opcion', 'producto', $3)`, company, connection, product)
		exigeViolacionDeRestriccion(t, err, "una opción de la plataforma ligada a un producto")
	})

	t.Run("la guarda aborta con parejas de opción guardadas como producto", func(t *testing.T) {
		migrarAbajoHasta(t, st.Pool, 76)
		company, product, connection := testStoreIn(t, ctx, st)
		if _, err := st.Pool.Exec(ctx, `
			insert into platform_item_links (company_id, connection_id, external_id, kind, local_kind, product_id)
			values ($1, $2, 'opcion-vieja', 'opcion', 'opcion_de_modificador', $3)`, company, connection, product); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = st.Pool.Exec(ctx, `delete from platform_item_links where external_id = 'opcion-vieja'`)
		})
		db := prepararGoose(t, st.Pool)
		err := goose.UpContext(ctx, db, ".")
		if err == nil || !strings.Contains(err.Error(), "guardadas como producto") {
			t.Fatalf("la 0077 debe abortar con una pareja de opción guardada como producto, fue: %v", err)
		}
	})
}
