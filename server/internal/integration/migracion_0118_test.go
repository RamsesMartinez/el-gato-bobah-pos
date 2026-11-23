//go:build integration

package integration

import (
	"context"
	"testing"
)

// salesChannelPriceVersion es la migración de los lugares de venta con precio (spec 092).
// PROVISIONAL: se mueve al siguiente número libre de develop al fusionar, y esta constante con ella.
const salesChannelPriceVersion = 118

// LA 0118 SOBRE UN RESPALDO REAL CON DOS EMPRESAS (constitución IV).
//
// Lo que solo se ve con datos reales: que ningún producto amanece agotado ni «editado hoy», que toda
// empresa —también una sin catálogo de lugares de venta— queda con Mostrador y Web, y que un canal de
// otra empresa ya no se puede ligar a un producto ni como owner.
func TestMigrationSalesChannelPriceOnARealBackup(t *testing.T) {
	st := restoredStore(t)
	ctx := context.Background()

	start := versionDeEsquema(t, st)
	if start < salesChannelPriceVersion-1 {
		migrarArriba(t, st.Pool)
	}
	migrarAbajoHasta(t, st.Pool, salesChannelPriceVersion-1)
	t.Cleanup(func() {
		if versionDeEsquema(t, st) < start {
			migrarArriba(t, st.Pool)
		}
	})

	// Una empresa sin catálogo de lugares de venta, como las que crea hoy la consola, y productos con
	// «vender sin existencia» apagado como los dejó la importación de FUDO.
	var bare int64
	if err := st.Pool.QueryRow(ctx, `insert into companies (slug, name) values ('mig-0118-sin-canales', 'Sin canales') returning id`).Scan(&bare); err != nil {
		t.Fatal(err)
	}
	var falseBefore int
	if err := st.Pool.QueryRow(ctx, `select count(*) from products where not allow_oversell`).Scan(&falseBefore); err != nil {
		t.Fatal(err)
	}
	if falseBefore == 0 {
		if _, err := st.Pool.Exec(ctx, `update products set allow_oversell = false where id in (select id from products order by id limit 3)`); err != nil {
			t.Fatal(err)
		}
	}
	type stamp struct {
		id      int64
		updated string
	}
	var flipped []stamp
	rows, err := st.Pool.Query(ctx, `select id, updated_at::text from products where not allow_oversell order by id limit 20`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s stamp
		if err := rows.Scan(&s.id, &s.updated); err != nil {
			t.Fatal(err)
		}
		flipped = append(flipped, s)
	}
	rows.Close()

	migrarArriba(t, st.Pool)

	t.Run("ningún producto queda con vender sin existencia apagado, y ninguno se marca editado", func(t *testing.T) {
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from products where not allow_oversell`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%d productos siguen con allow_oversell=false: amanecerían agotados con un valor de FUDO que nadie aplicó", n)
		}
		for _, s := range flipped {
			var now string
			if err := st.Pool.QueryRow(ctx, `select updated_at::text from products where id = $1`, s.id).Scan(&now); err != nil {
				t.Fatal(err)
			}
			if now != s.updated {
				t.Fatalf("el producto %d cambió de updated_at (%s → %s): el catálogo diría que se editó hoy", s.id, s.updated, now)
			}
		}
	})

	t.Run("toda empresa tiene Mostrador, Menú QR y Web", func(t *testing.T) {
		var missing int
		if err := st.Pool.QueryRow(ctx, `
			select count(*) from companies c cross join (values ('pos','Mostrador'),('qr','Menú QR'),('online','Web')) v(code, name)
			 where not exists (select 1 from channels ch where ch.company_id = c.id and ch.code = v.code and ch.name = v.name)`).Scan(&missing); err != nil {
			t.Fatal(err)
		}
		if missing != 0 {
			t.Fatalf("faltan %d lugares de venta (o con el nombre viejo): apagar «Web» en esa empresa no tendría a qué apuntar", missing)
		}
		var bareN int
		if err := st.Pool.QueryRow(ctx, `select count(*) from channels where company_id = $1`, bare).Scan(&bareN); err != nil {
			t.Fatal(err)
		}
		if bareN != 3 {
			t.Fatalf("la empresa sin catálogo quedó con %d lugares de venta, se esperaban 3", bareN)
		}
	})

	t.Run("los ids de lugar de venta son bigint", func(t *testing.T) {
		rows, err := st.Pool.Query(ctx, `
			select table_name || '.' || column_name, data_type from information_schema.columns
			 where (table_name, column_name) in (('channels','id'),('product_channels','channel_id'),('category_channels','channel_id'))`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		seen := 0
		for rows.Next() {
			var col, typ string
			if err := rows.Scan(&col, &typ); err != nil {
				t.Fatal(err)
			}
			seen++
			if typ != "bigint" {
				t.Fatalf("%s es %s: con un id global por empresa y lugar de venta, smallint topa en miles de empresas", col, typ)
			}
		}
		if seen != 3 {
			t.Fatalf("se esperaban 3 columnas, se vieron %d", seen)
		}
	})

	var prodA, coA, chanOther int64
	if err := st.Pool.QueryRow(ctx, `
		select p.id, p.company_id, (select ch.id from channels ch where ch.company_id <> p.company_id and ch.code = 'online' limit 1)
		  from products p order by p.id limit 1`).Scan(&prodA, &coA, &chanOther); err != nil {
		t.Fatal(err)
	}

	t.Run("el precio por lugar de venta acepta null y rechaza cero o negativo", func(t *testing.T) {
		var web int64
		if err := st.Pool.QueryRow(ctx, `select id from channels where company_id = $1 and code = 'online'`, coA).Scan(&web); err != nil {
			t.Fatal(err)
		}
		if _, err := st.Pool.Exec(ctx, `insert into product_channels (company_id, product_id, channel_id, visibility, price) values ($1, $2, $3, 'visible', null)`, coA, prodA, web); err != nil {
			t.Fatalf("un precio web vacío (hereda) se rechazó: %v", err)
		}
		for _, bad := range []string{"0", "-1"} {
			if _, err := st.Pool.Exec(ctx, `update product_channels set price = $3::numeric where product_id = $1 and channel_id = $2`, prodA, web, bad); err == nil {
				t.Fatalf("un precio web de %s se aceptó", bad)
			}
		}
		if _, err := st.Pool.Exec(ctx, `delete from product_channels where product_id = $1`, prodA); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("un canal de otra empresa no se liga a un producto, ni como owner", func(t *testing.T) {
		if _, err := st.Pool.Exec(ctx, `insert into product_channels (company_id, product_id, channel_id, visibility) values ($1, $2, $3, 'oculto')`, coA, prodA, chanOther); err == nil {
			t.Fatal("el producto de una empresa quedó ligado al lugar de venta de otra: los chequeos de integridad saltan RLS")
		}
	})

	t.Run("el Down quita el precio y vuelve a subir", func(t *testing.T) {
		migrarAbajoHasta(t, st.Pool, salesChannelPriceVersion-1)
		var n int
		if err := st.Pool.QueryRow(ctx, `select count(*) from information_schema.columns where table_name = 'product_channels' and column_name = 'price'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatal("el Down dejó la columna price")
		}
		migrarArriba(t, st.Pool)
	})
}
