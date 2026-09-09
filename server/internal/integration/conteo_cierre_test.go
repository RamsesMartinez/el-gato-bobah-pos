//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Cerrar la caja contando el cajón (spec 003, US2).
//
// La apertura es la mitad fácil: no hay nada contra qué comparar. Aquí el conteo se convierte en el
// DECLARADO de un método y de uno solo, y la diferencia del arqueo sale de esa cifra — si el conteo
// alimenta el método equivocado, o alimenta dos, el corte reporta un faltante que no existe. Eso ya
// pasó una vez con el fondo de caja ($4,500 de faltante inexplicable) y es lo que este archivo
// existe para que no se repita.

// turnoConVentaEnEfectivo abre la caja principal con un fondo declarado a mano, cobra una venta en
// efectivo y entrega lo vendido, que es lo que haría el operador antes de cerrar.
//
// Devuelve el id de la caja, el del método de efectivo y lo que el cajón debería tener.
func turnoConVentaEnEfectivo(t *testing.T, ctx context.Context, st *store.Store,
	cajero int64, fondo, venta string) (int64, int64, decimal.Decimal) {
	t.Helper()
	backoffice := app.NewBackofficeService(st, clock)
	orders := app.NewOrdersService(st, clock)
	principal := registerID(t, st, "Caja principal")
	if _, err := backoffice.OpenSession(ctx, principal, aperturaAMano(decimal.RequireFromString(fondo)), cajero); err != nil {
		t.Fatalf("abrir la caja: %v", err)
	}
	efectivo := paymentMethodID(t, st, "Efectivo")
	prod := makeProduct(t, st, "Crepa de prueba", decimal.RequireFromString(venta), false)
	if _, err := crearYCobrar(t, ctx, orders, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString(venta)}},
	}); err != nil {
		t.Fatalf("cobrar la venta en efectivo: %v", err)
	}
	entregarPendientes(t, st)
	esperado := decimal.RequireFromString(fondo).Add(decimal.RequireFromString(venta))
	return principal, int64(efectivo), esperado
}

// declaradoDe lee lo que quedó guardado para un método, incluida la DIFERENCIA, que es columna
// generada: se lee de la base y no del view para probar que nadie la calcula a mano.
func declaradoDe(t *testing.T, st *store.Store, principal, metodo int64) (decimal.Decimal, decimal.Decimal) {
	t.Helper()
	var declarado, diferencia decimal.Decimal
	if err := st.Pool.QueryRow(context.Background(),
		`select t.declared, t.difference
		 from register_session_totals t
		 join register_sessions s on s.id = t.session_id
		 where s.register_id = $1 and t.payment_method_id = $2
		 order by t.session_id desc limit 1`, principal, metodo).Scan(&declarado, &diferencia); err != nil {
		t.Fatalf("leer el declarado del método %d: %v", metodo, err)
	}
	return declarado, diferencia
}

// cierreAMano: cerrar declarando las cifras sin contar el cajón, que es lo que hacían todos los
// tests antes de esta feature. Lleva motivo porque FR-014 lo exige en cuanto el efectivo viene como
// cifra: un arqueo sin desglose y sin explicación no se puede auditar, y eso vale igual para un
// turno de prueba.
func cierreAMano(declarado map[int]decimal.Decimal) app.CierreCmd {
	return app.CierreCmd{
		Declarado: declarado,
		Motivo:    "efectivo declarado sin contar (fixture de prueba)",
	}
}

// EL CONTEO ALIMENTA EL DECLARADO DEL EFECTIVO, Y DE NINGÚN OTRO MÉTODO.
//
// Es el requisito entero de US2 en una línea. El fondo de caja ya enseñó lo que cuesta equivocarse
// de método: sumarlo a los cuatro que tocan cajón inventó $4,500 de faltante.
func TestElConteoDeCierreAlimentaSoloElDeclaradoDelEfectivo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_cierre", "cajero")

	// Fondo de $200 y una venta de $140 en efectivo: el cajón debe tener $340.
	principal, efectivo, esperado := turnoConVentaEnEfectivo(t, ctx, st, cajero, "200", "140")
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")

	// Piezas que suman exactamente los $340: 3 de $100, 2 de $20.
	cerrada, err := backoffice.CloseSession(ctx, principal, cajero, app.CierreCmd{
		Piezas:    piezasDe(t, st, "100", 3, "20", 2),
		Declarado: map[int]decimal.Decimal{int(tarjeta): decimal.RequireFromString("0")},
	})
	if err != nil {
		t.Fatalf("cerrar contando el cajón: %v", err)
	}

	declarado, diferencia := declaradoDe(t, st, principal, efectivo)
	if !declarado.Equal(esperado) {
		t.Fatalf("el efectivo declarado quedó en %s y las piezas suman %s: el conteo no alimentó el arqueo",
			declarado, esperado)
	}
	if !diferencia.IsZero() {
		t.Fatalf("la diferencia del efectivo salió %s con un conteo exacto: el arqueo reporta un faltante que no existe",
			diferencia)
	}
	// Y el conteo NO se derramó a otro método: la tarjeta declaró lo que mandó la pantalla.
	tarjetaDeclarada, _ := declaradoDe(t, st, principal, int64(tarjeta))
	if !tarjetaDeclarada.IsZero() {
		t.Fatalf("la tarjeta declaró %s: el conteo del cajón se derramó a un método que no es efectivo",
			tarjetaDeclarada)
	}
	if cerrada.Status != "cerrada" {
		t.Fatalf("el turno quedó en %q", cerrada.Status)
	}

	// El desglose quedó guardado con su momento, que es lo que US3 lee después.
	var piezas int
	if err := st.Pool.QueryRow(ctx,
		`select coalesce(sum(l.pieces), 0) from session_cash_count_lines l
		 join session_cash_counts c on c.id = l.count_id
		 where c.moment = 'cierre'`).Scan(&piezas); err != nil {
		t.Fatalf("contar los renglones del cierre: %v", err)
	}
	if piezas != 5 {
		t.Fatalf("quedaron %d piezas en el conteo de cierre y se capturaron 5", piezas)
	}
}

// LOS DOS CAMINOS NO PUEDEN LLEGAR JUNTOS PARA EL EFECTIVO (FR-015).
//
// Son dos cifras del mismo dinero: quedarse con cualquiera es inventar cuál era la buena. Y el
// rechazo tiene que ser ANTES de escribir: un cierre a medias deja el turno sin poder cerrarse.
func TestMandarConteoYDeclaradoDelEfectivoSeRechaza(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_ambiguo", "cajero")
	principal, efectivo, _ := turnoConVentaEnEfectivo(t, ctx, st, cajero, "200", "140")

	_, err := backoffice.CloseSession(ctx, principal, cajero, app.CierreCmd{
		Piezas:    piezasDe(t, st, "100", 3, "20", 2),
		Declarado: map[int]decimal.Decimal{int(efectivo): decimal.RequireFromString("999")},
	})
	if !errors.Is(err, domain.ErrConteoAmbiguo) {
		t.Fatalf("mandar conteo y declarado del efectivo dio %v y tiene que ser ErrConteoAmbiguo", err)
	}

	// Y el turno sigue abierto: nada se escribió.
	var estado string
	if err := st.Pool.QueryRow(ctx,
		`select status from register_sessions where register_id = $1 order by id desc limit 1`,
		principal).Scan(&estado); err != nil {
		t.Fatalf("leer el estado del turno: %v", err)
	}
	if estado != "abierta" {
		t.Fatalf("el turno quedó %q tras un cierre rechazado: el operador no puede volver a cerrar", estado)
	}
}

// EL FALTANTE SALE DE LA COLUMNA GENERADA, no de una resta que alguien escribió.
func TestElFaltanteDelCierreSaleDeLaColumnaGenerada(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_faltante", "cajero")
	principal, efectivo, _ := turnoConVentaEnEfectivo(t, ctx, st, cajero, "200", "140")

	// El cajón debía tener $340 y se cuentan $290 —2 de $100, 4 de $20 y 1 de $10—, así que faltan
	// $50. La cifra la resta Postgres, no este test.
	if _, err := backoffice.CloseSession(ctx, principal, cajero, app.CierreCmd{
		Piezas: piezasDe(t, st, "100", 2, "20", 4, "10", 1),
	}); err != nil {
		t.Fatalf("cerrar con faltante: %v", err)
	}

	_, diferencia := declaradoDe(t, st, principal, efectivo)
	if !diferencia.Equal(decimal.RequireFromString("-50")) {
		t.Fatalf("la diferencia salió %s y el faltante real es -50", diferencia)
	}
}

// EL CONTEO SE ESCRIBE DENTRO DE LA MISMA TRANSACCIÓN QUE CIERRA EL TURNO, y un conteo que ya
// existe no se pisa: se dice.
//
// T024b. El caso no es hipotético: las dos tabletas comparten cuenta, así que las dos pueden tener
// el cierre abierto y las dos confirmar. Aquí se fuerza de forma determinista sembrando el conteo de
// cierre antes de cerrar — es el mismo choque con `unique (session_id, moment)` que produce la
// carrera, sin depender del reloj.
//
// Lo que exige el test: que salga un CONFLICTO accionable y no un 500, y que el turno siga ABIERTO
// con sus totales sin escribir. Sin transacción, los totales quedarían guardados contra un cierre
// que no ocurrió.
func TestUnConteoDeCierreQueYaExisteNoSePisaYElCierreNoQuedaAMedias(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_dos_cierres", "cajero")
	principal, _, _ := turnoConVentaEnEfectivo(t, ctx, st, cajero, "200", "140")

	var sesion int64
	if err := st.Pool.QueryRow(ctx,
		`select id from register_sessions where register_id = $1 order by id desc limit 1`,
		principal).Scan(&sesion); err != nil {
		t.Fatalf("leer el turno: %v", err)
	}
	if _, err := st.Pool.Exec(ctx,
		`insert into session_cash_counts (session_id, moment, total, created_by)
		 values ($1, 'cierre', 340, $2)`, sesion, cajero); err != nil {
		t.Fatalf("sembrar el conteo de cierre del primero: %v", err)
	}

	_, err := backoffice.CloseSession(ctx, principal, cajero, app.CierreCmd{
		Piezas: piezasDe(t, st, "100", 3, "20", 2),
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("el segundo cierre dio %v y tiene que ser ErrConflict: un 500 le dice al operador "+
			"que el sistema se rompió cuando lo que pasó es que alguien más ya cerró", err)
	}

	var estado string
	var totales int
	if err := st.Pool.QueryRow(ctx,
		`select s.status, (select count(*) from register_session_totals where session_id = s.id)
		 from register_sessions s where s.id = $1`, sesion).Scan(&estado, &totales); err != nil {
		t.Fatalf("leer el turno: %v", err)
	}
	if estado != "abierta" || totales != 0 {
		t.Fatalf("el turno quedó %q con %d totales escritos: el cierre no fue atómico",
			estado, totales)
	}
}

// DECLARAR EL EFECTIVO A MANO EXIGE MOTIVO, igual que al abrir (FR-014 y FR-016).
//
// Es la mitad que el contrato del cierre no decía y el spec sí: un arqueo con una cifra de efectivo
// que nadie puede reconstruir ni justificar es el problema que esta feature viene a cerrar, y da
// igual si la cifra entró al abrir o al cerrar.
func TestDeclararElEfectivoDelCierreAManoExigeMotivo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_sin_motivo", "cajero")
	principal, efectivo, esperado := turnoConVentaEnEfectivo(t, ctx, st, cajero, "200", "140")

	sinMotivo := app.CierreCmd{Declarado: map[int]decimal.Decimal{int(efectivo): esperado}}
	if _, err := backoffice.CloseSession(ctx, principal, cajero, sinMotivo); !errors.Is(err, domain.ErrConteoSinExplicar) {
		t.Fatalf("cerrar declarando el efectivo sin motivo dio %v y tiene que exigir el motivo", err)
	}

	conMotivo := sinMotivo
	conMotivo.Motivo = "el cajón trae billetes que no están en la lista"
	if _, err := backoffice.CloseSession(ctx, principal, cajero, conMotivo); err != nil {
		t.Fatalf("cerrar declarando el efectivo con motivo: %v", err)
	}

	var motivo *string
	if err := st.Pool.QueryRow(ctx,
		`select manual_reason from session_cash_counts where moment = 'cierre'`).Scan(&motivo); err != nil {
		t.Fatalf("leer el motivo del cierre: %v", err)
	}
	if motivo == nil || *motivo != conMotivo.Motivo {
		t.Fatalf("el motivo no quedó guardado con el arqueo: %v", motivo)
	}
}

// CERRAR SIN DECLARAR EFECTIVO NO INVENTA UN CONTEO.
//
// Una caja secundaria que no vendió nada cierra sin nada que contar, y ahí no hay arqueo de efectivo
// que explicar. Guardar un conteo en cero diría "conté el cajón y estaba vacío", que es una
// afirmación distinta —y falsa— de "nadie contó". FR-008 ya obliga a que la pantalla sepa mostrar un
// corte sin desglose: éste es el caso nuevo que sigue produciéndolos.
func TestCerrarSinDeclararEfectivoNoInventaUnConteo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_sin_efectivo", "cajero")
	principal := registerID(t, st, "Caja fuerte")
	if _, err := backoffice.OpenSession(ctx, principal, app.AperturaCmd{}, cajero); err != nil {
		t.Fatalf("abrir la caja: %v", err)
	}

	if _, err := backoffice.CloseSession(ctx, principal, cajero, app.CierreCmd{}); err != nil {
		t.Fatalf("cerrar una caja sin efectivo: %v", err)
	}

	var conteos int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from session_cash_counts where moment = 'cierre'`).Scan(&conteos); err != nil {
		t.Fatalf("contar conteos de cierre: %v", err)
	}
	if conteos != 0 {
		t.Fatalf("quedaron %d conteos de cierre sin que nadie contara: un arqueo inventado es peor "+
			"que no tenerlo, porque se lee como un hecho", conteos)
	}
}

// EL DESGLOSE SE LEE DESPUÉS, que es la razón de guardarlo (US3).
//
// Un corte con faltante y sin desglose es un número sin historia: no hay forma de distinguir "faltan
// dos billetes de $500" de "falta dinero". Eso ya costó un turno con $1,662 sin explicación.
func TestElDetalleDelCorteTraeElDesgloseDeLoContado(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_desglose", "cajero")
	principal, _, _ := turnoConVentaEnEfectivo(t, ctx, st, cajero, "200", "140")

	// Se cierra contando $290 contra $340 esperados: dos de $100, cuatro de $20 y una de $10.
	cerrada, err := backoffice.CloseSession(ctx, principal, cajero, app.CierreCmd{
		Piezas: piezasDe(t, st, "100", 2, "20", 4, "10", 1),
	})
	if err != nil {
		t.Fatalf("cerrar: %v", err)
	}

	detalle, err := backoffice.SessionDetail(ctx, cerrada.ID)
	if err != nil {
		t.Fatalf("SessionDetail: %v", err)
	}
	if detalle.Counts == nil || detalle.Counts.Cierre == nil {
		t.Fatal("el detalle no trae el conteo de cierre: el desglose se guardó y no se puede leer")
	}
	cierre := detalle.Counts.Cierre
	if len(cierre.Lines) != 3 {
		t.Fatalf("el desglose trae %d renglones y se capturaron 3", len(cierre.Lines))
	}
	// De mayor a menor, que es como se cuenta un cajón y como se compara contra él.
	if !cierre.Lines[0].Value.Equal(decimal.RequireFromString("100")) {
		t.Fatalf("el primer renglón es de $%s: el desglose tiene que venir de mayor a menor",
			cierre.Lines[0].Value)
	}
	// El SUBTOTAL viaja calculado: quien lo lee está comparando contra su cajón, y si lo multiplica
	// la pantalla hay dos multiplicaciones del mismo dato que pueden diferir.
	if !cierre.Lines[0].Subtotal.Equal(decimal.RequireFromString("200")) {
		t.Fatalf("el subtotal de 2 × $100 salió %s", cierre.Lines[0].Subtotal)
	}
	if cierre.ManualReason != nil {
		t.Fatalf("un conteo por denominaciones no lleva motivo, trajo %q", *cierre.ManualReason)
	}
	// Y el de la apertura sigue ahí, con su motivo: el fondo se declaró a mano.
	if detalle.Counts.Apertura == nil || detalle.Counts.Apertura.ManualReason == nil {
		t.Fatal("el detalle perdió el conteo de apertura o su motivo")
	}
}

// UN CORTE CERRADO ANTES DE ESTA FUNCIONALIDAD NO INVENTA UN DESGLOSE (FR-008 / SC-006).
//
// Es el caso de TODOS los cortes que ya existen en producción. La pantalla los tiene que mostrar
// como siempre, y sus cifras no pueden cambiar: un desglose vacío que se pinte como "se contó y no
// había nada" es una afirmación que nadie hizo.
func TestUnCorteViejoSinConteoSigueLeyendoseIgual(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_viejo", "cajero")
	principal, _, esperado := turnoConVentaEnEfectivo(t, ctx, st, cajero, "200", "140")

	cerrada, err := backoffice.CloseSession(ctx, principal, cajero, cierreAMano(
		map[int]decimal.Decimal{int(paymentMethodID(t, st, "Efectivo")): esperado}))
	if err != nil {
		t.Fatalf("cerrar: %v", err)
	}
	// Se borran los conteos para dejar el turno como los que cerraron antes de la 0066.
	if _, err := st.Pool.Exec(ctx, `delete from session_cash_counts`); err != nil {
		t.Fatalf("simular un corte anterior a la feature: %v", err)
	}

	detalle, err := backoffice.SessionDetail(ctx, cerrada.ID)
	if err != nil {
		t.Fatalf("SessionDetail de un corte sin conteo: %v", err)
	}
	if detalle.Counts != nil && (detalle.Counts.Apertura != nil || detalle.Counts.Cierre != nil) {
		t.Fatal("el detalle inventó un conteo que nadie capturó")
	}
	// Y sus cifras siguen intactas: SC-006.
	if !detalle.OpeningCash.Equal(decimal.RequireFromString("200")) {
		t.Fatalf("el fondo del corte viejo cambió a %s", detalle.OpeningCash)
	}
}

// EL MOTIVO SE LEE DESPUÉS, igual que el desglose: es lo que hace auditable un arqueo sin piezas.
func TestElDetalleDelCorteTraeElMotivoCuandoNoSeContó(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_motivo_detalle", "cajero")
	principal, efectivo, esperado := turnoConVentaEnEfectivo(t, ctx, st, cajero, "200", "140")

	cerrada, err := backoffice.CloseSession(ctx, principal, cajero, app.CierreCmd{
		Declarado: map[int]decimal.Decimal{int(efectivo): esperado},
		Motivo:    "el cajón trae un billete que no está en la lista",
	})
	if err != nil {
		t.Fatalf("cerrar a mano: %v", err)
	}

	detalle, err := backoffice.SessionDetail(ctx, cerrada.ID)
	if err != nil {
		t.Fatalf("SessionDetail: %v", err)
	}
	cierre := detalle.Counts.Cierre
	if cierre == nil || cierre.ManualReason == nil {
		t.Fatal("el corte cerrado a mano no trae su motivo: el arqueo queda sin explicación")
	}
	if len(cierre.Lines) != 0 {
		t.Fatalf("un arqueo capturado a mano trajo %d renglones", len(cierre.Lines))
	}
}

// EL TURNO ABIERTO TAMBIÉN MUESTRA LO QUE SE CONTÓ AL ABRIR.
//
// La misma vista, derivada una sola vez: si el turno abierto y el corte cerrado sacaran el desglose
// por caminos distintos, tendríamos dos pantallas que no coinciden y ninguna forma de saber cuál
// miente.
func TestElTurnoAbiertoTraeElConteoDeSuApertura(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_abierto", "cajero")
	principal := registerID(t, st, "Caja principal")

	if _, err := backoffice.OpenSession(ctx, principal, app.AperturaCmd{
		Piezas: piezasDe(t, st, "500", 1, "100", 2),
	}, cajero); err != nil {
		t.Fatalf("abrir contando: %v", err)
	}

	vista, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	if vista.Counts == nil || vista.Counts.Apertura == nil {
		t.Fatal("el turno abierto no trae el conteo de su apertura")
	}
	if len(vista.Counts.Apertura.Lines) != 2 {
		t.Fatalf("la apertura trae %d renglones y se capturaron 2", len(vista.Counts.Apertura.Lines))
	}
	if vista.Counts.Cierre != nil {
		t.Fatal("un turno abierto no puede tener conteo de cierre")
	}
}
