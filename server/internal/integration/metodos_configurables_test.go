//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// CONFIGURAR LOS MÉTODOS DE PAGO (spec 015, US2).
//
// Las dos columnas existen desde siempre y ningún endpoint las escribía: apagar un método o decidir
// si su efectivo llega al cajón exigía entrar a la base. Lo segundo es la respuesta a que el reparto
// de un pedido de app en efectivo lo hace a veces gente del local —y el dinero regresa al cajón— y a
// veces el repartidor de la plataforma, que se lo lleva.

func ptrBool(v bool) *bool { return &v }

// UN PATCH DE UN INTERRUPTOR NO APAGA LOS OTROS DOS.
//
// El handler viejo declaraba el campo como `bool` sin puntero, donde un campo ausente y un `false`
// explícito son indistinguibles. Copiado tal cual para tres interruptores, un `PATCH` que solo
// quería desactivar un método le habría sacado el dinero del arqueo — plata movida por el tipo de
// dato y no por el negocio.
func TestUnPatchDeUnInterruptorNoPisaLosOtros(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	didiEfectivo := int(paymentMethodID(t, st, "Didi efectivo"))

	// Nace activo y con su efectivo en el cajón.
	antes, err := backoffice.UpdatePaymentMethod(ctx, didiEfectivo, app.MetodoDePagoCmd{})
	if err != nil {
		t.Fatalf("un PATCH vacío no debería fallar al leer: %v", err)
	}
	if !antes.IsActive || !antes.AffectsCashDrawer {
		t.Fatalf("el fixture esperaba el método activo y en el cajón: %+v", antes)
	}

	despues, err := backoffice.UpdatePaymentMethod(ctx, didiEfectivo, app.MetodoDePagoCmd{
		IsActive: ptrBool(false),
	})
	if err != nil {
		t.Fatalf("desactivar el método: %v", err)
	}
	if despues.AffectsCashDrawer != antes.AffectsCashDrawer {
		t.Fatal("desactivar el método le cambió si su efectivo va al cajón: el dinero se movió por el tipo de dato, no por el negocio")
	}
	if despues.AutoDeclare != antes.AutoDeclare {
		t.Fatal("desactivar el método le cambió el auto-declarar")
	}
	if despues.IsActive {
		t.Fatal("el método sigue activo")
	}
}

// EL EFECTIVO DEL MOSTRADOR NO SALE DEL CAJÓN.
//
// Es el único valor del interruptor que no tiene sentido: los billetes que el cliente pone en el
// mostrador están en el cajón por definición. Apagarlo dejaría el fondo de apertura y los
// movimientos de caja fuera del esperado, y el arqueo se compararía contra una cifra que no incluye
// el dinero con el que abrió el turno.
func TestElEfectivoDelMostradorNoPuedeSalirDelCajon(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	efectivo := int(paymentMethodID(t, st, "Efectivo"))

	_, err := backoffice.UpdatePaymentMethod(ctx, efectivo, app.MetodoDePagoCmd{
		AffectsCashDrawer: ptrBool(false),
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("sacar el efectivo del mostrador del cajón dio %v y tiene que ser ErrValidation", err)
	}
}

// UN MÉTODO DESACTIVADO DEJA DE OFRECERSE PARA COBRAR.
func TestUnMetodoDesactivadoDejaDeOfrecerse(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	transferencia := int(paymentMethodID(t, st, "Transferencia SPEI"))

	if _, err := backoffice.UpdatePaymentMethod(ctx, transferencia, app.MetodoDePagoCmd{
		IsActive: ptrBool(false),
	}); err != nil {
		t.Fatalf("desactivar: %v", err)
	}

	metodos, err := backoffice.PaymentMethods(ctx)
	if err != nil {
		t.Fatalf("PaymentMethods: %v", err)
	}
	for _, m := range metodos {
		if m.ID == transferencia {
			t.Fatal("un método desactivado se sigue ofreciendo para cobrar")
		}
	}
}

// DESACTIVAR UN MÉTODO NO DESAPARECE EL DINERO QUE YA COBRÓ.
//
// Es la falla más grave posible en una caja, porque cuadra por construcción y nadie la ve: el
// esperado baja en lo que ese método ya recibió y el arqueo se compara contra una cifra más chica.
// Hasta esta feature nadie podía apagar un método desde la aplicación, así que el camino no existía.
func TestDesactivarUnMetodoNoDesapareceElDineroQueYaCobro(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_apagado", "cajero")
	principal, _, didiEfectivo, esperado := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)

	if _, err := backoffice.UpdatePaymentMethod(ctx, int(didiEfectivo), app.MetodoDePagoCmd{
		IsActive: ptrBool(false),
	}); err != nil {
		t.Fatalf("desactivar el método a media jornada: %v", err)
	}

	abierta, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	if abierta.Drawer == nil || !abierta.Drawer.Expected.Equal(esperado) {
		t.Fatalf("el cajón espera %v y el dinero ya cobrado son %s: apagar un método no puede borrarlo del arqueo",
			abierta.Drawer.Expected, esperado)
	}
}

// UN CORTE YA CERRADO NO CAMBIA AL MOVER LOS INTERRUPTORES.
func TestUnCorteCerradoNoCambiaAlMoverLosInterruptores(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_corte_firme", "cajero")
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

	for _, cmd := range []app.MetodoDePagoCmd{
		{IsActive: ptrBool(false)},
		{AffectsCashDrawer: ptrBool(false)},
	} {
		if _, err := backoffice.UpdatePaymentMethod(ctx, int(didiEfectivo), cmd); err != nil {
			t.Fatalf("mover un interruptor: %v", err)
		}
	}

	despues, err := backoffice.SessionDetail(ctx, cerrada.ID)
	if err != nil {
		t.Fatalf("SessionDetail después: %v", err)
	}
	if !antes.Drawer.Expected.Equal(*despues.Drawer.Expected) ||
		len(antes.Drawer.MethodIDs) != len(despues.Drawer.MethodIDs) {
		t.Fatalf("el corte cambió: antes esperaba %v con %v y ahora %v con %v",
			antes.Drawer.Expected, antes.Drawer.MethodIDs, despues.Drawer.Expected, despues.Drawer.MethodIDs)
	}
	// Y sus cifras por método, intactas.
	for i := range antes.Totals {
		if !antes.Totals[i].Declared.Equal(despues.Totals[i].Declared) {
			t.Fatalf("el declarado de %s cambió de %s a %s",
				antes.Totals[i].Name, antes.Totals[i].Declared, despues.Totals[i].Declared)
		}
	}
}

// UN MÉTODO QUE NO EXISTE NO SE CONFIGURA.
func TestConfigurarUnMetodoQueNoExisteEs404(t *testing.T) {
	st := newTestStore(t)
	backoffice := app.NewBackofficeService(st, clock)
	_, err := backoffice.UpdatePaymentMethod(context.Background(), 9999, app.MetodoDePagoCmd{
		IsActive: ptrBool(false),
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("configurar un método inexistente dio %v y tiene que ser ErrNotFound", err)
	}
}

// EL AUTO-DECLARAR SIGUE SIN PODER ENCENDERSE EN UN MÉTODO DE CAJÓN.
//
// Es la regla que ya existía y que esta feature no puede aflojar: auto-declarar el efectivo dejaría
// el corte sin forma de detectar un faltante, porque el servidor declararía lo que él mismo espera.
func TestElAutoDeclararSigueProhibidoEnUnMetodoDeCajon(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	efectivo := int(paymentMethodID(t, st, "Efectivo"))

	_, err := backoffice.UpdatePaymentMethod(ctx, efectivo, app.MetodoDePagoCmd{
		AutoDeclare: ptrBool(true),
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("auto-declarar el efectivo dio %v y tiene que seguir prohibido", err)
	}
	// Pero sí se puede en uno que no toca el cajón.
	tarjeta := int(paymentMethodID(t, st, "Tarjeta débito"))
	if _, err := backoffice.UpdatePaymentMethod(ctx, tarjeta, app.MetodoDePagoCmd{
		AutoDeclare: ptrBool(true),
	}); err != nil {
		t.Fatalf("auto-declarar la tarjeta: %v", err)
	}
}

// DESACTIVAR «EFECTIVO» NO PUEDE BORRAR EL FONDO DE APERTURA DEL ARQUEO.
//
// Borde que el spec dejó escrito como pregunta abierta —"se desactiva el único método de efectivo,
// ¿el cierre sigue pidiendo contar el cajón?"— y que nadie había respondido ni en código ni por
// escrito. Hasta esta feature no había forma de apagarlo desde la aplicación.
//
// El fondo de apertura y los movimientos de caja se suman al renglón del ÚNICO método de tipo
// efectivo. Si ese renglón desaparece del corte porque el método está apagado y no cobró nada en
// el turno, el fondo desaparece con él: el cajón pasa a esperar $0 con los billetes de la apertura
// adentro, y el corte cierra con un sobrante del tamaño del fondo.
func TestDesactivarElEfectivoNoBorraElFondoDelArqueo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_sin_efectivo", "cajero")

	principal := registerID(t, st, "Caja principal")
	fondo := decimal.RequireFromString("500")
	if _, err := backoffice.OpenSession(ctx, principal, aperturaAMano(fondo), cajero); err != nil {
		t.Fatalf("abrir la caja con fondo: %v", err)
	}
	efectivo := int(paymentMethodID(t, st, "Efectivo"))
	if _, err := backoffice.UpdatePaymentMethod(ctx, efectivo, app.MetodoDePagoCmd{
		IsActive: ptrBool(false),
	}); err != nil {
		t.Fatalf("desactivar el efectivo del mostrador: %v", err)
	}

	abierta, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	if abierta.Drawer == nil {
		t.Fatal("el cajón desapareció al apagar el método de efectivo: los $500 del fondo siguen ahí adentro")
	}
	if !abierta.Drawer.Expected.Equal(fondo) {
		t.Fatalf("el cajón espera %s y el fondo con el que abrió son %s: apagar un método no saca los billetes del cajón",
			abierta.Drawer.Expected, fondo)
	}
}

// SACAR UN MÉTODO DEL CAJÓN Y AUTO-DECLARARLO EN EL MISMO REQUEST NO PUEDE VALER.
//
// El bypass que encontró la auditoría de seguridad. La regla que el código llama innegociable
// —"un método cuyo dinero se cuenta en el cajón no se auto-declara"— se evaluaba solo contra el
// estado resultante, así que apagar «va al cajón» en el MISMO PATCH la satisfacía.
//
// Verificado: con eso puesto antes del primer cobro, los $135 de Didi en efectivo entran, el cajón
// espera 500 —sin ellos— y el método reporta `autoDeclare` con `requiresEntry` en falso. El cierre
// contando solo el fondo da diferencia $0.00 en todo. Y en el catálogo el método queda idéntico a
// «Didi en línea»: no quedaba ningún marcador de que su dinero era efectivo.
//
// Por eso el marcador ahora es propio (`is_cash`) y no el mismo interruptor que dice dónde cae el
// dinero: un método de efectivo no se auto-declara, lo reparta quien lo reparta.
func TestSacarDelCajonYAutoDeclararEnElMismoRequestSeRechaza(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	didiEfectivo := int(paymentMethodID(t, st, "Didi efectivo"))

	_, err := backoffice.UpdatePaymentMethod(ctx, didiEfectivo, app.MetodoDePagoCmd{
		AffectsCashDrawer: ptrBool(false),
		AutoDeclare:       ptrBool(true),
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("sacar del cajón y auto-declarar en el mismo PATCH dio %v: tiene que rechazarse", err)
	}

	// Y tampoco en dos pasos: el método sigue siendo de efectivo aunque su dinero deje de caer en
	// nuestro cajón, y auto-declarar efectivo hace que un faltante sea indetectable.
	if _, err := backoffice.UpdatePaymentMethod(ctx, didiEfectivo, app.MetodoDePagoCmd{
		AffectsCashDrawer: ptrBool(false),
	}); err != nil {
		t.Fatalf("sacar del cajón un método que no ha cobrado: %v", err)
	}
	_, err = backoffice.UpdatePaymentMethod(ctx, didiEfectivo, app.MetodoDePagoCmd{
		AutoDeclare: ptrBool(true),
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("auto-declarar un método de efectivo fuera del cajón dio %v: sigue siendo efectivo", err)
	}
}

// Y AL REVÉS: UN MÉTODO QUE NO ES DE EFECTIVO NO PUEDE ENTRAR AL CAJÓN.
//
// Marcar «Tarjeta débito» como que va al cajón sumaría al esperado dinero que nunca son billetes,
// y el arqueo pediría contar algo que está en la terminal.
func TestUnMetodoQueNoEsEfectivoNoEntraAlCajon(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	tarjeta := int(paymentMethodID(t, st, "Tarjeta débito"))

	_, err := backoffice.UpdatePaymentMethod(ctx, tarjeta, app.MetodoDePagoCmd{
		AffectsCashDrawer: ptrBool(true),
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("meter la tarjeta al cajón dio %v: ese dinero nunca son billetes", err)
	}
}

// APAGAR UN MÉTODO NO PUEDE BORRARLO DE LA PANTALLA QUE TIENE SU INTERRUPTOR.
//
// Era una puerta de un solo sentido: la tabla de ajustes se pintaba con la misma lista que ofrece
// el POS para cobrar, que filtra los apagados. Un toque en «Activo» sobre «Efectivo» —el mismo
// dedo que falla por milímetros que el código ya nombra— dejaba al mostrador sin cobrar en
// efectivo y sin camino en la aplicación para volver a encenderlo.
func TestUnMetodoApagadoSigueEnLaListaDeAjustes(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	efectivo := int(paymentMethodID(t, st, "Efectivo"))

	if _, err := backoffice.UpdatePaymentMethod(ctx, efectivo, app.MetodoDePagoCmd{
		IsActive: ptrBool(false),
	}); err != nil {
		t.Fatalf("apagar el efectivo: %v", err)
	}

	// Para cobrar ya no se ofrece.
	paraCobrar, err := backoffice.PaymentMethods(ctx)
	if err != nil {
		t.Fatalf("PaymentMethods: %v", err)
	}
	for _, m := range paraCobrar {
		if m.ID == efectivo {
			t.Fatal("un método apagado se sigue ofreciendo para cobrar")
		}
	}

	// Pero en ajustes sigue, con su interruptor en falso: es el único camino para reactivarlo.
	todos, err := backoffice.AllPaymentMethods(ctx)
	if err != nil {
		t.Fatalf("AllPaymentMethods: %v", err)
	}
	var encontrado bool
	for _, m := range todos {
		if m.ID == efectivo {
			encontrado = true
			if m.IsActive {
				t.Fatal("la lista de ajustes dice que el método está activo y acaba de apagarse")
			}
		}
	}
	if !encontrado {
		t.Fatalf("«Efectivo» desapareció de la lista de ajustes (%d métodos): no hay forma de volver a encenderlo", len(todos))
	}
}

// UNA EMPRESA NUEVA TIENE QUE PODER NACER, Y SUS MÉTODOS QUEDAR COHERENTES.
//
// Lo encontró el `check` de la 0067 al sembrar la segunda empresa de los tests de aislamiento:
// `SeedBasePaymentMethods` insertaba «Efectivo» con `affects_cash_drawer` y sin `is_cash`, que la
// columna nueva deja en falso — y la restricción, con razón, lo rechazaba. Con eso puesto, **crear
// una empresa nueva fallaba**, que es justo el camino por el que este producto se vende a otro
// negocio.
//
// El test afirma las dos cosas: que la empresa se crea y que sus métodos nacen con la forma que el
// arqueo espera, en vez de solo comprobar que no truene.
func TestUnaEmpresaNuevaNaceConSusMetodosCoherentes(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	otra := makeCompany(t, st, "nueva-para-cobrar")

	filas, err := st.Pool.Query(ctx,
		`select name, kind, is_cash, affects_cash_drawer, auto_declare
		   from payment_methods where company_id = $1 order by sort_key`, otra)
	if err != nil {
		t.Fatalf("leer los métodos de la empresa nueva: %v", err)
	}
	defer filas.Close()

	var cuantos, enBilletes int
	for filas.Next() {
		var nombre, tipo string
		var esEfectivo, alCajon, automatico bool
		if err := filas.Scan(&nombre, &tipo, &esEfectivo, &alCajon, &automatico); err != nil {
			t.Fatalf("scan: %v", err)
		}
		cuantos++
		if esEfectivo {
			enBilletes++
			if !alCajon {
				t.Errorf("«%s» se cobra en billetes y no entra al cajón: el fondo de apertura se quedaría sin dueño", nombre)
			}
			if automatico {
				t.Errorf("«%s» nace auto-declarado siendo efectivo: un faltante nunca aparecería", nombre)
			}
		} else if alCajon {
			t.Errorf("«%s» entra al cajón sin cobrarse en billetes", nombre)
		}
	}
	if cuantos == 0 {
		t.Fatal("la empresa nueva nació sin métodos de cobro: no podría cobrar nada")
	}
	if enBilletes != 1 {
		t.Fatalf("la empresa nueva nació con %d métodos de efectivo y tiene que ser exactamente uno: el fondo se sumaría una vez por cada uno", enBilletes)
	}
}
