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
)

// DEVOLVER EL EFECTIVO DE UNA APP SACA DINERO DEL CAJÓN, Y EL ARQUEO TIENE QUE ENTERARSE.
//
// Cruce entre la spec 007 y la 015. La 007 decidió que una devolución sale del cajón "cuando el
// cobro fue en efectivo" y dejó fuera a las plataformas porque "ese dinero nunca estuvo en la
// caja". Era cierto cuando se escribió: solo existía el efectivo del mostrador.
//
// Con la 015 dejó de serlo. «Didi efectivo» tiene `kind = 'plataforma'` pero
// `affects_cash_drawer = true`: son billetes que el repartidor del local trajo y que están en el
// mismo montón que todo lo demás. El reparto de la devolución decide por `kind = 'efectivo'`, así
// que al devolverle $135 a un cliente de Didi el operador los saca del cajón y el sistema no
// registra ninguna salida — el arqueo sigue esperando ese dinero y el corte cierra con un faltante
// de $135 que nadie puede explicar.
//
// Es exactamente la clase de defecto que la 015 vino a cerrar, entrando por la otra puerta.
func TestDevolverElEfectivoDeUnaAppSaleDelCajon(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	orders := app.NewOrdersService(st, clock)
	cajero := makeUser(t, st, "cajero_devol_app", "cajero")

	principal := registerID(t, st, "Caja principal")
	if _, err := backoffice.OpenSession(ctx, principal, aperturaAMano(decimal.RequireFromString("500")), cajero); err != nil {
		t.Fatalf("abrir la caja: %v", err)
	}
	didiEfectivo := paymentMethodID(t, st, "Didi efectivo")
	didi := platformID(t, st, defaultCompanyID, "Didi")
	prod := makeProduct(t, st, "Crepa devuelta", decimal.RequireFromString("135"), false)

	cobrado := decimal.RequireFromString("135") // lo que el repartidor trajo en billetes
	pedido, err := crearYCobrar(t, ctx, orders, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "domicilio", DeliveryPlatformID: &didi, OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: didiEfectivo, Amount: cobrado}},
	})
	if err != nil {
		t.Fatalf("cobrar el pedido de Didi en efectivo: %v", err)
	}
	entregarPendientes(t, st)

	antesDelArqueo, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister antes: %v", err)
	}
	antesSalidas := salidasDeCaja(t, st)

	if err := orders.Devolver(ctx, app.DevolucionCmd{
		OrderID: pedido.ID, Monto: cobrado, Motivo: "el cliente rechazó el pedido", ActorID: cajero,
	}); err != nil {
		t.Fatalf("devolver el pedido de la app: %v", err)
	}

	if salio := salidasDeCaja(t, st).Sub(antesSalidas); !salio.Equal(cobrado) {
		t.Fatalf("del cajón salieron %s y el operador devolvió %s en billetes: «Didi efectivo» va al cajón (affects_cash_drawer), así que su devolución tiene que registrar la salida",
			salio, cobrado)
	}

	despues, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister después: %v", err)
	}
	bajo := antesDelArqueo.Drawer.Expected.Sub(*despues.Drawer.Expected)
	if !bajo.Equal(cobrado) {
		t.Fatalf("el cajón esperaba %s y ahora espera %s: bajó %s en vez de %s. El corte cerraría con un faltante inventado del tamaño de la devolución",
			antesDelArqueo.Drawer.Expected, despues.Drawer.Expected, bajo, cobrado)
	}
}

// APAGAR «VA AL CAJÓN» A MEDIA JORNADA NO PUEDE BORRAR DEL ARQUEO EL DINERO QUE YA ESTÁ EN EL CAJÓN.
//
// Hermano del anterior, por el otro lado. Hasta la 015 nadie podía mover este interruptor desde la
// aplicación, así que el camino no existía. Ahora sí: el dueño lo apaga porque de hoy en adelante
// el repartidor de la app se lleva el dinero, y el esperado del cajón baja en todo lo que ese
// método YA cobró — billetes que están físicamente en el cajón y que el arqueo deja de pedir. El
// corte cierra con un sobrante fantasma y nadie lo audita, porque un sobrante no duele.
//
// Es la misma falla que `TestDesactivarUnMetodoNoDesapareceElDineroQueYaCobro` cierra para
// `is_active`, entrando por el otro interruptor.
func TestApagarVaAlCajonNoBorraElDineroQueYaEstaEnElCajon(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_apaga_cajon", "cajero")
	principal, _, didiEfectivo, esperado := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)

	_, err := backoffice.UpdatePaymentMethod(ctx, int(didiEfectivo), app.MetodoDePagoCmd{
		AffectsCashDrawer: ptrBool(false),
	})
	if !errors.Is(err, domain.ErrCajonConDineroDelTurno) {
		t.Fatalf("apagar «va al cajón» con dinero de ese método en el turno dio %v: tiene que rechazarse", err)
	}

	// Y lo que protege el rechazo: el esperado no se movió.
	abierta, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	if !abierta.Drawer.Expected.Equal(esperado) {
		t.Fatalf("el cajón esperaba %s y ahora espera %s: los billetes que la app ya trajo siguen en el cajón",
			esperado, abierta.Drawer.Expected)
	}

	// El mismo interruptor SÍ se mueve en un método que no ha cobrado en el turno: lo que se
	// rechaza es mover dinero de sitio, no configurar.
	rappi := paymentMethodID(t, st, "Rappi efectivo")
	if _, err := backoffice.UpdatePaymentMethod(ctx, int(rappi), app.MetodoDePagoCmd{
		AffectsCashDrawer: ptrBool(false),
	}); err != nil {
		t.Fatalf("apagar «va al cajón» de un método que no cobró nada: %v", err)
	}
}
