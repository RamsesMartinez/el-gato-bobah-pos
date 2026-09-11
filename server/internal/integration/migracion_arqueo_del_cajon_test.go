//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
)

// LA MIGRACIÓN DEL ARQUEO DEL CAJÓN (spec 015, migración 0067).
//
// Dos piezas irreversibles y tres guardarraíles. Las irreversibles son hechos que, si no se guardan
// en el momento del cierre, no se pueden reconstruir después: contra qué se comparó el conteo, y si
// ese método tocaba el cajón. Los guardarraíles son checks que impiden que un bug futuro se disfrace
// de dato viejo.
//
// Corre con DOS empresas: con una sola, todo camino "por cada otra empresa" es un no-op y la
// migración pasa verde para romper en producción.

// LO QUE EL CONTEO GUARDA AHORA: contra qué se comparó.
//
// El esperado sale de `order_payments`, y una venta cancelada o reembolsada después del cierre lo
// mueve. Un arqueo firmado tiene que poder reconstruirse tal como se firmó, así que el esperado es
// un hecho del momento —como `order_lines.unit_price`— y no una consulta que se rehace.
func TestElConteoGuardaContraQueSeComparo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	turno := turnoDe(t, st, defaultCompanyID)

	var conteo int64
	if err := st.Pool.QueryRow(ctx,
		`insert into session_cash_counts (company_id, session_id, moment, total, expected, created_by)
		 values ($1, $2, 'cierre', 340, 390,
		         (select id from users where company_id = $1 order by id limit 1)) returning id`,
		defaultCompanyID, turno).Scan(&conteo); err != nil {
		t.Fatalf("guardar un conteo de cierre con su esperado: %v", err)
	}

	// La diferencia es GENERADA: la resta la hace Postgres para que no puedan existir dos versiones
	// del mismo faltante.
	var diferencia string
	if err := st.Pool.QueryRow(ctx,
		`select difference::text from session_cash_counts where id = $1`, conteo).Scan(&diferencia); err != nil {
		t.Fatalf("leer la diferencia: %v", err)
	}
	if diferencia != "-50.00" {
		t.Fatalf("la diferencia salió %s y contar 340 contra 390 son -50.00", diferencia)
	}

	// Y no se puede escribir a mano.
	if _, err := st.Pool.Exec(ctx,
		`update session_cash_counts set difference = 0 where id = $1`, conteo); err == nil {
		t.Fatal("la diferencia del conteo se pudo escribir a mano: si es generada, nadie la escribe")
	}
}

// EL ESPERADO ESTÁ ATADO AL MOMENTO, y no a la disciplina de la aplicación.
//
// Sin este check, un `CloseSession` que algún día olvide setear el esperado deja un nulo que se lee
// IGUAL que un corte anterior a esta feature: el bug se disfraza de historia. Con él, falla ruidoso.
func TestElEsperadoDelConteoEstaAtadoAlMomento(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	turno := turnoDe(t, st, defaultCompanyID)

	insertar := func(momento string, expected any) error {
		_, err := st.Pool.Exec(ctx,
			`insert into session_cash_counts (company_id, session_id, moment, total, expected, created_by)
			 values ($1, $2, $3::cash_count_moment, 100, $4,
			         (select id from users where company_id = $1 order by id limit 1))`,
			defaultCompanyID, turno, momento, expected)
		return err
	}

	// La apertura no tiene contra qué comparar: el fondo ES lo que se contó.
	if err := insertar("apertura", nil); err != nil {
		t.Fatalf("una apertura sin esperado tiene que poder guardarse: %v", err)
	}
	if err := insertar("apertura", "100"); err == nil {
		t.Fatal("una apertura CON esperado se guardó: al abrir no hay nada que esperar")
	}
	// Un cierre sin esperado es el bug que este check atrapa.
	if _, err := st.Pool.Exec(ctx, `delete from session_cash_counts`); err != nil {
		t.Fatalf("limpiar: %v", err)
	}
	err := insertar("cierre", nil)
	if err == nil {
		t.Fatal("un cierre SIN esperado se guardó: ese nulo se lee igual que un corte viejo y esconde el bug")
	}
	if !strings.Contains(err.Error(), "23514") && !strings.Contains(err.Error(), "check") {
		t.Fatalf("falló por %v y tenía que ser el check del momento", err)
	}
}

// LOS CONTEOS QUE YA EXISTEN SOBREVIVEN.
//
// El check va `not valid` a propósito: la 0066 ya está en el ambiente de pruebas con conteos de
// cierre sin esperado, y una migración que los rechace no corre. `not valid` perdona el pasado y
// obliga al futuro.
func TestLosConteosDeLa0066SobrevivenAlCheck(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	turno := turnoDe(t, st, defaultCompanyID)

	// Se siembra un cierre sin esperado saltándose el check, que es exactamente la forma de las
	// filas que la 0066 dejó en dev.
	if _, err := st.Pool.Exec(ctx,
		`alter table session_cash_counts drop constraint session_cash_counts_expected_por_momento`); err != nil {
		t.Fatalf("el check del momento no existe con ese nombre: %v", err)
	}
	if _, err := st.Pool.Exec(ctx,
		`insert into session_cash_counts (company_id, session_id, moment, total, created_by)
		 values ($1, $2, 'cierre', 340,
		         (select id from users where company_id = $1 order by id limit 1))`,
		defaultCompanyID, turno); err != nil {
		t.Fatalf("sembrar un conteo estilo 0066: %v", err)
	}
	// Y se vuelve a poner como lo pone la migración: `not valid` no escanea lo que ya está.
	if _, err := st.Pool.Exec(ctx,
		`alter table session_cash_counts add constraint session_cash_counts_expected_por_momento check (
		   (moment = 'apertura' and expected is null) or (moment = 'cierre' and expected is not null)
		 ) not valid`); err != nil {
		t.Fatalf("el check no se pudo agregar sobre filas viejas: tiene que ir NOT VALID: %v", err)
	}

	var viejos int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from session_cash_counts where moment = 'cierre' and expected is null`).Scan(&viejos); err != nil {
		t.Fatalf("contar los viejos: %v", err)
	}
	if viejos != 1 {
		t.Fatalf("quedaron %d conteos viejos y se sembró 1: la migración no puede borrar el pasado", viejos)
	}
}

// EL RENGLÓN DEL CORTE GUARDA SI ESE MÉTODO TOCABA EL CAJÓN.
//
// Snapshot. Hoy el flag se reconstruye en vivo por join a `payment_methods`, y es inofensivo porque
// nadie puede cambiarlo. Esta feature le da un PATCH: el día que el dueño apague «el efectivo de
// Didi llega al cajón», todo corte cerrado antes se reagruparía con el flag de hoy. Las cifras no
// cambian, pero la forma del reporte sí — y eso es reescribir el pasado.
func TestElRenglonDelCorteGuardaSiTocabaElCajon(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	turno := turnoDe(t, st, defaultCompanyID)
	efectivo := paymentMethodID(t, st, "Efectivo")

	if _, err := st.Pool.Exec(ctx,
		`insert into register_session_totals (company_id, session_id, payment_method_id, expected, declared, affects_cash_drawer)
		 values ($1, $2, $3, 500, 500, true)`,
		defaultCompanyID, turno, efectivo); err != nil {
		t.Fatalf("guardar un renglón con su snapshot: %v", err)
	}

	// Apagar el flag del catálogo NO cambia lo que el corte guardó.
	if _, err := st.Pool.Exec(ctx,
		`update payment_methods set affects_cash_drawer = false where id = $1`, efectivo); err != nil {
		t.Fatalf("apagar el flag del catálogo: %v", err)
	}
	var guardado bool
	if err := st.Pool.QueryRow(ctx,
		`select affects_cash_drawer from register_session_totals where session_id = $1 and payment_method_id = $2`,
		turno, efectivo).Scan(&guardado); err != nil {
		t.Fatalf("leer el snapshot: %v", err)
	}
	if !guardado {
		t.Fatal("el corte cambió de forma al apagar el flag del catálogo: el snapshot no está guardando nada")
	}
}

// UN MÉTODO DE CAJÓN NO SE DECLARA POR SEPARADO, y la base lo hace cumplir.
//
// Es la mitad de FR-003 que ningún código puede garantizar solo: si un camino futuro escribe un
// declarado propio para un método de cajón, vuelve el faltante repartido que cancela sobrantes con
// faltantes — el defecto que esta feature entera viene a cerrar.
func TestUnMetodoDeCajonNoPuedeDeclararUnaCifraPropia(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	turno := turnoDe(t, st, defaultCompanyID)
	efectivo := paymentMethodID(t, st, "Efectivo")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")

	_, err := st.Pool.Exec(ctx,
		`insert into register_session_totals (company_id, session_id, payment_method_id, expected, declared, affects_cash_drawer)
		 values ($1, $2, $3, 500, 480, true)`,
		defaultCompanyID, turno, efectivo)
	exigeViolacionDeRestriccion(t, err,
		"un método de cajón con un declarado distinto de su esperado")

	// Y el que NO toca el cajón sí declara lo suyo: es su propia conciliación.
	if _, err := st.Pool.Exec(ctx,
		`insert into register_session_totals (company_id, session_id, payment_method_id, expected, declared, affects_cash_drawer)
		 values ($1, $2, $3, 300, 280, false)`,
		defaultCompanyID, turno, tarjeta); err != nil {
		t.Fatalf("un método que no toca el cajón tiene que poder declarar su cifra: %v", err)
	}
}

// EL ARQUEO CIEGO NACE APAGADO para las empresas que ya existen.
//
// El comportamiento de hoy —la diferencia se ve antes de confirmar— ya está implementado y probado
// (FR-005 de la 003). Una migración que lo cambie de golpe le mueve la pantalla a quien opera sin
// que nadie lo haya pedido.
func TestElArqueoCiegoNaceApagado(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// La empresa que siembra la migración 0022: la que ya existe en producción.
	var ciego bool
	if err := st.Pool.QueryRow(ctx,
		`select blind_cash_count from business_settings where company_id = $1`,
		defaultCompanyID).Scan(&ciego); err != nil {
		t.Fatalf("leer el ajuste de la empresa sembrada: %v", err)
	}
	if ciego {
		t.Fatal("la empresa que ya existe nació con el arqueo ciego encendido: la migración le movió la pantalla a quien opera")
	}

	// Y una empresa nueva tampoco lo hereda encendido: el default de la columna, no un insert que se
	// acuerde de nombrarla.
	otra := makeCompany(t, st, "arqueo-ciego-vecina")
	if _, err := st.Pool.Exec(ctx,
		`insert into business_settings (company_id, business_name) values ($1, 'Vecina')`, otra); err != nil {
		t.Fatalf("crear los ajustes de la empresa nueva: %v", err)
	}
	if err := st.Pool.QueryRow(ctx,
		`select blind_cash_count from business_settings where company_id = $1`, otra).Scan(&ciego); err != nil {
		t.Fatalf("leer el ajuste de la empresa nueva: %v", err)
	}
	if ciego {
		t.Fatal("una empresa nueva nace con el arqueo ciego encendido")
	}
}

// EL DOWN NO DEJA BASURA Y NO SE LLEVA LO DE ANTES.
func TestElDownDelArqueoDelCajonNoDejaBasura(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Hasta la 66, no "la última": el día que exista la 0068 esto seguiría revirtiendo una sola
	// migración y estaría comprobando el Down equivocado. Es la misma nota que dejó la 0066.
	t.Cleanup(func() { migrarArriba(t, st.Pool) })
	migrarAbajoHasta(t, st.Pool, 66)

	for _, c := range []struct{ tabla, columna string }{
		{"session_cash_counts", "expected"},
		{"session_cash_counts", "difference"},
		{"register_session_totals", "affects_cash_drawer"},
		{"business_settings", "blind_cash_count"},
	} {
		var existe bool
		if err := st.Pool.QueryRow(ctx,
			`select exists (select 1 from information_schema.columns
			                where table_name = $1 and column_name = $2)`, c.tabla, c.columna).Scan(&existe); err != nil {
			t.Fatalf("consultar %s.%s: %v", c.tabla, c.columna, err)
		}
		if existe {
			t.Errorf("el Down dejó %s.%s: un Up posterior chocaría con ella", c.tabla, c.columna)
		}
	}

	// Las restricciones también: una que se queda hace fallar el Up siguiente con "already exists",
	// y eso solo se descubre al reaplicar.
	for _, c := range []struct{ tabla, restriccion string }{
		{"session_cash_counts", "session_cash_counts_expected_por_momento"},
		{"register_session_totals", "register_session_totals_cajon_no_se_declara"},
		{"payment_methods", "payment_methods_cajon_no_se_autodeclara"},
	} {
		var existe bool
		if err := st.Pool.QueryRow(ctx,
			`select exists (select 1 from pg_constraint where conname = $1)`, c.restriccion).Scan(&existe); err != nil {
			t.Fatalf("consultar %s: %v", c.restriccion, err)
		}
		if existe {
			t.Errorf("el Down dejó la restricción %s en %s: el Up siguiente chocaría con ella", c.restriccion, c.tabla)
		}
	}

	var indice bool
	if err := st.Pool.QueryRow(ctx,
		`select exists (select 1 from pg_class where relname = 'payment_methods_un_efectivo_por_empresa')`).Scan(&indice); err != nil {
		t.Fatalf("consultar el índice del dueño del fondo: %v", err)
	}
	if indice {
		t.Error("el Down dejó payment_methods_un_efectivo_por_empresa: el Up siguiente chocaría con él")
	}

	// Y lo que la 0066 dejó sigue ahí: revertir el arqueo no borra el conteo.
	var conteo bool
	if err := st.Pool.QueryRow(ctx,
		`select exists (select 1 from information_schema.tables where table_name = 'session_cash_counts')`).Scan(&conteo); err != nil {
		t.Fatalf("consultar la tabla del conteo: %v", err)
	}
	if !conteo {
		t.Fatal("el Down de la 0067 se llevó la tabla de la 0066")
	}
}
