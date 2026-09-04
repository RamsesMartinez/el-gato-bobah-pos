//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// LAS VENTAS DE UN CORTE Y SU TOTAL SALEN DEL MISMO PREDICADO.
//
// Si la lista y el resumen de una misma pantalla se derivan de cosas distintas, uno de los dos
// miente y quien lo lee no tiene forma de saber cuál. Aquí el borde concreto es la clasificación
// del dinero: una venta cancelada SÍ es parte de lo que pasó en el turno y se lista, pero su dinero
// NO entró y no puede sumar al total.
func TestElDetalleDelCorteListaSusVentasYSoloSumaElIngreso(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_ventas_corte", "cajero")
	prod := makeProduct(t, st, "Café del corte", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	sess := abrirCajaPrincipal(t, st, cajero)

	svc := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)

	nueva := func(cobrar bool) *app.OrderView {
		t.Helper()
		cmd := app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		}
		if cobrar {
			cmd.Payments = []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("100")}}
		}
		o, err := crearYCobrar(t, ctx, svc, cmd)
		if err != nil {
			t.Fatalf("crear: %v", err)
		}
		return o
	}

	nueva(true)
	nueva(true)
	// La tercera se cancela por el camino real. Sin cobrar, que es el único que se puede cancelar sin
	// devolver dinero desde la feature 007.
	cancelada := nueva(false)
	if err := svc.Cancel(ctx, cancelada.ID, cajero, "prueba del detalle del corte"); err != nil {
		t.Fatalf("cancelar: %v", err)
	}

	det, err := back.SessionDetail(ctx, sess)
	if err != nil {
		t.Fatalf("SessionDetail: %v", err)
	}

	if det.SalesCount != 3 {
		t.Errorf("el corte cobró 3 ventas y el detalle dice %d: la cancelada también pasó en este "+
			"turno y tiene que verse", det.SalesCount)
	}
	if len(det.Sales) != 3 {
		t.Errorf("la lista trae %d ventas y el conteo dice %d: no salen del mismo where",
			len(det.Sales), det.SalesCount)
	}
	if quiere := decimal.RequireFromString("200"); !det.SalesTotal.Equal(quiere) {
		t.Errorf("el total del corte es %s y debía ser %s: la venta cancelada está sumando dinero "+
			"que nunca entró", det.SalesTotal, quiere)
	}
}

// Ninguna venta de otro corte se cuela.
//
// El filtro es por turno y no por ventana de tiempo: dos turnos del mismo día comparten horas, y
// acotar por hora metería en un arqueo el dinero del otro.
func TestElDetalleDeUnCorteNoTraeVentasDeOtro(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	cajero := makeUser(t, st, "cajero_dos_cortes", "cajero")
	prod := makeProduct(t, st, "Café dos cortes", decimal.RequireFromString("50"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	principal := registerID(t, st, "Caja principal")
	back := app.NewBackofficeService(st, clock)
	svc := app.NewOrdersService(st, clock)

	primerTurno, err := back.OpenSession(ctx, principal, decimal.Zero, cajero)
	if err != nil {
		t.Fatalf("abrir el primer turno: %v", err)
	}
	venta := func() {
		t.Helper()
		if _, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
			Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("50")}},
		}); err != nil {
			t.Fatalf("crear: %v", err)
		}
	}
	venta()

	entregarPendientes(t, st)
	declarado := map[int]decimal.Decimal{int(efectivo): decimal.RequireFromString("50")}
	if _, err := back.CloseSession(ctx, principal, cajero, declarado, ""); err != nil {
		t.Fatalf("cerrar: %v", err)
	}
	segundoTurno, err := back.OpenSession(ctx, principal, decimal.Zero, cajero)
	if err != nil {
		t.Fatalf("reabrir: %v", err)
	}
	venta()
	venta()

	uno, err := back.SessionDetail(ctx, primerTurno.ID)
	if err != nil {
		t.Fatalf("detalle del primero: %v", err)
	}
	dos, err := back.SessionDetail(ctx, segundoTurno.ID)
	if err != nil {
		t.Fatalf("detalle del segundo: %v", err)
	}
	if uno.SalesCount != 1 {
		t.Errorf("el primer corte cobró 1 venta y su detalle dice %d", uno.SalesCount)
	}
	if dos.SalesCount != 2 {
		t.Errorf("el segundo corte cobró 2 ventas y su detalle dice %d", dos.SalesCount)
	}
	if !uno.SalesTotal.Equal(decimal.RequireFromString("50")) {
		t.Errorf("el primer corte suma %s y debía sumar 50: se le colaron ventas del otro turno", uno.SalesTotal)
	}
}

// EL AVISO DE TURNO VIEJO COMPARA DÍAS, NO HORAS.
//
// Nada le decía a quien opera que su turno abierto era de hace días, y por eso el defecto duró
// cinco sin que nadie lo notara. Un umbral de horas dejaría pasar el turno que abrió ayer a las
// 23:00 —el caso que importa— y molestaría al que abrió hoy temprano.
func TestElEstadoDeCajaAvisaCuandoElTurnoEsDeOtroDia(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	cajero := makeUser(t, st, "cajero_aviso", "cajero")
	back := app.NewBackofficeService(st, clock)

	// Sin turno abierto: ni "abierta" ni aviso. La pantalla ya tiene su propio mensaje para esto.
	sinTurno, err := back.EstadoDeCaja(ctx)
	if err != nil {
		t.Fatalf("EstadoDeCaja sin turno: %v", err)
	}
	if sinTurno.Open || sinTurno.DeOtroDia {
		t.Errorf("sin turno abierto: open=%v deOtroDia=%v, se esperaba false y false",
			sinTurno.Open, sinTurno.DeOtroDia)
	}

	// Turno abierto AYER a las 23:00 locales: lleva una hora, y ya es de otro día.
	zona := domain.LoadBusinessLocation(domain.DefaultTimezone)
	ayerTarde := time.Date(fixedNow.Year(), fixedNow.Month(), fixedNow.Day()-1, 23, 0, 0, 0, zona).UTC()
	abrirCajaEn(t, st, cajero, ayerTarde)

	viejo, err := back.EstadoDeCaja(ctx)
	if err != nil {
		t.Fatalf("EstadoDeCaja con turno viejo: %v", err)
	}
	if !viejo.Open {
		t.Fatal("el turno está abierto y el estado dice que no")
	}
	if !viejo.DeOtroDia {
		t.Errorf("el turno abrió el %s y hoy es %s: tenía que avisar que es de otro día",
			ayerTarde.In(zona).Format("2006-01-02 15:04"), fixedNow.In(zona).Format("2006-01-02"))
	}
	if viejo.OpenedAt == nil || viejo.BusinessDate == nil {
		t.Error("el aviso sin desde-cuándo no le sirve a quien opera: falta openedAt o businessDate")
	}
}
