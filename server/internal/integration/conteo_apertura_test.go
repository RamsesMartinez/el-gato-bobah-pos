//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Abrir la caja contando el cajón (spec 003, US1).
//
// El valor de la feature entero está en una línea: el operador captura PIEZAS y el servidor suma.
// Todo lo demás de este archivo existe para que esa línea no se pueda romper en silencio.

// piezasDe arma la captura buscando cada denominación por su valor, como haría la pantalla con el
// catálogo que le devuelve el endpoint.
func piezasDe(t *testing.T, st *store.Store, pares ...any) []app.PiezaCapturada {
	t.Helper()
	if len(pares)%2 != 0 {
		t.Fatalf("piezasDe quiere pares valor/piezas, llegaron %d argumentos", len(pares))
	}
	out := make([]app.PiezaCapturada, 0, len(pares)/2)
	for i := 0; i < len(pares); i += 2 {
		valor := pares[i].(string)
		out = append(out, app.PiezaCapturada{
			DenominationID: denominacionMXN(t, st, valor),
			Pieces:         pares[i+1].(int),
		})
	}
	return out
}

// El caso del spec: 6 monedas de $10 y 3 billetes de $50 son $210, y nadie escribió "210".
func TestAbrirLaCajaContandoElCajon(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_apertura", "cajero")
	principal := registerID(t, st, "Caja principal")

	vista, err := backoffice.OpenSession(ctx, principal, app.AperturaCmd{
		Piezas: piezasDe(t, st, "10", 6, "50", 3),
	}, cajero)
	if err != nil {
		t.Fatalf("abrir contando: %v", err)
	}
	if !vista.OpeningCash.Equal(decimal.RequireFromString("210")) {
		t.Fatalf("el fondo quedó en %s y las piezas suman 210: si el servidor no recalcula, el "+
			"operador sigue sumando en la libreta", vista.OpeningCash)
	}
}

// EL TOTAL QUE MANDE EL CLIENTE SE IGNORA (FR-003).
//
// Es la misma regla que `BuildOrder` con los precios: el servidor recalcula. Sin este caso, una
// pantalla con un bug —o alguien con curl— fija el fondo en lo que quiera y el arqueo del turno
// entero queda comparándose contra una cifra inventada.
func TestElFondoSaleDeLasPiezasYNoDeLoQueMandeElCliente(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_recalcula", "cajero")
	principal := registerID(t, st, "Caja principal")

	mentira := decimal.RequireFromString("999999")
	_, err := backoffice.OpenSession(ctx, principal, app.AperturaCmd{
		Piezas: piezasDe(t, st, "100", 2),
		Total:  &mentira,
		Motivo: "intento de colar un total",
	}, cajero)
	// Los dos caminos a la vez se rechazan ANTES de decidir cuál gana: son dos cifras del mismo
	// dinero y quedarse con cualquiera es inventar cuál era la buena.
	if !errors.Is(err, domain.ErrConteoAmbiguo) {
		t.Fatalf("mandar piezas y total a la vez debe rechazarse, dio: %v", err)
	}
}

// El camino manual existe, y exige decir por qué.
func TestAbrirCapturandoElTotalExigeMotivo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_manual", "cajero")
	principal := registerID(t, st, "Caja principal")

	monto := decimal.RequireFromString("2350")
	if _, err := backoffice.OpenSession(ctx, principal, app.AperturaCmd{Total: &monto}, cajero); !errors.Is(err, domain.ErrConteoSinExplicar) {
		t.Fatalf("capturar el total sin motivo debe rechazarse, dio: %v", err)
	}

	vista, err := backoffice.OpenSession(ctx, principal, app.AperturaCmd{
		Total:  &monto,
		Motivo: "había un billete que no está en la lista",
	}, cajero)
	if err != nil {
		t.Fatalf("con motivo debe pasar: %v", err)
	}
	if !vista.OpeningCash.Equal(monto) {
		t.Fatalf("el fondo capturado a mano quedó en %s, quiere %s", vista.OpeningCash, monto)
	}
}

// Un cajón vacío no es un error: una caja puede arrancar sin dinero.
func TestAbrirConElCajonVacioNoEsError(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_vacio", "cajero")
	principal := registerID(t, st, "Caja principal")

	vista, err := backoffice.OpenSession(ctx, principal, app.AperturaCmd{}, cajero)
	if err != nil {
		t.Fatalf("abrir con el cajón vacío debe pasar: %v", err)
	}
	if !vista.OpeningCash.IsZero() {
		t.Fatalf("el fondo quedó en %s y quiere 0", vista.OpeningCash)
	}
}

// UNA DENOMINACIÓN DE OTRA MONEDA NO SE SUMA (FR-011).
//
// La regla la declaraba el plan y no la hacía cumplir nadie. Un turno en pesos que recibe ids de
// otra moneda suma un fondo sin significado, y el arqueo del turno entero se compara después contra
// esa cifra.
func TestUnaDenominacionDeOtraMonedaSeRechaza(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_moneda", "cajero")
	principal := registerID(t, st, "Caja principal")

	// Una denominación de USD, que el catálogo admite aunque hoy no se siembre.
	var usd int64
	if err := st.Pool.QueryRow(ctx,
		`insert into cash_denominations (currency, value, is_coin, sort_key)
		 values ('USD', 100, false, 1) returning id`).Scan(&usd); err != nil {
		t.Fatalf("sembrar denominación en USD: %v", err)
	}

	_, err := backoffice.OpenSession(ctx, principal, app.AperturaCmd{
		Piezas: []app.PiezaCapturada{{DenominationID: usd, Pieces: 3}},
	}, cajero)
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("contar dólares en un turno en pesos debe rechazarse, dio: %v", err)
	}
}

// LA APERTURA ES ATÓMICA.
//
// Hoy `OpenSession` hace un solo insert y no necesita transacción; con el conteo son tres
// escrituras. Si la segunda o la tercera fallan después de que la primera comprometió, queda una
// sesión ABIERTA SIN CONTEO Y SIN MOTIVO —lo que FR-016 y SC-007 prohíben— y además la caja queda
// bloqueada: `one_open_session_per_register` no deja abrir otra hasta resolver la huérfana a mano.
//
// EL FALLO TIENE QUE SER TARDÍO, y esto costó una vuelta: la primera versión de este test mandaba
// una denominación borrada, pero eso lo rechaza `piezasConSuValor` ANTES de escribir nada — el test
// pasaba en verde con la transacción quitada, o sea que no probaba nada. Lo que sí falla tarde es
// mandar el MISMO renglón dos veces: el total se calcula sin protestar, la sesión y el conteo se
// escriben, y el segundo `insert` de la línea choca con `unique (count_id, denomination_id)`.
// Es además un fallo realista: una pantalla con un bug que manda el renglón repetido.
func TestUnaAperturaQueFallaNoDejaLaCajaBloqueada(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_atomico", "cajero")
	principal := registerID(t, st, "Caja principal")

	billete := denominacionMXN(t, st, "100")
	repetido := []app.PiezaCapturada{{DenominationID: billete, Pieces: 2}, {DenominationID: billete, Pieces: 3}}

	if _, err := backoffice.OpenSession(ctx, principal, app.AperturaCmd{Piezas: repetido}, cajero); err == nil {
		t.Fatal("abrir con el mismo renglón dos veces debía fallar")
	}

	// Y la caja tiene que seguir libre. Sin transacción, la sesión quedó escrita y esto falla con
	// ErrConflict: el operador no puede abrir el turno hasta que alguien entre a la base.
	if _, err := backoffice.OpenSession(ctx, principal, app.AperturaCmd{}, cajero); err != nil {
		t.Fatalf("la caja quedó bloqueada por una apertura que falló a medias: %v", err)
	}

	// Y no quedó un conteo huérfano de la sesión que no llegó a existir.
	var conteos int
	if err := st.Pool.QueryRow(ctx, `select count(*) from session_cash_counts`).Scan(&conteos); err != nil {
		t.Fatalf("contar conteos: %v", err)
	}
	if conteos != 1 {
		t.Fatalf("quedaron %d conteos y solo la apertura buena debería haber escrito uno", conteos)
	}
}

// aperturaAMano: abrir declarando el total sin contar, que es lo que hacían todos los tests antes
// de esta feature. Lleva motivo porque FR-016 lo exige: un arqueo sin desglose y sin explicación es
// una cifra que nadie puede auditar, y eso vale igual para un turno de prueba.
func aperturaAMano(total decimal.Decimal) app.AperturaCmd {
	return app.AperturaCmd{Total: &total, Motivo: "fondo declarado sin contar (fixture de prueba)"}
}

// UN RENGLÓN REPETIDO ES UNA CAPTURA INVÁLIDA, NO UN 500.
//
// Regresión. El caso lo dispara ya `TestUnaAperturaQueFallaNoDejaLaCajaBloqueada`, pero ese solo
// comprueba que la caja quede libre: el error salía como el `23505` crudo de
// `session_cash_count_lines_unicas` y `httpapi.Error` lo dejaba caer al default, así que el
// operador veía "el servidor se rompió" ante un dato que él puede corregir. El principio V exige
// 400.
//
// Y hay un segundo efecto que la base tapaba: `TotalDelConteo` SUMA los dos renglones —el dominio no
// ve ids, así que no puede detectarlo—, de modo que lo único que impedía un fondo inflado era la
// unique del esquema. Rechazarlo antes de escribir cierra las dos cosas.
func TestUnRenglonRepetidoSeRechazaComoCapturaInvalida(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_repetido", "cajero")
	principal := registerID(t, st, "Caja principal")

	billete := denominacionMXN(t, st, "100")
	repetido := []app.PiezaCapturada{{DenominationID: billete, Pieces: 2}, {DenominationID: billete, Pieces: 3}}

	_, err := backoffice.OpenSession(ctx, principal, app.AperturaCmd{Piezas: repetido}, cajero)
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("el renglón repetido dio %v y tiene que ser ErrValidation: si no, sale como 500 "+
			"y quien lo puede arreglar es justo el que no se entera", err)
	}

	// Y no escribió nada: ni el fondo de $500 que habrían sumado los dos renglones.
	var sesiones int
	if err := st.Pool.QueryRow(ctx, `select count(*) from register_sessions`).Scan(&sesiones); err != nil {
		t.Fatalf("contar sesiones: %v", err)
	}
	if sesiones != 0 {
		t.Fatalf("quedaron %d sesiones: el rechazo tiene que ser ANTES de abrir el turno", sesiones)
	}
}

// LAS DOS TABLETAS ABREN LA MISMA CAJA AL MISMO TIEMPO Y LA SEGUNDA VE UN CONFLICTO, NO UN 500.
//
// Regresión, y no es hipotético: las dos tabletas comparten cuenta, así que las dos pueden tener la
// caja cerrada en pantalla y tocar "Abrir". `GetOpenSessionByRegister` es un pre-check
// read-then-insert, no atómico: en la carrera las dos pasan el pre-check y la segunda choca con
// `one_open_session_per_register`. Ese `23505` no lo traducía nadie —la traducción que existía
// apunta a `session_cash_counts_un_momento`, que en la apertura es inalcanzable porque la sesión y
// su conteo son nuevos— y subía como 500.
//
// El camino secuencial ya devuelve ErrConflict, y la pantalla ya sabe pintarlo: lo que falta es que
// la carrera termine igual que la fila.
func TestDosAperturasSimultaneasDejanUnConflictoYNoUn500(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	cajero := makeUser(t, st, "cajero_carrera", "cajero")
	principal := registerID(t, st, "Caja principal")

	const intentos = 2
	errs := make([]error, intentos)
	var wg sync.WaitGroup
	arranque := make(chan struct{})
	for i := range intentos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc := app.NewBackofficeService(st, clock)
			<-arranque // que salgan juntas, no en fila
			_, errs[i] = svc.OpenSession(ctx, principal, app.AperturaCmd{}, cajero)
		}()
	}
	close(arranque)
	wg.Wait()

	abiertas, conflictos := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			abiertas++
		case errors.Is(err, domain.ErrConflict):
			conflictos++
		default:
			t.Fatalf("la apertura perdedora dio %v: tiene que ser ErrConflict, no el error crudo de "+
				"Postgres — con dos tabletas compartiendo cuenta esto pasa en un turno normal", err)
		}
	}
	if abiertas != 1 || conflictos != 1 {
		t.Fatalf("%d aperturas y %d conflictos: la caja tiene que quedar abierta exactamente una vez",
			abiertas, conflictos)
	}
}
