//go:build integration

package integration

import (
	"context"
	"testing"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// El fondo de apertura y los movimientos de efectivo se cuentan UNA sola vez en el corte, no una
// por cada método que toca el cajón.
//
// Hasta 0037 solo existía un método de cajón ("Efectivo"), así que sumarlo por método daba el
// resultado correcto por casualidad. Al desdoblar cada plataforma en "en línea" y "efectivo" hay
// cuatro, y sin este arreglo un turno con $1,500 de fondo y CERO ventas reporta $4,500 de faltante
// que no existe: los tres métodos nuevos esperan $1,500 cada uno y, como no se autodeclaran, el
// cierre los compara contra lo que el front no mandó.
func TestElFondoDeCajaSeCuentaUnaSolaVez(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_fondo", "cajero")
	principal := registerID(t, st, "Caja principal")
	fondo := decimal.RequireFromString("1500")
	if _, err := backoffice.OpenSession(ctx, principal, fondo, cajero); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	sess, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}

	// Sin ventas, el único método que espera dinero es el efectivo del mostrador, y espera
	// exactamente el fondo.
	var conFondo []string
	for _, tot := range sess.Totals {
		if tot.Expected.Equal(fondo) {
			conFondo = append(conFondo, tot.Name)
		}
	}
	if len(conFondo) != 1 {
		t.Fatalf("el fondo de $1,500 debe aparecer en UN solo método, apareció en %d: %v", len(conFondo), conFondo)
	}
	if conFondo[0] != "Efectivo" {
		t.Fatalf("el fondo debe ir al efectivo del mostrador, fue a %q", conFondo[0])
	}

	// Y el resto espera cero: no hubo ventas.
	for _, tot := range sess.Totals {
		if tot.Name == "Efectivo" {
			continue
		}
		if !tot.Expected.IsZero() {
			t.Fatalf("%q espera %s sin haber vendido nada", tot.Name, tot.Expected)
		}
	}
}

// Cerrar un turno sin ventas y declarando el fondo exacto no debe arrojar diferencia. Es la misma
// regla vista desde el cierre, que es donde el operador la sufre: un faltante inventado obliga a
// contar el cajón tres veces buscando dinero que nunca faltó.
func TestCerrarSinVentasNoInventaFaltante(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_cierre", "cajero")
	principal := registerID(t, st, "Caja principal")
	fondo := decimal.RequireFromString("1500")
	if _, err := backoffice.OpenSession(ctx, principal, fondo, cajero); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	// El operador cuenta el cajón y declara el fondo, que es lo único que hay.
	efectivo := paymentMethodID(t, st, "Efectivo")
	declarado := map[int]decimal.Decimal{int(efectivo): fondo}

	cerrada, err := backoffice.CloseSession(ctx, principal, cajero, declarado, "")
	if err != nil {
		t.Fatalf("CloseSession: %v", err)
	}
	for _, tot := range cerrada.Totals {
		dif := tot.Declared.Sub(tot.Expected)
		if !dif.IsZero() {
			t.Fatalf("%q cerró con diferencia de %s (esperado %s, declarado %s) sin haber vendido nada",
				tot.Name, dif, tot.Expected, tot.Declared)
		}
	}
}

// El corte suma lo cobrado EN ESTE TURNO, no lo cobrado desde una hora. La query lo hacía por
// ventana de tiempo (created_at >= apertura) y funcionaba de casualidad: solo la caja principal
// vende y no puede haber dos turnos suyos abiertos, así que la ventana y la sesión coincidían.
//
// Correcto por coincidencia no es correcto. Ahora que cada pago guarda su register_session_id, el
// vínculo es explícito: un pago de otro turno no puede colarse aunque caiga dentro de la ventana.
func TestElCorteSumaPorTurnoYNoPorHora(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	ordersSvc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_turnos", "cajero")
	prod := makeProduct(t, st, "Café", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	principal := registerID(t, st, "Caja principal")

	vender := func() {
		t.Helper()
		if _, err := ordersSvc.Create(ctx, app.CreateOrderCmd{
			ClientUUID:  uuid.New(),
			ServiceType: "mostrador",
			OpenedBy:    cajero,
			Lines:       []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
			Payments:    []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("100")}},
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	// Turno 1: una venta de $100, y se cierra declarando lo que hay.
	if _, err := backoffice.OpenSession(ctx, principal, decimal.Zero, cajero); err != nil {
		t.Fatalf("OpenSession 1: %v", err)
	}
	vender()
	if _, err := backoffice.CloseSession(ctx, principal, cajero,
		map[int]decimal.Decimal{int(efectivo): decimal.RequireFromString("100")}, ""); err != nil {
		t.Fatalf("CloseSession 1: %v", err)
	}

	// Turno 2, el mismo día y con el mismo reloj: debe esperar SOLO su propia venta.
	if _, err := backoffice.OpenSession(ctx, principal, decimal.Zero, cajero); err != nil {
		t.Fatalf("OpenSession 2: %v", err)
	}
	vender()
	segunda, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	for _, tot := range segunda.Totals {
		if tot.Name != "Efectivo" {
			continue
		}
		if !tot.Expected.Equal(decimal.RequireFromString("100")) {
			t.Fatalf("el segundo turno espera %s: se le coló la venta del turno anterior", tot.Expected)
		}
	}
}

// El subtotal por plataforma sale de datos reales, no de sumar renglones a mano.
//
// El corte ya separaba por método, pero cada plataforma tiene DOS —en línea y efectivo—, así que
// para saber cuánto facturó Uber en el turno había que sumarlos de cabeza. Es el número contra el
// que se concilia el depósito que la plataforma manda después, y sumarlo a mano a las once de la
// noche es donde se equivoca cualquiera.
func TestElCorteSubtotalizaPorPlataforma(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	ordersSvc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_subtotal", "cajero")
	prod := makeProduct(t, st, "Boneless sub", decimal.RequireFromString("100"), false)
	principal := registerID(t, st, "Caja principal")
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	didi := platformID(t, st, defaultCompanyID, "Didi")
	uberEnLinea := paymentMethodID(t, st, "Uber Eats en línea")
	uberEfectivo := paymentMethodID(t, st, "Uber Eats efectivo")
	didiEnLinea := paymentMethodID(t, st, "Didi en línea")
	efectivo := paymentMethodID(t, st, "Efectivo")

	if _, err := backoffice.OpenSession(ctx, principal, decimal.RequireFromString("500"), cajero); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	// Con el 35% sembrado, cada pedido de una pieza cobra 135.
	vender := func(plataforma *int16, metodo int16, monto string) {
		t.Helper()
		st := "mostrador"
		if plataforma != nil {
			st = "domicilio"
		}
		if _, err := ordersSvc.Create(ctx, app.CreateOrderCmd{
			ClientUUID:         uuid.New(),
			ServiceType:        st,
			DeliveryPlatformID: plataforma,
			OpenedBy:           cajero,
			Lines:              []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
			Payments:           []app.PaymentInput{{MethodID: metodo, Amount: decimal.RequireFromString(monto)}},
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	vender(&uber, uberEnLinea, "135")  // Uber, la plataforma pagó
	vender(&uber, uberEfectivo, "135") // Uber, el repartidor pagó en efectivo
	vender(&didi, didiEnLinea, "135")
	vender(nil, efectivo, "100") // mostrador: no debe aparecer en ninguna plataforma

	sess, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}

	porNombre := map[string]decimal.Decimal{}
	for _, p := range sess.Breakdown.Plataformas {
		porNombre[p.Platform] = p.Total
	}
	if len(porNombre) != 2 {
		t.Fatalf("quiere 2 plataformas con ventas (Rappi no vendió), hubo %d: %+v", len(porNombre), sess.Breakdown.Plataformas)
	}
	// 270 y no 135: los DOS métodos de Uber suman, que es justo lo que el corte no hacía.
	if got := porNombre["Uber Eats"]; !got.Equal(decimal.RequireFromString("270")) {
		t.Fatalf("Uber Eats = %s, quiere 270 (135 en línea + 135 efectivo)", got)
	}
	if got := porNombre["Didi"]; !got.Equal(decimal.RequireFromString("135")) {
		t.Fatalf("Didi = %s, quiere 135", got)
	}

	// El efectivo de mostrador NO es de ninguna plataforma. Si se colara, el subtotal de Uber
	// incluiría el fondo de caja de $500 y el operador buscaría un depósito que nunca llega.
	suma := decimal.Zero
	for _, p := range sess.Breakdown.Plataformas {
		suma = suma.Add(p.Total)
	}
	if !suma.Equal(decimal.RequireFromString("405")) {
		t.Fatalf("la suma de plataformas = %s, quiere 405: el mostrador o el fondo se colaron", suma)
	}
}

// El subtotal por plataforma tiene que seguir ahí DESPUÉS de cerrar el turno.
//
// Es cuando de verdad sirve: el depósito de la plataforma llega días después del cierre, y ese
// número es contra el que se concilia. Salía en la sesión viva y desaparecía en el histórico —sin
// error, sin renglón, sin nada que avisara— porque la consulta del turno cerrado es otra y no
// traía la plataforma del método.
func TestElSubtotalPorPlataformaSobreviveAlCierre(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	ordersSvc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_cierre_plat", "cajero")
	prod := makeProduct(t, st, "Boneless cierre", decimal.RequireFromString("100"), false)
	principal := registerID(t, st, "Caja principal")
	uber := platformID(t, st, defaultCompanyID, "Uber Eats")
	uberEnLinea := paymentMethodID(t, st, "Uber Eats en línea")
	uberEfectivo := paymentMethodID(t, st, "Uber Eats efectivo")
	efectivo := paymentMethodID(t, st, "Efectivo")

	sess, err := backoffice.OpenSession(ctx, principal, decimal.Zero, cajero)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	vender := func(metodo int16, monto string) {
		t.Helper()
		tipo, plataforma := "mostrador", (*int16)(nil)
		if metodo != efectivo {
			tipo, plataforma = "domicilio", &uber
		}
		if _, err := ordersSvc.Create(ctx, app.CreateOrderCmd{
			ClientUUID:         uuid.New(),
			ServiceType:        tipo,
			DeliveryPlatformID: plataforma,
			OpenedBy:           cajero,
			Lines:              []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
			Payments:           []app.PaymentInput{{MethodID: metodo, Amount: decimal.RequireFromString(monto)}},
		}); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	vender(uberEnLinea, "135")
	vender(uberEfectivo, "135")
	vender(efectivo, "100")

	if _, err := backoffice.CloseSession(ctx, principal, cajero, map[int]decimal.Decimal{
		int(efectivo):     decimal.RequireFromString("100"),
		int(uberEnLinea):  decimal.RequireFromString("135"),
		int(uberEfectivo): decimal.RequireFromString("135"),
	}, ""); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	detalle, err := backoffice.SessionDetail(ctx, sess.ID)
	if err != nil {
		t.Fatalf("SessionDetail: %v", err)
	}
	if len(detalle.Breakdown.Plataformas) != 1 {
		t.Fatalf("el turno cerrado debe conservar el subtotal por plataforma, trajo %d: %+v",
			len(detalle.Breakdown.Plataformas), detalle.Breakdown.Plataformas)
	}
	p := detalle.Breakdown.Plataformas[0]
	if p.Platform != "Uber Eats" || !p.Total.Equal(decimal.RequireFromString("270")) {
		t.Fatalf("subtotal del histórico = %+v, quiere Uber Eats 270", p)
	}
}
