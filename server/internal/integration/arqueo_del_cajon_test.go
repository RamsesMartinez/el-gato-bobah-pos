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

// UN SOLO ARQUEO DE CAJÓN (spec 015, US1).
//
// El cajón es un solo montón de billetes. El corte pedía una cifra declarada por CADA método que lo
// toca —el efectivo del mostrador y los tres de plataforma en efectivo—, y un turno real quedó con
// «Efectivo» en diferencia $0.00 y «Didi efectivo» en −$64.80: dos diferencias del mismo dinero, sin
// forma de saber si el faltante era real o si ese dinero se contó dentro del otro renglón.
//
// Lo que este archivo defiende: una cifra esperada, un conteo, una diferencia.

// turnoConEfectivoDeMostradorYDeApp deja un turno abierto con $500 de fondo, $200 cobrados en
// efectivo de mostrador y un pedido de Didi pagado en efectivo al repartidor del local.
//
// Devuelve la caja, el id del método de efectivo, el del efectivo de Didi y lo que el cajón debería
// tener. El pedido de plataforma cobra con el precio de la plataforma (35% sembrado), así que su
// importe se toma de lo que de verdad se cobró y no de una suposición.
func turnoConEfectivoDeMostradorYDeApp(t *testing.T, ctx context.Context, st *store.Store,
	cajero int64) (int64, int16, int16, decimal.Decimal) {
	t.Helper()
	backoffice := app.NewBackofficeService(st, clock)
	orders := app.NewOrdersService(st, clock)

	principal := registerID(t, st, "Caja principal")
	if _, err := backoffice.OpenSession(ctx, principal, aperturaAMano(decimal.RequireFromString("500")), cajero); err != nil {
		t.Fatalf("abrir la caja: %v", err)
	}
	efectivo := paymentMethodID(t, st, "Efectivo")
	didiEfectivo := paymentMethodID(t, st, "Didi efectivo")
	didi := platformID(t, st, defaultCompanyID, "Didi")

	// Mostrador, $200 en efectivo.
	mostrador := makeProduct(t, st, "Crepa de mostrador", decimal.RequireFromString("200"), false)
	if _, err := crearYCobrar(t, ctx, orders, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: mostrador, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("200")}},
	}); err != nil {
		t.Fatalf("cobrar la venta de mostrador: %v", err)
	}

	// Didi en efectivo: el repartidor del local trae el dinero y entra al MISMO cajón.
	deApp := makeProduct(t, st, "Crepa de app", decimal.RequireFromString("100"), false)
	pedido, err := crearYCobrar(t, ctx, orders, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "domicilio", DeliveryPlatformID: &didi, OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: deApp, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: didiEfectivo, Amount: decimal.RequireFromString("135")}},
	})
	if err != nil {
		t.Fatalf("cobrar el pedido de Didi en efectivo: %v", err)
	}
	entregarPendientes(t, st)

	// $500 de fondo + $200 del mostrador + lo que cobró el pedido de la app.
	esperado := decimal.RequireFromString("700").Add(pedido.Total)
	return principal, efectivo, didiEfectivo, esperado
}

// EL CAJÓN SE ARQUEA UNA SOLA VEZ, contra una sola cifra.
func TestElCajonSeArqueaUnaSolaVez(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_cajon", "cajero")
	principal, efectivo, didiEfectivo, esperado := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)

	if !esperado.Equal(decimal.RequireFromString("835")) {
		t.Fatalf("el fixture esperaba $835 en el cajón y armó %s: revisa el precio de plataforma", esperado)
	}

	abierta, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	if abierta.Drawer == nil {
		t.Fatal("el turno abierto no trae el arqueo del cajón: la pantalla no tiene contra qué comparar")
	}
	if abierta.Drawer.Expected == nil || !abierta.Drawer.Expected.Equal(esperado) {
		t.Fatalf("el cajón espera %v y debería esperar %s — el fondo y el efectivo de la app van adentro",
			abierta.Drawer.Expected, esperado)
	}
	if !abierta.Drawer.RequiresCount {
		t.Fatal("un cajón que espera dinero tiene que exigir conteo: si no, el cierre se firma vacío")
	}
	// El cajón lo forman TODOS los métodos configurados para que su dinero llegue ahí —el del
	// mostrador y los tres de plataforma en efectivo—, hayan vendido o no: si mañana cobran, ese
	// dinero cae en el mismo montón. Lo que no puede estar es la tarjeta.
	enElCajon := map[int]bool{}
	for _, id := range abierta.Drawer.MethodIDs {
		enElCajon[id] = true
	}
	for _, m := range []int16{efectivo, didiEfectivo} {
		if !enElCajon[int(m)] {
			t.Fatalf("el método %d no está en el cajón y su dinero sí: %v", m, abierta.Drawer.MethodIDs)
		}
	}
	if enElCajon[int(paymentMethodID(t, st, "Tarjeta débito"))] {
		t.Fatalf("la tarjeta entró al cajón: %v — su dinero está en una terminal", abierta.Drawer.MethodIDs)
	}

	// Se cuenta exactamente lo esperado: 1 de $500, 3 de $100, 1 de $20, 1 de $10, 1 de $5.
	cerrada, err := backoffice.CloseSession(ctx, principal, cajero, app.CierreCmd{
		Piezas: piezasDe(t, st, "500", 1, "100", 3, "20", 1, "10", 1, "5", 1),
	})
	if err != nil {
		t.Fatalf("cerrar contando el cajón: %v", err)
	}
	if cerrada.Drawer == nil || cerrada.Drawer.Difference == nil {
		t.Fatal("el corte cerrado no trae la diferencia del cajón")
	}
	if !cerrada.Drawer.Difference.IsZero() {
		t.Fatalf("la diferencia del cajón salió %v con un conteo exacto", cerrada.Drawer.Difference)
	}

	// Y NINGÚN método de cajón reporta diferencia propia: es el defecto del turno 3 —«Efectivo» en 0
	// y «Didi efectivo» en −64.80— y aquí no puede volver a pasar.
	for _, metodo := range []int16{efectivo, didiEfectivo} {
		declarado, diferencia := declaradoDe(t, st, principal, int64(metodo))
		if !diferencia.IsZero() {
			t.Fatalf("el método %d reporta una diferencia propia de %s: el cajón se arquea una vez",
				metodo, diferencia)
		}
		if declarado.IsZero() {
			t.Fatalf("el método %d quedó declarado en cero: su dinero está en el cajón y se contó", metodo)
		}
	}
}

// UN FALTANTE DEL CAJÓN ES UNO SOLO, y llega al histórico.
func TestUnFaltanteDelCajonEsUnoSoloYLlegaAlHistorico(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_faltante_cajon", "cajero")
	principal, _, _, _ := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)

	// Se cuentan $785 contra $835: faltan $50. 1 de $500, 2 de $100, 4 de $20, 1 de $5.
	cerrada, err := backoffice.CloseSession(ctx, principal, cajero, app.CierreCmd{
		Piezas: piezasDe(t, st, "500", 1, "100", 2, "20", 4, "5", 1),
	})
	if err != nil {
		t.Fatalf("cerrar con faltante: %v", err)
	}
	if cerrada.Drawer.Difference == nil || !cerrada.Drawer.Difference.Equal(decimal.RequireFromString("-50")) {
		t.Fatalf("la diferencia del cajón salió %v y el faltante real es -50", cerrada.Drawer.Difference)
	}

	// EL HISTÓRICO LO REPORTA. Es el caso que sin la subconsulta del conteo daría CERO: los métodos
	// de cajón guardan declared = expected, así que la suma por método no ve el faltante y la
	// pantalla donde alguien audita mostraría cuadrado un corte que no cuadra.
	historico, err := backoffice.SessionHistory(ctx, 20)
	if err != nil {
		t.Fatalf("SessionHistory: %v", err)
	}
	var visto bool
	for _, fila := range historico {
		if fila.ID != cerrada.ID {
			continue
		}
		visto = true
		if !fila.TotalDifference.Equal(decimal.RequireFromString("-50")) {
			t.Fatalf("el histórico reporta %s de diferencia y el faltante es -50", fila.TotalDifference)
		}
	}
	if !visto {
		t.Fatalf("el corte %d no apareció en el histórico", cerrada.ID)
	}
}

// UN MÉTODO DE CAJÓN EN `declared` SE RECHAZA, nombrándolo.
//
// El caso realista no es un atacante: es una tableta con el front viejo en caché —la aplicación es
// una PWA con service worker— mandando el cuerpo de antes. Ignorar esa cifra dejaría al operador
// creyendo que declaró algo que no se guardó.
func TestUnMetodoDeCajonEnDeclaradoSeRechaza(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_declarado_viejo", "cajero")
	principal, efectivo, _, _ := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)

	_, err := backoffice.CloseSession(ctx, principal, cajero, app.CierreCmd{
		Piezas:    piezasDe(t, st, "500", 1),
		Declarado: map[int]decimal.Decimal{int(efectivo): decimal.RequireFromString("835")},
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("declarar por método el dinero del cajón dio %v y tiene que ser ErrValidation", err)
	}

	var estado string
	if err := st.Pool.QueryRow(ctx,
		`select status::text from register_sessions where register_id = $1 order by id desc limit 1`,
		principal).Scan(&estado); err != nil {
		t.Fatalf("leer el estado del turno: %v", err)
	}
	if estado != "abierta" {
		t.Fatalf("el turno quedó %q tras un cierre rechazado", estado)
	}
}

// EL EFECTIVO DE UNA APP QUE NO LLEGA AL CAJÓN NO SE ESPERA EN EL CAJÓN.
//
// Es la respuesta a que el reparto lo hace a veces gente del local —y el dinero regresa— y a veces
// el repartidor de la plataforma, que se lo lleva y lo descuenta del depósito.
func TestElEfectivoDeUnaAppQueNoLlegaAlCajonSeDeclaraAparte(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_app_sin_cajon", "cajero")
	principal, _, didiEfectivo, esperadoConApp := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)

	// El dinero de Didi ya no entra al cajón: lo cobra su repartidor. Se mueve por SQL y no por el
	// endpoint de US2 a propósito: esta historia tiene que poder probarse sola.
	if _, err := st.Pool.Exec(ctx,
		`update payment_methods set affects_cash_drawer = false where id = $1`, didiEfectivo); err != nil {
		t.Fatalf("sacar el efectivo de Didi del cajón: %v", err)
	}

	abierta, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	deApp := esperadoConApp.Sub(decimal.RequireFromString("700"))
	quiere := esperadoConApp.Sub(deApp)
	if !abierta.Drawer.Expected.Equal(quiere) {
		t.Fatalf("el cajón espera %v y debería esperar %s: el dinero que se lleva el repartidor de la app no está ahí",
			abierta.Drawer.Expected, quiere)
	}
	// Y ese método pasa a exigir su propia cifra: es la conciliación con la plataforma, no el cajón.
	var exige bool
	for _, m := range abierta.Totals {
		if m.MethodID == int(didiEfectivo) {
			exige = m.RequiresEntry
		}
	}
	if !exige {
		t.Fatal("el efectivo de la app que no llega al cajón tiene que pedir su cifra: si no, ese dinero no se declara en ningún lado")
	}
}

// UN CORTE CERRADO NO SE REAGRUPA AL CAMBIAR EL INTERRUPTOR (T009).
//
// El snapshot. Sin él, apagar «el efectivo de Didi llega al cajón» el mes que viene hace que todo
// corte cerrado antes se lea con la configuración de hoy: las cifras no cambian, pero la forma del
// reporte sí, y un arqueo que se lee distinto según cuándo lo abras no se puede auditar.
func TestUnCorteCerradoNoSeReagrupaAlCambiarElInterruptor(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_snapshot", "cajero")
	principal, _, didiEfectivo, _ := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)

	cerrada, err := backoffice.CloseSession(ctx, principal, cajero, app.CierreCmd{
		Piezas: piezasDe(t, st, "500", 1, "100", 3, "20", 1, "10", 1, "5", 1),
	})
	if err != nil {
		t.Fatalf("cerrar: %v", err)
	}
	antes, err := backoffice.SessionDetail(ctx, cerrada.ID)
	if err != nil {
		t.Fatalf("SessionDetail antes: %v", err)
	}

	if _, err := st.Pool.Exec(ctx,
		`update payment_methods set affects_cash_drawer = false where id = $1`, didiEfectivo); err != nil {
		t.Fatalf("cambiar el interruptor: %v", err)
	}

	despues, err := backoffice.SessionDetail(ctx, cerrada.ID)
	if err != nil {
		t.Fatalf("SessionDetail después: %v", err)
	}
	if len(antes.Drawer.MethodIDs) != len(despues.Drawer.MethodIDs) {
		t.Fatalf("el corte se reagrupó: antes el cajón lo formaban %v y ahora %v",
			antes.Drawer.MethodIDs, despues.Drawer.MethodIDs)
	}
	if !antes.Drawer.Expected.Equal(*despues.Drawer.Expected) {
		t.Fatalf("el esperado del corte cambió de %v a %v al mover un interruptor",
			antes.Drawer.Expected, despues.Drawer.Expected)
	}
}

// CERRAR SIN CONTAR EL CAJÓN NO PUEDE DAR UN CORTE CUADRADO.
//
// El agujero que abrió esta feature y que su propio diseño esconde. Como cada método del cajón
// declara su esperado —la única forma de escribir "a este nadie le declaró una cifra propia" en una
// columna `not null`—, un cierre que llega sin conteo dejaba los cuatro renglones con diferencia
// $0.00 y ninguna fila de conteo de la cual sacar la diferencia real: el corte reportaba **$0 de
// diferencia con $835 que nadie contó**, en la pantalla y en el histórico.
//
// Es PEOR que antes de la 015: ahí el mismo cierre dejaba a «Efectivo» con declarado 0 y un
// faltante de −$835 bien visible. Y es exactamente lo que FR-016 prohíbe — un campo vacío no se
// trata como cero.
//
// El camino no es hipotético: `counts: []` sin `countedCash` es lo que manda una tableta con el
// front viejo en caché, o cualquier cliente al que le falle el envío del conteo.
func TestCerrarSinContarElCajonSeRechaza(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_sin_contar", "cajero")
	principal, _, _, esperado := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)

	_, err := backoffice.CloseSession(ctx, principal, cajero, app.CierreCmd{})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("cerrar sin contar un cajón que espera %s dio %v: tiene que rechazarse, no cerrar en $0 de diferencia",
			esperado, err)
	}

	// Y el turno sigue abierto: un cierre rechazado no puede dejarlo a medias.
	abierta, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	if abierta == nil {
		t.Fatal("el cierre se rechazó y aun así cerró el turno")
	}
}
