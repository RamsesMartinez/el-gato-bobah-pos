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
		{MetodoID: 1, Nombre: "Efectivo", EsEfectivo: true, Monto: d("300")},
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

// El efectivo se marca, porque es el único que sale del cajón y hace un movimiento de caja. La
// tarjeta no: ese dinero nunca estuvo en la caja y descontarlo inventaría un faltante.
func TestSoloElEfectivoSaleDelCajon(t *testing.T) {
	entradas := []CobradoPorMetodo{
		{MetodoID: 2, Nombre: "Tarjeta", Monto: d("200")},
		{MetodoID: 1, Nombre: "Efectivo", EsEfectivo: true, Monto: d("300")},
	}
	partes := RepartirDevolucion(entradas, d("500"))
	enCajon := decimal.Zero
	for _, p := range partes {
		if p.SaleDelCajon {
			enCajon = enCajon.Add(p.Monto)
		}
	}
	if !enCajon.Equal(d("300")) {
		t.Fatalf("del cajón salen %s, quiere 300: solo el efectivo", enCajon)
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
	entradas := []CobradoPorMetodo{{MetodoID: 1, Nombre: "Efectivo", EsEfectivo: true, Monto: d("100")}}
	total := decimal.Zero
	for _, p := range RepartirDevolucion(entradas, d("999")) {
		total = total.Add(p.Monto)
	}
	if !total.Equal(d("100")) {
		t.Fatalf("el reparto devolvió %s de los 100 que entraron", total)
	}
}
