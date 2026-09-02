//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"time"

	"uuid"

	"github.com/shopspring/decimal"
)

// EL AJUSTE NACE EN MEDIANOCHE Y RECHAZA CUALQUIER OTRO VALOR.
//
// El default es la medianoche porque es lo que un operador espera sin que nadie se lo explique, y el
// único de los tres que no depende de que alguien se acuerde de cerrar la caja.
//
// Y un valor desconocido se RECHAZA en vez de caer al default: un ajuste que acepta cualquier cosa y
// se comporta como el default deja al dueño creyendo que configuró algo que no configuró.
//
// Con dos empresas: el ajuste de una no puede tocar el de la otra.
func TestElCorteDeVistaNaceEnMedianocheYValida(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	admin := makeUser(t, st, "admin_corte", "admin")

	antes, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if antes.CorteDeVista != domain.CorteMedianoche {
		t.Errorf("el corte nace en %q, quiere %q", antes.CorteDeVista, domain.CorteMedianoche)
	}

	info := domain.BusinessInfo{Name: antes.BusinessName, Address: antes.Address, Phone: antes.Phone}
	guardar := func(modo string) error {
		ident := domain.DefaultIdentity()
		_, err := settings.SetBusinessInfo(ctx, info, domain.PrintSettings{CorteDeVista: modo},
			ident, antes.Timezone, admin)
		return err
	}

	for _, modo := range []string{domain.CorteMedianoche, domain.CorteTurno, domain.CorteCierreDeCaja} {
		if err := guardar(modo); err != nil {
			t.Errorf("guardar %q: %v", modo, err)
		}
	}
	if err := guardar("cuando-yo-diga"); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("un modo inventado = %v, quiere ErrValidation: el dueño creería que configuró algo que no configuró", err)
	}
}

// "ENTREGADOS HOY" SE VACÍA A LA MEDIANOCHE DEL LOCAL, NO A LA DEL SERVIDOR.
//
// Filtraba por el día del SERVIDOR, que corre en UTC. En México la medianoche UTC cae a las 18:00
// locales: la lista se vaciaba a media hora pico, con los pedidos del día todavía frescos y el turno
// abierto. El reloj de este test está puesto justo en ese hueco — las 23:00 del local, cuando en UTC
// ya es el día siguiente — porque a cualquier otra hora el defecto no se nota.
func TestLosEntregadosNoSeVacianALas18DelLocal(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// 2026-09-01 23:00 en México = 2026-09-02 05:00 UTC.
	lasOnceDeLaNoche := time.Date(2026, 9, 2, 5, 0, 0, 0, time.UTC)
	// El pedido se entrega a las 20:00 del local, del mismo día.
	lasOchoDeLaNoche := time.Date(2026, 9, 2, 2, 0, 0, 0, time.UTC)

	cajero := makeUser(t, st, "cajero_medianoche", "cajero")
	prod := makeProduct(t, st, "Café medianoche", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	alAnochecer := app.NewOrdersService(st, func() time.Time { return lasOchoDeLaNoche })
	ord, err := crearYCobrar(t, ctx, alAnochecer, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("100")}},
	})
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	if err := alAnochecer.DeliverAll(ctx, ord.ID); err != nil {
		t.Fatalf("entregar: %v", err)
	}

	// A las 23:00 del local —cuando el servidor ya cree que es mañana— el pedido SIGUE en la lista.
	alasOnce := app.NewOrdersService(st, func() time.Time { return lasOnceDeLaNoche })
	lista, err := alasOnce.DeliveredToday(ctx)
	if err != nil {
		t.Fatalf("DeliveredToday: %v", err)
	}
	if !contieneID(lista, ord.ID) {
		t.Error("el pedido entregado hace tres horas desapareció de la lista: se vació a las 18:00 locales, en plena hora pico")
	}

	// Pasada la medianoche del LOCAL, ya no.
	pasadaLaMedianoche := app.NewOrdersService(st, func() time.Time {
		return time.Date(2026, 9, 2, 6, 30, 0, 0, time.UTC) // 00:30 del día 2 en México
	})
	tras, err := pasadaLaMedianoche.DeliveredToday(ctx)
	if err != nil {
		t.Fatalf("DeliveredToday: %v", err)
	}
	if contieneID(tras, ord.ID) {
		t.Error("el pedido de ayer sigue en la lista pasada la medianoche del local")
	}
}

func contieneID(lista []app.BoardOrder, id int64) bool {
	for _, o := range lista {
		if o.ID == id {
			return true
		}
	}
	return false
}

// EL CORTE DE VISTA NO TOCA NI UN PESO DE UN ARQUEO CERRADO.
//
// Un ajuste que se llama "hasta cuándo se ve" y vive junto a los de impresión invita a creer que
// mueve dinero. No lo hace, y esto es lo que lo sostiene: cambiar el modo y la zona no puede alterar
// ninguna cifra de un turno ya cuadrado. Si alguna vez lo hiciera, el turno de ayer cerraría distinto
// y nadie sabría por qué hasta el corte siguiente.
func TestElCorteDeVistaNoCambiaUnArqueoCerrado(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)
	settings := app.NewSettingsService(st, "pepper-de-prueba")

	cajero := makeUser(t, st, "cajero_arqueo", "cajero")
	admin := makeUser(t, st, "admin_arqueo", "admin")
	prod := makeProduct(t, st, "Café arqueo", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	caja := registerID(t, st, "Caja principal")
	abrirCajaPrincipal(t, st, cajero)

	ord, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("100")}},
	})
	if err != nil {
		t.Fatalf("vender: %v", err)
	}
	if err := svc.DeliverAll(ctx, ord.ID); err != nil {
		t.Fatalf("entregar: %v", err)
	}

	antes, err := back.CurrentByRegister(ctx, caja)
	if err != nil {
		t.Fatalf("arqueo antes: %v", err)
	}

	// Se cambian las dos cosas que esta feature toca.
	prev, _ := settings.Get(ctx)
	info := domain.BusinessInfo{Name: prev.BusinessName, Address: prev.Address, Phone: prev.Phone}
	for _, modo := range []string{domain.CorteTurno, domain.CorteCierreDeCaja, domain.CorteMedianoche} {
		if _, err := settings.SetBusinessInfo(ctx, info,
			domain.PrintSettings{CorteDeVista: modo}, domain.DefaultIdentity(), "America/Tijuana", admin); err != nil {
			t.Fatalf("cambiar a %q: %v", modo, err)
		}
	}

	despues, err := back.CurrentByRegister(ctx, caja)
	if err != nil {
		t.Fatalf("arqueo después: %v", err)
	}
	if !despues.OpeningCash.Equal(antes.OpeningCash) || !despues.NetMovements.Equal(antes.NetMovements) {
		t.Errorf("el arqueo cambió: fondo %s→%s, movimientos %s→%s. Un ajuste de PANTALLA movió dinero",
			antes.OpeningCash, despues.OpeningCash, antes.NetMovements, despues.NetMovements)
	}
	if len(despues.Totals) != len(antes.Totals) {
		t.Fatalf("el arqueo pasó de %d métodos a %d", len(antes.Totals), len(despues.Totals))
	}
	for i := range antes.Totals {
		if !despues.Totals[i].Expected.Equal(antes.Totals[i].Expected) {
			t.Errorf("el esperado de %s cambió de %s a %s: un ajuste de PANTALLA movió dinero",
				antes.Totals[i].Name, antes.Totals[i].Expected, despues.Totals[i].Expected)
		}
	}
}

// LA LISTA DE ENTREGADOS TIENE QUE FUNCIONAR CON LA CAJA CERRADA.
//
// `MomentosDelTurnoPrincipal` trae cuándo abrió el turno vigente con una subconsulta escalar, que
// devuelve NULL cuando no hay ninguno abierto. sqlc infirió el tipo copiando la nulabilidad de la
// COLUMNA —`opened_at` es not null— así que generó un `time.Time` que no acepta NULL, y el escaneo
// truena antes de llegar a la rama que ya contemplaba ese caso.
//
// El efecto es todas las noches: en cuanto se cierra la caja, la sección de reembolsos del tablero
// contesta 500. Y en un negocio recién instalado, desde el primer día hasta la primera apertura.
// Pasa con CUALQUIER modo de corte, porque la consulta corre antes de saber cuál se va a usar.
func TestLosEntregadosSeVenConLaCajaCerrada(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	// A propósito SIN abrir caja: es el estado del negocio la mayor parte del día.
	if _, err := svc.DeliveredToday(ctx); err != nil {
		t.Fatalf("con la caja cerrada la lista de entregados truena: %v", err)
	}
}

// EL PEDIDO QUE SE ABRE ANTES DEL CORTE Y SE ENTREGA DESPUÉS TIENE QUE VERSE.
//
// La lista filtraba por `opened_at`, pero lo que decide si un entregado pertenece a la ventana
// visible es cuándo se COMPLETÓ, no cuándo se abrió. Un pedido levantado a las 23:50 y entregado a
// las 00:05 quedaba fuera justo cuando se vuelve accionable: recién entregado y candidato a
// reembolso. La misma consulta ya ordenaba por `completed_at`, así que el filtro miraba una columna
// y el orden otra.
func TestElPedidoQueCruzaElCorteAlEntregarseSigueALaVista(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// 23:50 en México del día 1 = 05:50 UTC del día 2.
	antesDelCorte := time.Date(2026, 9, 2, 5, 50, 0, 0, time.UTC)
	// 00:05 del día 2 en México = 06:05 UTC.
	despuesDelCorte := time.Date(2026, 9, 2, 6, 5, 0, 0, time.UTC)

	cajero := makeUser(t, st, "cajero_cruce", "cajero")
	prod := makeProduct(t, st, "Café cruce", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	abrirCajaPrincipal(t, st, cajero)

	alFilo := app.NewOrdersService(st, func() time.Time { return antesDelCorte })
	ord, err := crearYCobrar(t, ctx, alFilo, app.CreateOrderCmd{
		ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
		Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("100")}},
	})
	if err != nil {
		t.Fatalf("crear a las 23:50: %v", err)
	}

	yaPasoLaMedianoche := app.NewOrdersService(st, func() time.Time { return despuesDelCorte })
	if err := yaPasoLaMedianoche.DeliverAll(ctx, ord.ID); err != nil {
		t.Fatalf("entregar a las 00:05: %v", err)
	}
	// `completed_at` lo pone el reloj de la BASE, no el que se le inyecta al servicio, así que se
	// fija a mano al instante que este caso describe. Sin esto el test mide la hora real de la
	// máquina y no el borde que viene a cubrir.
	if _, err := st.Pool.Exec(ctx,
		`update orders set completed_at = $2 where id = $1`, ord.ID, despuesDelCorte); err != nil {
		t.Fatalf("fijar la hora de entrega: %v", err)
	}

	lista, err := yaPasoLaMedianoche.DeliveredToday(ctx)
	if err != nil {
		t.Fatalf("DeliveredToday: %v", err)
	}
	if !contieneID(lista, ord.ID) {
		t.Error("el pedido entregado hace cinco minutos no aparece: se filtró por cuándo se abrió y no por cuándo se completó, justo cuando se vuelve accionable")
	}
}
