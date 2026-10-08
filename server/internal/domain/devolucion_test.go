package domain

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

// NO SE DEVUELVE MÁS DE LO QUE ENTRÓ.
//
// El defecto que esto cierra: `Refund` anotaba como pérdida `o.Total` sin mirar los cobros. Un
// pedido entregado con $220 pendientes registraba $220 de pérdida por un ingreso que nunca ocurrió,
// y la cuenta por cobrar desaparecía del tablero sin haberse cobrado.
func TestNoSeDevuelveMasDeLoQueEntro(t *testing.T) {
	casos := []struct {
		nombre              string
		cobrado, yaDevuelto string
		pide                string
		quiere              bool // true = se acepta
	}{
		{"lo cobrado completo", "500", "0", "500", true},
		{"una parte", "500", "0", "60", true},
		{"el resto tras una devolución previa", "500", "440", "60", true},
		{"un peso más de lo cobrado", "500", "0", "500.01", false},
		{"dos veces lo mismo", "500", "500", "0.01", false},
		{"más de lo que queda", "500", "440", "61", false},
		{"cero no es devolver", "500", "0", "0", false},
		{"negativo es un cobro disfrazado", "500", "0", "-50", false},
		// Un pedido que nadie pagó no tiene nada que devolver, y decirlo es la mitad del arreglo:
		// hoy el tablero ofrece "Reembolsar" junto a "Cobrar $220" en la misma tarjeta.
		{"un pedido sin cobros", "0", "0", "1", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := ValidarDevolucion(d(c.pide), d(c.cobrado), d(c.yaDevuelto))
			if c.quiere && err != nil {
				t.Fatalf("debía aceptarse, fue %v", err)
			}
			if !c.quiere && err == nil {
				t.Fatal("debía rechazarse y pasó")
			}
			if !c.quiere && !errors.Is(err, ErrValidation) && !errors.Is(err, ErrDevolucionExcede) &&
				!errors.Is(err, ErrSinCobrosQueDevolver) {
				t.Fatalf("el rechazo debe ser de dominio, fue %v", err)
			}
		})
	}
}

// Un pedido sin un solo cobro se rechaza con SU error, no con uno genérico: es el caso que la
// pantalla tiene que poder explicar sin mandar al operador a adivinar.
func TestUnPedidoSinCobrosSeRechazaConSuPropioError(t *testing.T) {
	if err := ValidarDevolucion(d("1"), d("0"), d("0")); !errors.Is(err, ErrSinCobrosQueDevolver) {
		t.Fatalf("err = %v, quiere ErrSinCobrosQueDevolver", err)
	}
}

func TestMontoDevolvible(t *testing.T) {
	casos := []struct{ cobrado, yaDevuelto, quiere string }{
		{"500", "0", "500"},
		{"500", "440", "60"},
		{"500", "500", "0"},
		{"0", "0", "0"},
		// Nunca negativo: si por lo que sea se devolvió de más, lo que queda por devolver es cero,
		// no una deuda del cliente hacia el negocio.
		{"500", "600", "0"},
	}
	for _, c := range casos {
		got := MontoDevolvible(d(c.cobrado), d(c.yaDevuelto))
		if !got.Equal(d(c.quiere)) {
			t.Fatalf("MontoDevolvible(%s, %s) = %s, quiere %s", c.cobrado, c.yaDevuelto, got, c.quiere)
		}
	}
}

// EL DINERO SALE POR DONDE ENTRÓ.
//
// Devolver en efectivo lo que entró por tarjeta saca del cajón dinero que nunca estuvo ahí, y el
// arqueo cierra con un faltante inventado. El reparto es la regla que lo impide, y va en `domain`
// porque es aritmética de dinero que tiene que poder probarse sin base de datos.
func TestElRepartoSacaElDineroPorDondeEntro(t *testing.T) {
	entradas := []CobradoPorMetodo{
		{MetodoID: 1, Nombre: "Efectivo", TocaElCajon: true, Monto: d("300")},
		{MetodoID: 2, Nombre: "Tarjeta", Monto: d("200")},
	}

	// Menos que el primer método: sale todo de ahí y el segundo ni aparece.
	partes := RepartirDevolucion(entradas, d("120"))
	if len(partes) != 1 || partes[0].MetodoID != 1 || !partes[0].Monto.Equal(d("120")) {
		t.Fatalf("reparto de 120 = %+v, quiere 120 solo del efectivo", partes)
	}

	// Más que el primero: se agota el primero y el resto sale del segundo.
	partes = RepartirDevolucion(entradas, d("450"))
	if len(partes) != 2 {
		t.Fatalf("reparto de 450 = %+v, quiere dos partes", partes)
	}
	if !partes[0].Monto.Equal(d("300")) || !partes[1].Monto.Equal(d("150")) {
		t.Fatalf("reparto de 450 = %s y %s, quiere 300 y 150", partes[0].Monto, partes[1].Monto)
	}

	// Todo: cada método devuelve exactamente lo suyo.
	partes = RepartirDevolucion(entradas, d("500"))
	total := decimal.Zero
	for _, p := range partes {
		total = total.Add(p.Monto)
		for _, e := range entradas {
			if e.MetodoID == p.MetodoID && p.Monto.GreaterThan(e.Monto) {
				t.Fatalf("por %s se devuelven %s y solo entraron %s", e.Nombre, p.Monto, e.Monto)
			}
		}
	}
	if !total.Equal(d("500")) {
		t.Fatalf("el reparto suma %s, quiere 500", total)
	}
}

// SALE DEL CAJÓN LO QUE ESTABA EN EL CAJÓN, y eso lo dice el interruptor del método, no su tipo.
//
// «Didi efectivo» es de tipo plataforma y sus billetes están en el mismo montón que los del
// mostrador cuando reparte gente del local. Con la regla vieja —"solo el efectivo"— devolverle a
// ese cliente sacaba los billetes de la caja sin registrar la salida y el corte cerraba con un
// faltante del tamaño de la devolución. La tarjeta sigue sin salir: ese dinero nunca estuvo ahí y
// descontarlo inventaría el faltante en el otro sentido.
func TestSaleDelCajonLoQueEstabaEnElCajon(t *testing.T) {
	entradas := []CobradoPorMetodo{
		{MetodoID: 2, Nombre: "Tarjeta", Monto: d("200")},
		{MetodoID: 1, Nombre: "Efectivo", TocaElCajon: true, Monto: d("300")},
		{MetodoID: 8, Nombre: "Didi efectivo", TocaElCajon: true, Monto: d("135")},
		{MetodoID: 9, Nombre: "Rappi efectivo", Monto: d("80")}, // el repartidor de la app se lo llevó
	}
	partes := RepartirDevolucion(entradas, d("715"))
	enCajon := decimal.Zero
	for _, p := range partes {
		if p.SaleDelCajon {
			enCajon = enCajon.Add(p.Monto)
		}
	}
	if !enCajon.Equal(d("435")) {
		t.Fatalf("del cajón salen %s, quiere 435 (300 del mostrador + 135 de Didi en efectivo): la tarjeta y el efectivo que se lleva el repartidor no estaban en la caja", enCajon)
	}
}

// Devolver por un método DESACTIVADO se permite. Cobrar con uno inactivo se rechaza porque no debe
// entrar dinero nuevo por ahí; el que ya entró tiene que poder salir por donde entró, o queda
// atrapado y el arqueo nunca cuadra.
func TestSeDevuelvePorUnMetodoDesactivado(t *testing.T) {
	entradas := []CobradoPorMetodo{
		{MetodoID: 7, Nombre: "Tarjeta vieja", Activo: false, Monto: d("150")},
	}
	partes := RepartirDevolucion(entradas, d("150"))
	if len(partes) != 1 || !partes[0].Monto.Equal(d("150")) {
		t.Fatalf("reparto = %+v, quiere devolver los 150 por el método inactivo", partes)
	}
}

// El reparto nunca puede devolver más de lo que entró en total: si alguien pide de más, se acota.
// La validación ya lo rechaza antes, pero el reparto no puede confiar en que lo llamen bien.
func TestElRepartoNoInventaDinero(t *testing.T) {
	entradas := []CobradoPorMetodo{{MetodoID: 1, Nombre: "Efectivo", TocaElCajon: true, Monto: d("100")}}
	total := decimal.Zero
	for _, p := range RepartirDevolucion(entradas, d("999")) {
		total = total.Add(p.Monto)
	}
	if !total.Equal(d("100")) {
		t.Fatalf("el reparto devolvió %s de los 100 que entraron", total)
	}
}

// UNA SEGUNDA DEVOLUCIÓN NO VUELVE A SACAR LO QUE YA SALIÓ POR UN MEDIO (D1, spec 031).
//
// El reparto recibía lo cobrado en bruto por medio. Pedido de $100: $40 en efectivo y $60 con
// tarjeta; se devuelven $40 (salen del efectivo) y luego los $60 restantes. La segunda volvía a
// empezar por el efectivo —«disponible» $40 otra vez— y sacaba otros $40 de billetes y solo $20 de
// tarjeta: el libro decía que salieron $80 en efectivo de $40 que entraron.
func TestLaSegundaDevolucionNoRepiteElPrimerMedio(t *testing.T) {
	entradas := []CobradoPorMetodo{
		{MetodoID: 1, Nombre: "Efectivo", TocaElCajon: true, Monto: d("40"), Refunded: d("40")},
		{MetodoID: 2, Nombre: "Tarjeta", Monto: d("60")},
	}
	partes := RepartirDevolucion(entradas, d("60"))
	if len(partes) != 1 || partes[0].MetodoID != 2 || !partes[0].Monto.Equal(d("60")) {
		t.Fatalf("reparto de los 60 restantes = %+v, quiere 60 por tarjeta y nada en efectivo", partes)
	}

	// A medias: lo que queda del efectivo sale primero, y nunca más de eso.
	entradas[0].Refunded = d("25")
	partes = RepartirDevolucion(entradas, d("75"))
	if len(partes) != 2 || !partes[0].Monto.Equal(d("15")) || !partes[1].Monto.Equal(d("60")) {
		t.Fatalf("reparto de 75 con 25 ya devueltos en efectivo = %+v, quiere 15 de efectivo y 60 de tarjeta", partes)
	}
}

// EL TOPE DE UN RENGLÓN ES LO QUE VALE ESE RENGLÓN, Y NUNCA MÁS DE LO QUE QUEDA DEL PEDIDO (D4).
//
// Con renglón, el tope era lo cobrado del pedido ENTERO menos lo devuelto de ese renglón: un
// platillo de $60 en un pedido de $500 devolvía $500, y otra vez por cada platillo.
func TestElTopeDeUnRenglon(t *testing.T) {
	casos := []struct {
		nombre                                           string
		cobrado, devueltoTotal, importe, devueltoRenglon string
		quiere                                           string
	}{
		{"el platillo, no el pedido", "500", "0", "60", "0", "60"},
		{"lo que queda del platillo", "500", "20", "60", "20", "40"},
		{"un platillo ya devuelto", "500", "60", "60", "60", "0"},
		{"la cuenta ya se devolvió entera", "500", "500", "60", "0", "0"},
		{"queda menos del pedido que del platillo", "500", "470", "60", "0", "30"},
		{"devuelto de más no da negativo", "500", "0", "60", "80", "0"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := LineRefundable(d(c.cobrado), d(c.devueltoTotal), d(c.importe), d(c.devueltoRenglon))
			if !got.Equal(d(c.quiere)) {
				t.Fatalf("tope = %s, quiere %s", got, c.quiere)
			}
		})
	}
}

// DEVOLVER UN PAGO NO PUEDE DEJAR UNA DEVOLUCIÓN SIN COBRO DETRÁS (D2).
//
// Pedido de $100 en efectivo, «Devolver» $40 y luego «Devolver pago» de los $100: el cliente
// recibía $140 por un pedido que pagó con $100, y el pedido volvía a deber.
func TestDevolverUnPagoRespetaLasDevoluciones(t *testing.T) {
	casos := []struct {
		nombre                  string
		quedaDelMedio, devuelto string
		ok                      bool
	}{
		{"sin devoluciones", "0", "0", true},
		{"devolución de $40 y el único pago se va", "0", "40", false},
		{"otro pago del mismo medio la cubre", "50", "40", true},
		{"otro pago del mismo medio no alcanza", "30", "40", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := VoidKeepsRefunds(d(c.quedaDelMedio), d(c.devuelto))
			if c.ok && err != nil {
				t.Fatalf("err = %v, quiere permitido", err)
			}
			if !c.ok && !errors.Is(err, ErrPaymentHasRefunds) {
				t.Fatalf("err = %v, quiere ErrPaymentHasRefunds", err)
			}
		})
	}
}

// CANCELAR CON DEVOLUCIÓN REGRESA LO QUE QUEDA DE CADA MEDIO, CUENTA Y PROPINA (D9).
//
// Solo se devolvía la cuenta: la propina se quedaba en el esperado del cajón y ningún reparto de
// propinas la contaba, porque el pedido estaba cancelado.
func TestCancelarDevuelveCuentaYPropinaPorMedio(t *testing.T) {
	entradas := []CobradoPorMetodo{
		{MetodoID: 1, Nombre: "Efectivo", TocaElCajon: true, Monto: d("40"), Refunded: d("40"), Tip: d("5")},
		{MetodoID: 2, Nombre: "Tarjeta", Monto: d("60"), Tip: d("10"), TipRefunded: d("4")},
		{MetodoID: 3, Nombre: "Transferencia", Monto: d("20"), Refunded: d("20")},
	}
	partes := SplitCancellationRefund(entradas)
	if len(partes) != 2 {
		t.Fatalf("partes = %+v, quiere dos: efectivo (solo propina) y tarjeta", partes)
	}
	if partes[0].MetodoID != 1 || !partes[0].Monto.IsZero() || !partes[0].Tip.Equal(d("5")) || !partes[0].SaleDelCajon {
		t.Fatalf("efectivo = %+v, quiere 0 de cuenta y 5 de propina, del cajón", partes[0])
	}
	if partes[1].MetodoID != 2 || !partes[1].Monto.Equal(d("60")) || !partes[1].Tip.Equal(d("6")) {
		t.Fatalf("tarjeta = %+v, quiere 60 de cuenta y 6 de propina", partes[1])
	}
}

// NADA POR DEVOLVER SE DICE COMO TAL (spec 029). Devolver sin monto pide «lo que queda», y cuando lo
// que queda es cero el monto llegaba en $0 y rebotaba con «el monto a devolver no es una cantidad de
// dinero»: el operador revisaba un campo que nunca tecleó. Medido en el ambiente de pruebas.
func TestNadaPorDevolverSeDiceComoTal(t *testing.T) {
	err := ValidarDevolucion(d("0"), d("100"), d("100"))
	if !errors.Is(err, ErrNothingLeftToRefund) {
		t.Fatalf("pedido ya devuelto completo: err = %v, quiere ErrNothingLeftToRefund", err)
	}
	if err := ValidateLineRefund(d("0"), d("0")); !errors.Is(err, ErrNothingLeftOnLine) {
		t.Fatalf("renglón ya devuelto: err = %v, quiere ErrNothingLeftOnLine", err)
	}
	if err := ValidateLineRefund(d("61"), d("60")); !errors.Is(err, ErrDevolucionExcede) {
		t.Fatalf("más de lo que queda del renglón: err = %v, quiere ErrDevolucionExcede", err)
	}
	if err := ValidateLineRefund(d("60"), d("60")); err != nil {
		t.Fatalf("lo que queda del renglón debe pasar: %v", err)
	}
	if !errors.Is(ErrNothingLeftOnLine, ErrValidation) || !errors.Is(ErrNothingLeftToRefund, ErrValidation) {
		t.Fatal("los dos tienen que llegar como 4xx")
	}
}
