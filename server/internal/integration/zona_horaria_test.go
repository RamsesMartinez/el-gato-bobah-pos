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

// Un negocio nace con la zona de México, sin que nadie configure nada: el producto se vende aquí y
// el local que lo estrena no debería tener que tocar ajustes para que su primer corte cuadre.
func TestNegocioNaceConZonaDeMexico(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	var tz string
	if err := st.Pool.QueryRow(ctx,
		`select timezone from business_settings where company_id = $1`, defaultCompanyID).Scan(&tz); err != nil {
		t.Fatalf("leer la zona: %v", err)
	}
	if tz != domain.DefaultTimezone {
		t.Fatalf("zona por default = %q, quería %q", tz, domain.DefaultTimezone)
	}
	if !domain.ValidTimezone(tz) {
		t.Fatalf("la zona sembrada no es un nombre IANA válido: %q", tz)
	}
}

// El caso real que destapó todo: una venta de las 20:28 hora de México, que en UTC ya es el día
// siguiente, tiene que quedar en el día de HOY. Con la fecha calculada en UTC el folio se
// reiniciaba a media cena y salían dos tickets #1 la misma noche.
func TestVentaDeLaNocheCuentaEnElDiaDelLocal(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// 02:28 UTC del 30 = 20:28 del 29 en México.
	noche := time.Date(2026, 8, 30, 2, 28, 0, 0, time.UTC)
	ordersSvc := app.NewOrdersService(st, func() time.Time { return noche })
	backoffice := app.NewBackofficeService(st, func() time.Time { return noche })

	cajero := makeUser(t, st, "cajero_noche", "cajero")
	prod := makeProduct(t, st, "Café", decimal.RequireFromString("80"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	principal := registerID(t, st, "Caja principal")

	sess, err := backoffice.OpenSession(ctx, principal, decimal.Zero, cajero)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	// El turno abre en la fecha del LOCAL, no en la de UTC.
	var sessDate time.Time
	if err := st.Pool.QueryRow(ctx,
		`select business_date from register_sessions where id = $1`, sess.ID).Scan(&sessDate); err != nil {
		t.Fatal(err)
	}
	if got := sessDate.Format("2006-01-02"); got != "2026-08-29" {
		t.Fatalf("el turno abrió con fecha %s, quería 2026-08-29 (20:28 hora de México)", got)
	}

	ord, err := crearYCobrar(t, ctx, ordersSvc, app.CreateOrderCmd{
		ClientUUID:  uuid.New(),
		ServiceType: "mostrador",
		OpenedBy:    cajero,
		Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		Payments:    []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("80")}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var ordDate time.Time
	if err := st.Pool.QueryRow(ctx,
		`select business_date from orders where id = $1`, ord.ID).Scan(&ordDate); err != nil {
		t.Fatal(err)
	}
	if got := ordDate.Format("2006-01-02"); got != "2026-08-29" {
		t.Fatalf("la venta quedó en %s, quería 2026-08-29", got)
	}
}

// LOS DOS CAMINOS SON INDEPENDIENTES: la fecha la da el reloj, el folio lo da el turno.
//
// Antes la venta HEREDABA la fecha del turno, y eso resolvía la numeración de un turno nocturno a
// costa de que un turno que nadie cerrara siguiera estampando su fecha días después. Se midió: 158
// pedidos archivados como 31 de agosto hasta el 4 de septiembre, y la pantalla de Ventas del día
// saliendo vacía con el negocio vendiendo.
//
// Ahora ninguno de los dos lee al otro, y este test prueba las dos mitades a la vez a propósito:
// si alguien vuelve a acoplarlas, una de las dos aserciones se cae.
func TestElFolioSigueAlTurnoAunqueCruceLaMedianoche(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	antes := time.Date(2026, 8, 30, 4, 30, 0, 0, time.UTC)   // 22:30 del 29 en México
	despues := time.Date(2026, 8, 30, 7, 30, 0, 0, time.UTC) // 01:30 del 30 en México

	cajero := makeUser(t, st, "cajero_medianoche", "cajero")
	prod := makeProduct(t, st, "Café", decimal.RequireFromString("80"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	principal := registerID(t, st, "Caja principal")

	backoffice := app.NewBackofficeService(st, func() time.Time { return antes })
	if _, err := backoffice.OpenSession(ctx, principal, decimal.Zero, cajero); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	venta := func(now time.Time) *app.OrderView {
		t.Helper()
		svc := app.NewOrdersService(st, func() time.Time { return now })
		o, err := crearYCobrar(t, ctx, svc, app.CreateOrderCmd{
			ClientUUID:  uuid.New(),
			ServiceType: "mostrador",
			OpenedBy:    cajero,
			Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
			Payments:    []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("80")}},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		return o
	}

	primera := venta(antes)
	segunda := venta(despues) // ya es otro día en México, pero el MISMO turno

	if primera.Number == segunda.Number {
		t.Fatalf("dos ventas del mismo turno con el folio %d: el contador se reinició a media noche", primera.Number)
	}
	if segunda.Number != primera.Number+1 {
		t.Fatalf("los folios del turno deben ir corridos: %d y %d", primera.Number, segunda.Number)
	}

	// Y la otra mitad: la fecha SÍ cambia, porque la decide el reloj y ya es otro día en México.
	var d1, d2 time.Time
	_ = st.Pool.QueryRow(ctx, `select business_date from orders where id = $1`, primera.ID).Scan(&d1)
	_ = st.Pool.QueryRow(ctx, `select business_date from orders where id = $1`, segunda.ID).Scan(&d2)
	zona := domain.LoadBusinessLocation(domain.DefaultTimezone)
	if quiere := domain.BusinessDate(antes, zona); !d1.Equal(quiere) {
		t.Errorf("la venta de las 22:30 quedó archivada el %s y ocurrió el %s",
			d1.Format("2006-01-02"), quiere.Format("2006-01-02"))
	}
	if quiere := domain.BusinessDate(despues, zona); !d2.Equal(quiere) {
		t.Errorf("la venta de las 01:30 quedó archivada el %s y ocurrió el %s",
			d2.Format("2006-01-02"), quiere.Format("2006-01-02"))
	}
	if d1.Equal(d2) {
		t.Errorf("las dos ventas quedaron el mismo día (%s): la fecha se sigue heredando del turno "+
			"en vez de leerse del reloj", d1.Format("2006-01-02"))
	}
}

// La zona se puede cambiar desde la configuración del local, y una inválida se rechaza AHÍ. Donde
// se usa cae a UTC para no tumbar un cobro, así que si nunca se rechazara al guardar, ese fallback
// correría las fechas de los cortes en silencio durante meses.
func TestLaZonaSeCambiaYSeValidaAlGuardar(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	admin := makeUser(t, st, "admin_zona", "admin")

	cur, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	info := domain.BusinessInfo{Name: cur.BusinessName, Address: cur.Address, Phone: cur.Phone}
	impresion := domain.PrintSettings{
		AutoPrintOnClose:   cur.AutoPrintOnClose,
		PrintFreeModifiers: cur.PrintFreeModifiers,
		PrintKitchenTicket: cur.PrintKitchenTicket,
	}

	if _, err := settings.SetBusinessInfo(ctx, info, impresion, domain.DefaultIdentity(), "Marte/Olympus", admin); err == nil {
		t.Fatal("una zona inventada debe rechazarse al guardar")
	}

	nueva, err := settings.SetBusinessInfo(ctx, info, impresion, domain.DefaultIdentity(), "America/Tijuana", admin)
	if err != nil {
		t.Fatalf("una zona válida debe guardarse: %v", err)
	}
	if nueva.Timezone != "America/Tijuana" {
		t.Fatalf("la zona quedó en %q", nueva.Timezone)
	}

	// Y manda de verdad: Tijuana es UTC-7, así que un instante que en México es del 29 a las 20:28
	// allá es del 29 a las 19:28 — mismo día, pero la frontera se mueve.
	sess := time.Date(2026, 8, 30, 6, 30, 0, 0, time.UTC) // 00:30 del 30 en CDMX, 23:30 del 29 en Tijuana
	backoffice := app.NewBackofficeService(st, func() time.Time { return sess })
	cajero := makeUser(t, st, "cajero_tj", "cajero")
	principal := registerID(t, st, "Caja principal")
	abierta, err := backoffice.OpenSession(ctx, principal, decimal.Zero, cajero)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	var fecha time.Time
	if err := st.Pool.QueryRow(ctx, `select business_date from register_sessions where id = $1`, abierta.ID).Scan(&fecha); err != nil {
		t.Fatal(err)
	}
	if got := fecha.Format("2006-01-02"); got != "2026-08-29" {
		t.Fatalf("con Tijuana el turno debía abrir el 2026-08-29, abrió el %s", got)
	}
}
