//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// La migración 0065 (folio de plataforma + liquidación), probada CONTRA UN RESPALDO REAL.
//
// No sobre una base sembrada: la constitución lo exige y la razón es concreta. Producción tiene dos
// empresas con historias distintas —61 y 62 pedidos, y los de plataforma viven solo en una—, y en
// una base limpia todo camino "por cada otra empresa" es un no-op. Una migración así pasa verde en
// local, verde en CI, y rompe en el VPS.
//
// Se prepara con `make respaldo-anonimo`. Sin TEST_RESTORED_DATABASE_URL estos tests se omiten.

// La SEGUNDA empresa del respaldo. `defaultCompanyID` (la 1, 'bobah-pruebas') es la que el harness
// sella por defecto; esta es la otra ('gatobobah', la que de verdad vende por plataformas). Se
// necesitan las dos para que un test de aislamiento pruebe algo: con una sola, pasa en verde sin
// haber comparado nada.
const otraEmpresa = 2

// nuevoPedidoDePlataforma inserta un pedido de plataforma como OWNER (salta RLS) en la empresa que
// se le pida. `register_session_id` va nulo a propósito: la unicidad del folio del turno es
// (company_id, register_session_id, daily_number) y dos NULL nunca chocan, así que el test no tiene
// que pelearse con los turnos que ya trae el respaldo.
func nuevoPedidoDePlataforma(t *testing.T, st *store.Store, empresa int64, plataforma int16, num int32) int64 {
	t.Helper()
	var id int64
	err := st.Pool.QueryRow(context.Background(), `
		insert into orders (client_uuid, business_date, daily_number, service_type,
		                    delivery_platform_id, opened_by, subtotal, total, status, company_id)
		values (gen_random_uuid(), date '2099-01-01', $1, 'domicilio', $2,
		        (select id from users where company_id = $3 order by id limit 1),
		        100, 100, 'entregada', $3)
		returning id`, num, plataforma, empresa).Scan(&id)
	if err != nil {
		t.Fatalf("sembrar pedido de plataforma en la empresa %d: %v", empresa, err)
	}
	t.Cleanup(func() {
		_, _ = st.Pool.Exec(context.Background(), `delete from orders where id = $1`, id)
	})
	return id
}

// plataformaDe devuelve una plataforma de reparto que pertenezca a esa empresa. Se resuelve por
// consulta y no con un id fijo porque los ids son globales y distintos por empresa (la 1 tiene
// 1-4, la 2 tiene 5-8): un literal aquí ataría el test a los datos de HOY.
func plataformaDe(t *testing.T, st *store.Store, empresa int64) int16 {
	t.Helper()
	var id int16
	if err := st.Pool.QueryRow(context.Background(),
		`select id from delivery_platforms where company_id = $1 and name <> 'Propio' order by id limit 1`,
		empresa).Scan(&id); err != nil {
		t.Fatalf("buscar plataforma de la empresa %d: %v", empresa, err)
	}
	return id
}

func TestLaMigracionDelFolioCorreSobreDatosRealesDeDosEmpresas(t *testing.T) {
	st := restoredStore(t)
	migrarArriba(t, st.Pool)
	ctx := context.Background()

	if v := versionDeEsquema(t, st); v < 65 {
		t.Fatalf("el esquema quedó en la versión %d y 0065 debería estar aplicada", v)
	}

	// Las tres columnas del folio existen y son nullable: el histórico no tiene folio y no se le
	// inventa uno.
	for _, col := range []string{"platform_order_ref", "platform_ref_set_by", "platform_ref_set_at"} {
		var nullable string
		err := st.Pool.QueryRow(ctx, `
			select is_nullable from information_schema.columns
			 where table_name = 'orders' and column_name = $1`, col).Scan(&nullable)
		if err != nil {
			t.Fatalf("orders.%s no existe tras migrar: %v", col, err)
		}
		if nullable != "YES" {
			t.Fatalf("orders.%s quedó NOT NULL: los %d pedidos que ya estaban no tienen folio y no se les inventa uno", col, 123)
		}
	}

	// Y ninguno de los pedidos que ya estaban se movió: una migración que "corre" no basta, lo que
	// importa es que no toque una fila.
	var conFolio int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from orders where platform_order_ref is not null`).Scan(&conFolio); err != nil {
		t.Fatalf("contar folios: %v", err)
	}
	if conFolio != 0 {
		t.Fatalf("la migración le puso folio a %d pedidos del histórico; debía dejarlos todos en null", conFolio)
	}
}

func TestElEsquemaRechazaUnFolioQueNoEsUnFolio(t *testing.T) {
	st := restoredStore(t)
	migrarArriba(t, st.Pool)
	ctx := context.Background()

	plataforma := plataformaDe(t, st, defaultCompanyID)
	pedido := nuevoPedidoDePlataforma(t, st, defaultCompanyID, plataforma, 9001)

	casos := []struct {
		nombre string
		folio  string
		porque string
	}{
		{"cadena vacía", "", "guardar \"\" haría que dos pedidos sin folio chocaran contra la unicidad, o peor, que pasaran"},
		{"solo espacios", "   ", "es la cadena vacía por otro camino"},
		{"con espacios en los extremos", " ABC123 ", "el recorte es de la frontera; al esquema debe llegar ya recortado"},
		{"más largo que el tope", strings.Repeat("x", 65), "64 es la cota de cordura contra un pegado accidental de media pantalla"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := st.Pool.Exec(ctx,
				`update orders set platform_order_ref = $1, platform_ref_set_by = opened_by,
				 platform_ref_set_at = now() where id = $2`, c.folio, pedido)
			if err == nil {
				_, _ = st.Pool.Exec(ctx, `update orders set platform_order_ref = null,
					platform_ref_set_by = null, platform_ref_set_at = null where id = $1`, pedido)
			}
			exigeViolacionDeRestriccion(t, err, c.nombre+": "+c.porque)
		})
	}
}

func TestElRastroDelFolioEsTodoONada(t *testing.T) {
	st := restoredStore(t)
	migrarArriba(t, st.Pool)
	ctx := context.Background()

	plataforma := plataformaDe(t, st, defaultCompanyID)
	pedido := nuevoPedidoDePlataforma(t, st, defaultCompanyID, plataforma, 9002)

	// Folio sin su rastro: un camino futuro que escriba solo la columna del folio deja el "quién lo
	// escribió" roto en silencio, y ese rastro existe para poder confiar en él meses después.
	_, err := st.Pool.Exec(ctx,
		`update orders set platform_order_ref = 'SIN-RASTRO-1' where id = $1`, pedido)
	exigeViolacionDeRestriccion(t, err, "folio sin platform_ref_set_by/_at: el rastro de quién lo "+
		"capturó queda roto y nadie se entera hasta que hace falta")

	// Y el rastro sin folio tampoco: sería una firma de algo que no está.
	_, err = st.Pool.Exec(ctx,
		`update orders set platform_ref_set_by = opened_by, platform_ref_set_at = now() where id = $1`, pedido)
	exigeViolacionDeRestriccion(t, err, "rastro sin folio: una firma sin nada que firmar")
}

func TestUnPedidoDeMostradorNoPuedeTenerFolioDePlataforma(t *testing.T) {
	st := restoredStore(t)
	migrarArriba(t, st.Pool)
	ctx := context.Background()

	var mostrador int64
	if err := st.Pool.QueryRow(ctx,
		`select id from orders where delivery_platform_id is null order by id limit 1`).Scan(&mostrador); err != nil {
		t.Fatalf("buscar un pedido de mostrador en el respaldo: %v", err)
	}
	_, err := st.Pool.Exec(ctx, `update orders set platform_order_ref = 'UBER-123',
		platform_ref_set_by = opened_by, platform_ref_set_at = now() where id = $1`, mostrador)
	exigeViolacionDeRestriccion(t, err, "un pedido de mostrador aceptó folio de plataforma. Tiene "+
		"que ser imposible por construcción y no por una validación de pantalla que un camino nuevo se salta")
}

func TestLaUnicidadDelFolioEsPorEmpresaYPorPlataforma(t *testing.T) {
	st := restoredStore(t)
	migrarArriba(t, st.Pool)
	ctx := context.Background()

	const folio = "4B2E9A10-77C3-4F1E-9E62-0A5C1D3F8B44"
	pone := func(pedido int64) error {
		_, err := st.Pool.Exec(ctx, `update orders set platform_order_ref = $1,
			platform_ref_set_by = opened_by, platform_ref_set_at = now() where id = $2`, folio, pedido)
		return err
	}

	uberA := plataformaDe(t, st, defaultCompanyID)
	unoA := nuevoPedidoDePlataforma(t, st, defaultCompanyID, uberA, 9010)
	if err := pone(unoA); err != nil {
		t.Fatalf("el primer folio no entró: %v", err)
	}

	// Mismo folio, misma empresa, MISMA plataforma → se rechaza. Es el dedazo de todos los días.
	otroA := nuevoPedidoDePlataforma(t, st, defaultCompanyID, uberA, 9011)
	exigeViolacionDeRestriccion(t, pone(otroA), "dos pedidos de la misma empresa y la misma "+
		"plataforma comparten folio: la conciliación deja de poder decir cuál pedido formó el depósito")

	// Mismo folio, misma empresa, OTRA plataforma → se acepta. Rechazarlo tiraría capturas
	// legítimas, y eso es lo caro de quitar después.
	var otraPlataforma int16
	if err := st.Pool.QueryRow(ctx,
		`select id from delivery_platforms where company_id = $1 and name <> 'Propio' and id <> $2
		 order by id limit 1`, defaultCompanyID, uberA).Scan(&otraPlataforma); err != nil {
		t.Fatalf("buscar una segunda plataforma de la empresa %d: %v", defaultCompanyID, err)
	}
	enOtraPlataforma := nuevoPedidoDePlataforma(t, st, defaultCompanyID, otraPlataforma, 9012)
	if err := pone(enOtraPlataforma); err != nil {
		t.Fatalf("el mismo número en OTRA plataforma se rechazó, y es una captura legítima: %v", err)
	}

	// Mismo folio, OTRA empresa → se acepta. La unicidad nunca es global.
	plataformaB := plataformaDe(t, st, otraEmpresa)
	enOtraEmpresa := nuevoPedidoDePlataforma(t, st, otraEmpresa, plataformaB, 9013)
	if err := pone(enOtraEmpresa); err != nil {
		t.Fatalf("dos empresas distintas no pudieron usar el mismo folio: %v", err)
	}
}

func TestLaLiquidacionEsUnaPorPedidoYSeReemplaza(t *testing.T) {
	st := restoredStore(t)
	migrarArriba(t, st.Pool)
	ctx := context.Background()

	plataforma := plataformaDe(t, st, defaultCompanyID)
	pedido := nuevoPedidoDePlataforma(t, st, defaultCompanyID, plataforma, 9020)
	t.Cleanup(func() {
		_, _ = st.Pool.Exec(ctx, `delete from platform_settlements where order_id = $1`, pedido)
	})

	inserta := func(bruto, comision, neto string) error {
		_, err := st.Pool.Exec(ctx, `
			insert into platform_settlements
			  (order_id, reported_gross, commission_amount, net_amount, captured_by, company_id)
			values ($1, $2::numeric, $3::numeric, $4::numeric,
			        (select opened_by from orders where id = $1), $5)
			on conflict (order_id) do update set
			  reported_gross = excluded.reported_gross,
			  commission_amount = excluded.commission_amount,
			  net_amount = excluded.net_amount`,
			pedido, bruto, comision, neto, defaultCompanyID)
		return err
	}

	if err := inserta("220.00", "33.00", "51.77"); err != nil {
		t.Fatalf("registrar la liquidación: %v", err)
	}
	// Documento corregido: reemplaza, no duplica.
	if err := inserta("220.00", "35.00", "49.77"); err != nil {
		t.Fatalf("recapturar desde un documento corregido: %v", err)
	}
	var n int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from platform_settlements where order_id = $1`, pedido).Scan(&n); err != nil {
		t.Fatalf("contar liquidaciones: %v", err)
	}
	if n != 1 {
		t.Fatalf("el pedido quedó con %d liquidaciones: la recaptura duplicó en vez de reemplazar, y "+
			"la comisión se contaría dos veces", n)
	}

	// El neto NEGATIVO es real: promoción que financió el restaurante por completo.
	if err := inserta("220.00", "33.00", "-31.20"); err != nil {
		t.Fatalf("el esquema rechazó un neto negativo, que es lo que de verdad pasa con una "+
			"promoción financiada por el restaurante: %v", err)
	}
}

func TestUnaLiquidacionNoPuedeColgarDelPedidoDeOtraEmpresa(t *testing.T) {
	st := restoredStore(t)
	migrarArriba(t, st.Pool)
	ctx := context.Background()

	plataforma := plataformaDe(t, st, defaultCompanyID)
	pedido := nuevoPedidoDePlataforma(t, st, defaultCompanyID, plataforma, 9030)

	// company_id de OTRA empresa apuntando a un pedido de la primera. Los chequeos de integridad
	// referencial de Postgres saltan RLS por diseño, así que una FK simple lo aceptaría y el error
	// aparecería en el resumen de dinero, no en el insert.
	_, err := st.Pool.Exec(ctx, `
		insert into platform_settlements
		  (order_id, reported_gross, commission_amount, net_amount, captured_by, company_id)
		values ($1, 100, 30, 70, (select opened_by from orders where id = $1), $2)`,
		pedido, otraEmpresa)
	if err == nil {
		_, _ = st.Pool.Exec(ctx, `delete from platform_settlements where order_id = $1`, pedido)
	}
	exigeViolacionDeRestriccion(t, err, "se pudo colgar una liquidación de la empresa B del pedido "+
		"de la empresa A: la FK tiene que ser compuesta con company_id, porque las FK simples saltan RLS")
}

func TestElRolDeAppPuedeUsarElFolioYLaLiquidacion(t *testing.T) {
	owner := restoredStore(t)
	migrarArriba(t, owner.Pool)

	plataforma := plataformaDe(t, owner, defaultCompanyID)
	pedido := nuevoPedidoDePlataforma(t, owner, defaultCompanyID, plataforma, 9040)

	st := restoredAppRoleStore(t)
	ctx := context.Background()
	propia := conexionDeEmpresa(t, st, defaultCompanyID)
	t.Cleanup(func() {
		_, _ = owner.Pool.Exec(ctx, `delete from platform_settlements where order_id = $1`, pedido)
	})

	// El grant de 0024 fue puntual (`on all tables in schema public`, sin default privileges), así
	// que cada tabla nueva necesita el suyo. Olvidarlo NO se nota en dev —la API se conecta como
	// owner— sino en producción, con un 42501 en la primera captura.
	if _, err := propia.Exec(ctx, `
		insert into platform_settlements
		  (order_id, reported_gross, commission_amount, net_amount, captured_by)
		values ($1, 220, 33, 51.77, (select opened_by from orders where id = $1))`, pedido); err != nil {
		t.Fatalf("el rol de la app no puede INSERTAR en platform_settlements: %v — falta su grant, y "+
			"en producción eso es un 42501 al capturar la primera liquidación", err)
	}
	var n int
	if err := propia.QueryRow(ctx,
		`select count(*) from platform_settlements where order_id = $1`, pedido).Scan(&n); err != nil {
		t.Fatalf("el rol de la app no puede LEER platform_settlements: %v — falta su grant", err)
	}
	// Escribir el folio también pasa por el rol de app: es lo que hace el cajero.
	if _, err := propia.Exec(ctx, `
		update orders set platform_order_ref = 'APP-ROLE-1', platform_ref_set_by = opened_by,
		 platform_ref_set_at = now() where id = $1`, pedido); err != nil {
		t.Fatalf("el rol de la app no puede escribir el folio: %v", err)
	}
}

func TestLaLiquidacionDeUnaEmpresaNoSeVeDesdeLaOtra(t *testing.T) {
	owner := restoredStore(t)
	migrarArriba(t, owner.Pool)
	ctx := context.Background()

	plataforma := plataformaDe(t, owner, defaultCompanyID)
	pedido := nuevoPedidoDePlataforma(t, owner, defaultCompanyID, plataforma, 9050)
	if _, err := owner.Pool.Exec(ctx, `
		insert into platform_settlements
		  (order_id, reported_gross, commission_amount, net_amount, captured_by, company_id)
		values ($1, 220, 33, 51.77, (select opened_by from orders where id = $1), $2)`,
		pedido, defaultCompanyID); err != nil {
		t.Fatalf("sembrar la liquidación: %v", err)
	}
	t.Cleanup(func() {
		_, _ = owner.Pool.Exec(ctx, `delete from platform_settlements where order_id = $1`, pedido)
	})

	st := restoredAppRoleStore(t)
	ajena := conexionDeEmpresa(t, st, otraEmpresa)

	var n int
	if err := ajena.QueryRow(ctx,
		`select count(*) from platform_settlements where order_id = $1`, pedido).Scan(&n); err != nil {
		t.Fatalf("consultar como la otra empresa: %v", err)
	}
	if n != 0 {
		t.Fatalf("la empresa %d ve la liquidación de la empresa %d: falta la policy de RLS, y con un "+
			"segundo cliente eso es una fuga de cuánto vende el otro", otraEmpresa, defaultCompanyID)
	}
}

func TestElDownDeLaMigracionDejaElEsquemaComoEstaba(t *testing.T) {
	st := restoredStore(t)
	migrarArriba(t, st.Pool)
	ctx := context.Background()

	antes := versionDeEsquema(t, st)
	if antes != 65 {
		t.Fatalf("la base está en la versión %d y este test revierte LA ÚLTIMA: correrlo así "+
			"desharía la migración equivocada", antes)
	}
	// Se vuelve a subir pase lo que pase: los demás tests de este paquete dan por hecho que la base
	// quedó migrada, y dejarla abajo los rompería según el orden en que corran.
	t.Cleanup(func() { migrarArriba(t, st.Pool) })

	migrarAbajo(t, st.Pool)

	for _, col := range []string{"platform_order_ref", "platform_ref_set_by", "platform_ref_set_at"} {
		var n int
		if err := st.Pool.QueryRow(ctx, `
			select count(*) from information_schema.columns
			 where table_name = 'orders' and column_name = $1`, col).Scan(&n); err != nil {
			t.Fatalf("consultar columnas tras el down: %v", err)
		}
		if n != 0 {
			t.Fatalf("el Down dejó orders.%s puesta: revertir a medias es peor que no revertir", col)
		}
	}
	var tablas int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from information_schema.tables where table_name = 'platform_settlements'`).Scan(&tablas); err != nil {
		t.Fatalf("consultar tablas tras el down: %v", err)
	}
	if tablas != 0 {
		t.Fatal("el Down dejó la tabla platform_settlements")
	}
	// El unique que la FK compuesta necesitaba también se va: si se queda, un Down/Up repetido
	// truena con \"ya existe\".
	var uniques int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from pg_constraint where conname = 'orders_id_company_key'`).Scan(&uniques); err != nil {
		t.Fatalf("consultar constraints tras el down: %v", err)
	}
	if uniques != 0 {
		t.Fatal("el Down dejó orders_id_company_key: el siguiente Up truena con \"ya existe\"")
	}

	if v := versionDeEsquema(t, st); v >= antes {
		t.Fatalf("el Down no movió la versión del esquema (sigue en %d)", v)
	}
}
