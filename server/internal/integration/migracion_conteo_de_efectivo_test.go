//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// La migración 0066 (conteo de efectivo por denominaciones), probada ANTES de escribirla.
//
// POR QUÉ ESTA NO EXIGE UN RESPALDO RESTAURADO, a diferencia de la 0065. La constitución pide que
// una migración se pruebe contra un respaldo real y con al menos dos empresas, y la razón concreta
// es que con una sola empresa todo camino "por cada otra empresa" es un no-op: una migración que
// rellena datos existentes pasa verde en local y rompe en producción. **Ésta no rellena nada**:
// crea tres tablas vacías y siembra un catálogo global. No hay fila previa que tocar, así que no
// hay backfill que pueda ser un no-op.
//
// Lo que SÍ necesita dos empresas es el aislamiento, y ésas se crean aquí con `makeCompany`. Un
// test de RLS con una sola empresa pasa en verde con la policy borrada, que es la trampa que
// importa.
//
// Todo lo que prueba restricciones pasa por `exigeViolacionDeRestriccion`: sin él, un test contra
// una tabla que todavía no existe truena con 42P01 y se lee como "la restricción funcionó".

// denominacionMXN devuelve el id de una denominación de MXN por su valor. Se resuelve por consulta
// y no con un literal: los ids los asigna la siembra y atarlos aquí ataría el test al orden en que
// se escribió el `insert`.
func denominacionMXN(t *testing.T, st *store.Store, valor string) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(),
		`select id from cash_denominations where currency = 'MXN' and value = $1::numeric`,
		valor).Scan(&id); err != nil {
		t.Fatalf("buscar la denominación de %s: %v", valor, err)
	}
	return id
}

// turnoDe abre un turno en la caja principal de esa empresa, como owner (salta RLS).
func turnoDe(t *testing.T, st *store.Store, empresa int64) int64 {
	t.Helper()
	ctx := context.Background()
	var registerID int64
	if err := st.Pool.QueryRow(ctx,
		`insert into cash_registers (company_id, name, is_primary, is_active)
		 values ($1, 'Caja de prueba', false, true) returning id`, empresa).Scan(&registerID); err != nil {
		t.Fatalf("crear caja de la empresa %d: %v", empresa, err)
	}
	usuario := makeUserIn(t, st, empresa, "cajero_conteo_"+itoa(int(empresa)), "cajero")
	var sessionID int64
	if err := st.Pool.QueryRow(ctx,
		`insert into register_sessions (company_id, register_id, business_date, opening_cash, opened_by, status)
		 values ($1, $2, current_date, 0, $3, 'abierta') returning id`,
		empresa, registerID, usuario).Scan(&sessionID); err != nil {
		t.Fatalf("abrir turno de la empresa %d: %v", empresa, err)
	}
	return sessionID
}

// El catálogo: lo que se puede contar, y que nadie pueda sembrar una pieza que no vale dinero.
func TestElCatalogoDeDenominacionesEsUsableYNoAceptaBasura(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Las once de MXN que declara el spec: monedas de 0.50, 1, 2, 5 y 10; billetes de 20, 50, 100,
	// 200, 500 y 1000. Ni una más: una denominación de más es un botón que el operador tiene que
	// saltarse en cada conteo, y las de 10¢ y 20¢ ya no circulan.
	var activas int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from cash_denominations where currency = 'MXN' and is_active`).Scan(&activas); err != nil {
		t.Fatalf("contar denominaciones sembradas: %v", err)
	}
	if activas != 11 {
		t.Fatalf("la siembra dejó %d denominaciones activas de MXN, quiere 11", activas)
	}

	// De mayor a menor, que es como se cuenta un cajón: se empieza por los billetes grandes.
	filas, err := st.Pool.Query(ctx,
		`select value from cash_denominations where currency = 'MXN' and is_active order by sort_key`)
	if err != nil {
		t.Fatalf("leer el catálogo: %v", err)
	}
	defer filas.Close()
	anterior := decimal.NewFromInt(1 << 30)
	for filas.Next() {
		var v decimal.Decimal
		if err := filas.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if v.GreaterThanOrEqual(anterior) {
			t.Fatalf("el catálogo no viene de mayor a menor: %s salió después de %s", v, anterior)
		}
		anterior = v
	}

	// Una pieza que no vale dinero rompe en silencio toda suma que dependa de que cada renglón
	// aporta algo. El check es lo único que impide sembrarla.
	_, err = st.Pool.Exec(ctx,
		`insert into cash_denominations (currency, value, is_coin, sort_key, is_active)
		 values ('MXN', 0, true, 999, true)`)
	exigeViolacionDeRestriccion(t, err, "una denominación de valor 0, que sumaría nada y ocuparía un botón")

	_, err = st.Pool.Exec(ctx,
		`insert into cash_denominations (currency, value, is_coin, sort_key, is_active)
		 values ('MXN', -50, false, 998, true)`)
	exigeViolacionDeRestriccion(t, err, "una denominación negativa, que restaría del cajón")

	// Dos filas con el mismo valor serían dos botones para la misma pieza, y el operador contaría
	// dos veces sin darse cuenta de cuál usó.
	_, err = st.Pool.Exec(ctx,
		`insert into cash_denominations (currency, value, is_coin, sort_key, is_active)
		 values ('MXN', 500, false, 997, true)`)
	exigeViolacionDeRestriccion(t, err, "un billete de $500 duplicado en MXN")
}

// El catálogo es global y por eso el rol de la app no lo escribe.
//
// Sin `company_id` no hay nada que aisle una fila de otra: un endpoint futuro mal filtrado que
// apague el billete de $1000 para un negocio que no lo acepta lo apagaría para TODAS las empresas
// de la base. Mientras siga global, se cambia como operación deliberada de owner.
func TestElRolDeLaAppNoPuedeTocarElCatalogo(t *testing.T) {
	st := appRoleStore(t)
	ctx := context.Background()

	// Leer sí: la pantalla necesita saber qué piezas existen.
	var n int
	if err := st.Pool.QueryRow(ctx, `select count(*) from cash_denominations`).Scan(&n); err != nil {
		t.Fatalf("el rol de la app tiene que poder LEER el catálogo: %v", err)
	}
	if n == 0 {
		t.Fatal("el catálogo llegó vacío al rol de la app")
	}

	for _, caso := range []struct {
		que string
		sql string
	}{
		{"insertar una denominación", `insert into cash_denominations (currency, value, is_coin, sort_key, is_active)
		                               values ('MXN', 2000, false, 12, true)`},
		{"apagar una denominación para todas las empresas", `update cash_denominations set is_active = false where value = 1000`},
		{"borrar una denominación", `delete from cash_denominations where value = 1000`},
	} {
		if _, err := st.Pool.Exec(ctx, caso.sql); err == nil {
			t.Errorf("el rol de la app pudo %s: sin company_id eso afecta a TODAS las empresas", caso.que)
		}
	}
}

// El aislamiento entre empresas, que es lo único que de verdad necesita dos.
func TestUnConteoDeEfectivoNoSeVeDesdeOtraEmpresa(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	empresaB := makeCompany(t, st, "conteo-vecina")
	turnoA := turnoDe(t, st, defaultCompanyID)
	turnoB := turnoDe(t, st, empresaB)

	for _, caso := range []struct {
		empresa int64
		turno   int64
		total   string
	}{
		{defaultCompanyID, turnoA, "210"},
		{empresaB, turnoB, "999"},
	} {
		if _, err := st.Pool.Exec(ctx,
			`insert into session_cash_counts (company_id, session_id, moment, total, created_by)
			 values ($1, $2, 'apertura', $3::numeric,
			         (select id from users where company_id = $1 order by id limit 1))`,
			caso.empresa, caso.turno, caso.total); err != nil {
			t.Fatalf("sembrar conteo de la empresa %d: %v", caso.empresa, err)
		}
	}

	// SE LEE CON EL ROL DE LA APP, no con el owner que sembró. Para el owner las políticas
	// sencillamente no aplican, así que un test de aislamiento escrito sobre `newTestStore` cuenta
	// las dos filas y falla — o peor, pasa en verde si uno "arregla" el número esperado.
	//
	// Y va por `conexionDeEmpresa` y no por `st.Pool.Query` con un ctx de tenant: una consulta suelta
	// toma otra conexión del pool, con el default de la base, y entonces el test pasa en verde con la
	// policy borrada.
	conn := conexionDeEmpresa(t, appRoleStore(t), defaultCompanyID)
	var visibles int
	if err := conn.QueryRow(ctx, `select count(*) from session_cash_counts`).Scan(&visibles); err != nil {
		t.Fatalf("leer conteos como la empresa %d: %v", defaultCompanyID, err)
	}
	if visibles != 1 {
		t.Fatalf("la empresa %d ve %d conteos y solo uno es suyo: RLS no está aislando",
			defaultCompanyID, visibles)
	}
}

// Un conteo NO puede colgarse del turno de otra empresa.
//
// Los chequeos de integridad referencial de Postgres saltan RLS por diseño, así que una FK simple
// deja pasar un conteo con el company_id de A colgado del session_id de B: queda invisible para las
// dos y el turno ajeno aparece con piezas que nadie contó ahí. Es la razón textual de 0041 y 0061.
func TestUnConteoNoSeCuelgaDelTurnoDeOtraEmpresa(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	empresaB := makeCompany(t, st, "conteo-fk-vecina")
	turnoB := turnoDe(t, st, empresaB)
	// La empresa 1 necesita al menos un usuario: el insert de abajo lo busca para `created_by`, y
	// sin él el test falla por un not-null y no por la FK que viene a probar.
	makeUserIn(t, st, defaultCompanyID, "cajero_fk_conteo", "cajero")

	_, err := st.Pool.Exec(ctx,
		`insert into session_cash_counts (company_id, session_id, moment, total, created_by)
		 values ($1, $2, 'apertura', 100,
		         (select id from users where company_id = $1 order by id limit 1))`,
		defaultCompanyID, turnoB)
	exigeViolacionDeRestriccion(t, err,
		"un conteo de la empresa 1 colgado del turno de otra empresa")
}

// Un turno tiene un conteo de apertura y uno de cierre, no más.
//
// Es lo que impide que un segundo cierre pise el del primero — y con dos tabletas compartiendo
// cuenta eso no es hipotético. Aquí se prueba que la BASE lo rechaza; que el servicio lo traduzca a
// un mensaje y no a un 500 es T024b.
func TestUnTurnoNoPuedeTenerDosConteosDelMismoMomento(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	turno := turnoDe(t, st, defaultCompanyID)

	insertar := func() error {
		_, err := st.Pool.Exec(ctx,
			`insert into session_cash_counts (company_id, session_id, moment, total, expected, created_by)
			 values ($1, $2, 'cierre', 500, 500,
			         (select id from users where company_id = $1 order by id limit 1))`,
			defaultCompanyID, turno)
		return err
	}
	if err := insertar(); err != nil {
		t.Fatalf("el primer conteo de cierre debe entrar: %v", err)
	}
	exigeViolacionDeRestriccion(t, insertar(),
		"un segundo conteo de cierre para el mismo turno, que pisaría el del primero")
}

// Borrar una denominación NO se lleva por delante arqueos ya firmados.
//
// El catálogo se retira con `is_active`, no borrando. Pero si alguien borra una fila —limpiando una
// semilla mal cargada al agregar otra moneda— un `on delete cascade` copiado por inercia del
// renglón de arriba se llevaría en silencio piezas de un arqueo que ya se cerró. Dinero contado y
// declarado no desaparece por una limpieza de catálogo.
func TestBorrarUnaDenominacionUsadaNoSeLlevaElArqueo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	turno := turnoDe(t, st, defaultCompanyID)
	billete := denominacionMXN(t, st, "500")

	var conteo int64
	if err := st.Pool.QueryRow(ctx,
		`insert into session_cash_counts (company_id, session_id, moment, total, created_by)
		 values ($1, $2, 'apertura', 1000,
		         (select id from users where company_id = $1 order by id limit 1)) returning id`,
		defaultCompanyID, turno).Scan(&conteo); err != nil {
		t.Fatalf("crear conteo: %v", err)
	}
	if _, err := st.Pool.Exec(ctx,
		`insert into session_cash_count_lines (company_id, count_id, denomination_id, pieces)
		 values ($1, $2, $3, 2)`, defaultCompanyID, conteo, billete); err != nil {
		t.Fatalf("crear renglón de conteo: %v", err)
	}

	_, err := st.Pool.Exec(ctx, `delete from cash_denominations where id = $1`, billete)
	exigeViolacionDeRestriccion(t, err,
		"borrar un billete de $500 que un arqueo ya declaró, llevándose sus piezas")
}

// Cero piezas no genera renglón, y el esquema lo hace cumplir.
//
// FR-009 acepta el cero de entrada; lo que no existe es la fila. "No hay" y "no se capturó" son lo
// mismo en un arqueo, y guardar once ceros por conteo es ruido que hay que filtrar al leer.
func TestUnRenglonDeConteoConCeroPiezasNoSeGuarda(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	turno := turnoDe(t, st, defaultCompanyID)
	moneda := denominacionMXN(t, st, "10")

	var conteo int64
	if err := st.Pool.QueryRow(ctx,
		`insert into session_cash_counts (company_id, session_id, moment, total, created_by)
		 values ($1, $2, 'apertura', 0,
		         (select id from users where company_id = $1 order by id limit 1)) returning id`,
		defaultCompanyID, turno).Scan(&conteo); err != nil {
		t.Fatalf("crear conteo: %v", err)
	}

	_, err := st.Pool.Exec(ctx,
		`insert into session_cash_count_lines (company_id, count_id, denomination_id, pieces)
		 values ($1, $2, $3, 0)`, defaultCompanyID, conteo, moneda)
	exigeViolacionDeRestriccion(t, err, "un renglón de conteo con cero piezas")

	_, err = st.Pool.Exec(ctx,
		`insert into session_cash_count_lines (company_id, count_id, denomination_id, pieces)
		 values ($1, $2, $3, -3)`, defaultCompanyID, conteo, moneda)
	exigeViolacionDeRestriccion(t, err, "un renglón con piezas negativas, que restaría del cajón")
}

// El rol de la app escribe conteos pero no los edita ni los borra: un arqueo firmado no se toca.
func TestElRolDeLaAppNoEditaNiBorraUnConteo(t *testing.T) {
	owner := newTestStore(t)
	turno := turnoDe(t, owner, defaultCompanyID)
	ctx := context.Background()
	var conteo int64
	if err := owner.Pool.QueryRow(ctx,
		`insert into session_cash_counts (company_id, session_id, moment, total, created_by)
		 values ($1, $2, 'apertura', 210,
		         (select id from users where company_id = $1 order by id limit 1)) returning id`,
		defaultCompanyID, turno).Scan(&conteo); err != nil {
		t.Fatalf("crear conteo: %v", err)
	}

	app := appRoleStore(t)
	if _, err := app.Pool.Exec(ctx,
		`update session_cash_counts set total = 99999 where id = $1`, conteo); err == nil {
		t.Error("el rol de la app pudo REESCRIBIR el total de un arqueo")
	}
	if _, err := app.Pool.Exec(ctx,
		`delete from session_cash_counts where id = $1`, conteo); err == nil {
		t.Error("el rol de la app pudo BORRAR un arqueo")
	}
}

// El Down deja el esquema como estaba, y lo que se pierde está nombrado.
//
// Revertir NO pierde ninguna cifra de dinero: el total de cada arqueo vive en
// `register_sessions.opening_cash` y en `register_session_totals.declared`, columnas que esta
// migración nunca tocó. Lo que sí se pierde es el DESGLOSE — la capacidad de explicar un faltante
// viejo—, y eso no se recupera volviendo a aplicar la migración. Es un `Down` seguro para el
// servicio y caro para la auditoría, y esa distinción es la que hay que saber antes de correrlo.
func TestElDownDeLaMigracionDelConteoNoDejaBasura(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Hasta la 65, no "la última": el día que exista la 0067 este test seguiría revirtiendo una sola
	// migración y estaría comprobando el Down equivocado.
	t.Cleanup(func() { migrarArriba(t, st.Pool) })
	migrarAbajoHasta(t, st.Pool, 65)

	for _, tabla := range []string{"cash_denominations", "session_cash_counts", "session_cash_count_lines"} {
		var existe bool
		if err := st.Pool.QueryRow(ctx,
			`select exists (select 1 from information_schema.tables where table_name = $1)`,
			tabla).Scan(&existe); err != nil {
			t.Fatalf("consultar si %s existe: %v", tabla, err)
		}
		if existe {
			t.Errorf("el Down dejó %s en pie", tabla)
		}
	}
	var tipo bool
	if err := st.Pool.QueryRow(ctx,
		`select exists (select 1 from pg_type where typname = 'cash_count_moment')`).Scan(&tipo); err != nil {
		t.Fatalf("consultar el tipo: %v", err)
	}
	if tipo {
		t.Error("el Down dejó el enum cash_count_moment: un Up posterior chocaría con él")
	}

	// Y las columnas de dinero de siempre siguen ahí: revertir el desglose no toca los totales.
	var col bool
	if err := st.Pool.QueryRow(ctx,
		`select exists (select 1 from information_schema.columns
		                where table_name = 'register_sessions' and column_name = 'opening_cash')`).Scan(&col); err != nil {
		t.Fatalf("consultar opening_cash: %v", err)
	}
	if !col {
		t.Fatal("el Down se llevó register_sessions.opening_cash: el arqueo perdería su cifra")
	}

}

// EL SEGUNDO CONTEO DE UN TURNO TAMBIÉN GUARDA SUS RENGLONES.
//
// Regresión de un defecto real de 0066: la FK compuesta de los renglones listaba las columnas
// referenciadas al revés —`references session_cash_counts (id, company_id)` contra las locales
// `(company_id, count_id)`— y Postgres las empareja por POSICIÓN, no por nombre. Así escrita
// comparaba `company_id` contra `id`, así que solo pasaba cuando los dos números coincidían.
//
// Por qué ningún test lo vio: cada uno arranca con el esquema limpio, la primera fila queda en
// `id=1` y la empresa sembrada es la 1. La coincidencia es exactamente el caso que todos cubrían.
// En producción el primer arqueo con desglose habría funcionado y el segundo —el cierre de ese
// mismo turno— habría tronado con un 500 al guardar el primer renglón, con la caja sin poder
// cerrarse.
//
// Por eso el test cuenta DOS momentos del mismo turno: el segundo conteo tiene `id != company_id`,
// que es la única condición que hace visible el defecto.
func TestElSegundoConteoDeUnTurnoTambienGuardaSusRenglones(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	turno := turnoDe(t, st, defaultCompanyID)
	billete := denominacionMXN(t, st, "50")

	for _, momento := range []string{"apertura", "cierre"} {
		var conteo int64
		if err := st.Pool.QueryRow(ctx,
			`insert into session_cash_counts (company_id, session_id, moment, total, expected, created_by)
			 values ($1, $2, $3::cash_count_moment, 100, case when $3 = 'cierre' then 100 end,
			         (select id from users where company_id = $1 order by id limit 1)) returning id`,
			defaultCompanyID, turno, momento).Scan(&conteo); err != nil {
			t.Fatalf("crear el conteo de %s: %v", momento, err)
		}
		if _, err := st.Pool.Exec(ctx,
			`insert into session_cash_count_lines (company_id, count_id, denomination_id, pieces)
			 values ($1, $2, $3, 2)`, defaultCompanyID, conteo, billete); err != nil {
			t.Fatalf("el renglón del conteo de %s no entró (conteo id=%d, company_id=%d): %v\n"+
				"la FK compuesta de session_cash_count_lines empareja por posición: si el orden de "+
				"las columnas referenciadas no es el mismo que el de las locales, solo pasa mientras "+
				"id y company_id coincidan por casualidad",
				momento, conteo, defaultCompanyID, err)
		}
	}
}
