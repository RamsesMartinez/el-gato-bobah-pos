//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// ARQUEO CIEGO (spec 015, US3).
//
// Quien cuenta el cajón no ve lo que el sistema espera, y la diferencia aparece después de
// confirmar. Es un control contra que alguien acomode lo que declara para que cuadre.
//
// Enciéndelo y FR-005 de la spec 003 —"la diferencia se ve antes de confirmar"— queda enmendado:
// las dos reglas protegen cosas distintas y por eso esto es un ajuste del negocio.

// prenderCiego enciende el interruptor por el MISMO camino que la pantalla: el servicio de ajustes,
// no un update a la columna. Un test que prende el interruptor por SQL no prueba que el endpoint lo
// sepa guardar.
func prenderCiego(t *testing.T, ctx context.Context, settings *app.SettingsService) {
	t.Helper()
	cur, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("leer los ajustes: %v", err)
	}
	info := domain.BusinessInfo{Name: cur.BusinessName, Address: cur.Address, Phone: cur.Phone,
		HeaderNote: cur.HeaderNote, FooterNote: cur.FooterNote}
	print := domain.PrintSettings{
		AutoPrintOnClose: cur.AutoPrintOnClose, PrintFreeModifiers: cur.PrintFreeModifiers,
		PrintKitchenTicket: cur.PrintKitchenTicket, CorteDeVista: cur.CorteDeVista,
		FolioScheme: cur.FolioScheme, KitchenCanCharge: cur.KitchenCanCharge,
		BlindCashCount: true,
	}
	ident := domain.IdentitySettings{PinOnlyUnlock: cur.PinOnlyUnlock,
		LockAfterSeconds: cur.LockAfterSeconds, SessionHours: cur.SessionHours}
	if _, err := settings.SetBusinessInfo(ctx, info, print, ident, cur.Timezone, 1); err != nil {
		t.Fatalf("encender el arqueo ciego: %v", err)
	}
}

// LO QUE LA PANTALLA NO DEBE MOSTRAR, NO SE LE MANDA.
//
// Ocultarlo en el cliente deja la cifra en la respuesta, legible con las herramientas del
// navegador: el control dejaría de serlo. Y aplica a TODOS los métodos, no solo al efectivo — ver
// el esperado de la tarjeta permite el mismo acomodo.
func TestConArqueoCiegoElEsperadoNoViaja(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	cajero := makeUser(t, st, "cajero_ciego", "cajero")
	principal, _, _, _ := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)

	// Con el interruptor APAGADO —el default— todo viaja como siempre.
	abierta, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	if abierta.Drawer.Expected == nil {
		t.Fatal("con el arqueo ciego apagado el esperado tiene que viajar: es FR-005 de la 003")
	}

	prenderCiego(t, ctx, settings)

	// LOS DOS ENDPOINTS. `GET /cash-sessions/{id}` está abierto a rol cajero y acepta el id del
	// turno abierto: nulificar solo el de `/current` dejaría la cifra a un request de distancia.
	abierta, err = backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister con ciego: %v", err)
	}
	if abierta.Drawer.Expected != nil {
		t.Fatalf("el esperado del cajón viajó con el arqueo ciego: %v", abierta.Drawer.Expected)
	}
	for _, m := range abierta.Totals {
		if m.Expected != nil {
			t.Fatalf("el esperado de «%s» viajó con el arqueo ciego: %v — ver el de la tarjeta permite el mismo acomodo que ver el del efectivo",
				m.Name, m.Expected)
		}
	}

	// EL OTRO CAMINO: `GET /cash-sessions/{id}` acepta el id del turno abierto y está abierto a rol
	// cajero, así que no basta con nulificar `/current`.
	//
	// Hoy no filtra nada por una razón distinta de la que uno esperaría, y por eso se afirma en vez
	// de recorrer: `register_session_totals` se escribe AL CERRAR, así que el detalle de un turno
	// abierto trae cero renglones y ningún arqueo. Recorrer `detalle.Totals` buscando esperados
	// pasaría en verde sobre una lista vacía — un test que no puede fallar. Lo que se fija aquí es
	// la forma real: si algún día este endpoint empieza a calcular el turno vivo, esta afirmación
	// se rompe y obliga a mirar el ocultamiento, que por eso se deja puesto en `SessionDetail`.
	detalle, err := backoffice.SessionDetail(ctx, abierta.ID)
	if err != nil {
		t.Fatalf("SessionDetail del turno abierto: %v", err)
	}
	if len(detalle.Totals) != 0 || detalle.Drawer != nil {
		t.Fatalf("el detalle del turno abierto empezó a traer cifras (%d métodos, arqueo %v): revisa que el ocultamiento del arqueo ciego las cubra",
			len(detalle.Totals), detalle.Drawer)
	}
}

// Y AUN ASÍ SE SIGUE SABIENDO QUÉ FALTA CAPTURAR.
//
// Es el borde que la revisión de arquitectura encontró: si la pantalla dedujera lo que falta de si
// el esperado es cero, con el esperado en null concluiría que no falta nada y el botón de cerrar
// quedaría habilitado con la pantalla en blanco. El faltante inventado de $1,662 por la puerta de
// atrás.
func TestConArqueoCiegoElServidorSigueDiciendoQueFalta(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	cajero := makeUser(t, st, "cajero_ciego_falta", "cajero")
	principal, _, _, _ := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)
	prenderCiego(t, ctx, settings)

	abierta, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	if !abierta.Drawer.RequiresCount {
		t.Fatal("con el arqueo ciego el cajón dejó de exigir conteo: el cierre se firmaría vacío")
	}
	// Y el que no vendió nada sigue sin exigir cifra.
	//
	// `encontrado` no sobra: sin él, el default de `exige` coincide con lo que el caso espera, así
	// que el día que la tarjeta desapareciera de `Totals` —un cambio en el filtro de
	// `ExpectedByMethodForSession`, por ejemplo— el `for` no encontraría nada y el test pasaría en
	// verde sin haber comprobado nada.
	tarjeta := paymentMethodID(t, st, "Tarjeta débito")
	var exige, encontrado bool
	for _, m := range abierta.Totals {
		if m.MethodID == int(tarjeta) {
			exige, encontrado = m.RequiresEntry, true
		}
	}
	if !encontrado {
		t.Fatalf("«Tarjeta débito» no aparece entre los %d métodos del turno: el caso no se probó", len(abierta.Totals))
	}
	if exige {
		t.Fatal("la tarjeta no vendió nada en este turno y aun así exige captura")
	}
}

// CERRADO EL TURNO, LAS CIFRAS VUELVEN: es cuando la diferencia se muestra.
func TestConArqueoCiegoLasCifrasVuelvenAlCerrar(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	cajero := makeUser(t, st, "cajero_ciego_cerrado", "cajero")
	principal, _, _, _ := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)
	prenderCiego(t, ctx, settings)

	cerrada, err := backoffice.CloseSession(ctx, principal, cajero, app.CierreCmd{
		Piezas: piezasDe(t, st, "500", 1, "100", 2, "20", 4, "5", 1),
	})
	if err != nil {
		t.Fatalf("cerrar a ciegas: %v", err)
	}
	if cerrada.Drawer == nil || cerrada.Drawer.Difference == nil {
		t.Fatal("al confirmar el cierre la diferencia tiene que aparecer: el control es no verla ANTES")
	}

	detalle, err := backoffice.SessionDetail(ctx, cerrada.ID)
	if err != nil {
		t.Fatalf("SessionDetail: %v", err)
	}
	if detalle.Drawer == nil || detalle.Drawer.Expected == nil {
		t.Fatal("el corte CERRADO tiene que traer su esperado: sin él no se puede auditar")
	}
}

// NO BASTA CON NULIFICAR `expected`: LO DERIVADO LO RECONSTRUYE EXACTO.
//
// El hallazgo de la auditoría, y es el que decide si esto es un control o un adorno. La misma
// respuesta que pone el esperado en null trae el desglose por método y lo que cobró cada cajero, y
// la pantalla los pinta ARRIBA de la tabla del cierre. Quien cuenta no necesita las herramientas
// del navegador: suma cuatro renglones contiguos.
//
// Medido: fondo 500 + neto 0 + Ventas del mostrador 200 + Ventas de Didi efectivo 135 = 835, que
// es exactamente el esperado oculto. `methodIds` hasta dice qué renglones sumar.
//
// Este test no busca claves llamadas `expected` —la fuga viaja en `amount` y en `total`—: reconstruye
// la cifra como lo haría quien la quiere, y falla si le sale.
func TestConArqueoCiegoLoDerivadoNoReconstruyeElEsperado(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	cajero := makeUser(t, st, "cajero_ciego_derivado", "cajero")
	principal, _, _, esperado := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)
	prenderCiego(t, ctx, settings)

	abierta, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	delCajon := map[string]bool{}
	for _, m := range abierta.Totals {
		for _, id := range abierta.Drawer.MethodIDs {
			if m.MethodID == id {
				delCajon[m.Name] = true
			}
		}
	}

	// Así es como se reconstruye: el fondo, el neto y los ingresos de los métodos del cajón.
	reconstruido := abierta.OpeningCash.Add(abierta.NetMovements)
	for _, ing := range abierta.Breakdown.Ingresos {
		if delCajon[ing.Method] {
			reconstruido = reconstruido.Add(ing.Total)
		}
	}
	if reconstruido.Equal(esperado) {
		t.Fatalf("con el arqueo ciego encendido, el desglose reconstruye el esperado exacto (%s): quien cuenta lo lee en pantalla sin abrir nada",
			reconstruido)
	}

	// Y por el otro camino: lo que cobró cada cajero en efectivo.
	porCajero := abierta.OpeningCash.Add(abierta.NetMovements)
	for _, c := range abierta.Cashiers {
		porCajero = porCajero.Add(c.Cash)
	}
	if porCajero.Equal(esperado) {
		t.Fatalf("el desglose por cajero reconstruye el esperado exacto (%s)", porCajero)
	}
}

// Y EL MOVIMIENTO DE CAJA ES OTRO CAMINO DE LECTURA.
//
// `POST /cash-sessions/movements` devuelve la vista del turno y está abierto a rol cajero: registrar
// una entrada de un centavo devolvía el esperado completo. El ocultamiento vivía en cada llamador y
// éste se lo saltó — que es exactamente por qué ahora vive en un solo lugar.
func TestConArqueoCiegoUnMovimientoNoDevuelveElEsperado(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	cajero := makeUser(t, st, "cajero_ciego_movimiento", "cajero")
	principal, _, _, _ := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)
	prenderCiego(t, ctx, settings)

	vista, err := backoffice.RecordCashMovement(ctx, principal, "entrada",
		decimal.RequireFromString("0.01"), "cambio para el turno", cajero)
	if err != nil {
		t.Fatalf("registrar el movimiento: %v", err)
	}
	if vista.Drawer != nil && vista.Drawer.Expected != nil {
		t.Fatalf("registrar un movimiento de un centavo devolvió el esperado del cajón: %v", vista.Drawer.Expected)
	}
	for _, m := range vista.Totals {
		if m.Expected != nil {
			t.Fatalf("registrar un movimiento devolvió el esperado de «%s»: %v", m.Name, m.Expected)
		}
	}
}
