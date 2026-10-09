package domain

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestPorCobrar(t *testing.T) {
	casos := []struct{ total, pagado, quiere string }{
		{"275", "0", "275"},
		{"275", "100", "175"},
		{"275", "275", "0"},
		// Sobrepagado: el pedido no "debe de menos". Arrastrar el negativo a la suma del tablero
		// lo convertiría en un descuento sobre lo que deben los demás pedidos.
		{"275", "300", "0"},
		{"0.1", "0.05", "0.05"},
		// UN CENTAVO DE MENOS ES DEUDA (spec 031, D16). La tolerancia de un centavo existía por el
		// residuo de dividir $100 en tres; desde la 027 las divisiones le cargan ese residuo al
		// último pago, y la tolerancia solo servía para dar por saldado un cobro tecleado de menos:
		// la venta decía $100 y el corte $99.99, para siempre.
		{"100", "99.99", "0.01"},
		{"100", "99.98", "0.02"},
	}
	for _, c := range casos {
		got := PorCobrar(decimal.RequireFromString(c.total), decimal.RequireFromString(c.pagado))
		if !got.Equal(decimal.RequireFromString(c.quiere)) {
			t.Errorf("PorCobrar(%s, %s) = %s, quiere %s", c.total, c.pagado, got, c.quiere)
		}
	}
}

func TestValidarCobro(t *testing.T) {
	casos := []struct {
		nombre               string
		estado               string
		total, pagado, monto string
		quiere               error
	}{
		{"cobrar todo lo que falta", StatusEntregada, "275", "0", "275", nil},
		{"un abono", StatusAbierta, "275", "0", "100", nil},
		{"completar lo que faltaba", StatusLista, "275", "100", "175", nil},

		// EL CASO CARO: un doble tap sobre "Cobrar $275" registraría $550 de ingreso por comida
		// que se vendió una vez, y el corte cuadraría contra una cifra inventada.
		{"más de lo que falta", StatusEntregada, "275", "0", "276", ErrCobroExcede},
		{"cobrar dos veces", StatusEntregada, "275", "275", "275", ErrPedidoYaPagado},

		{"monto en cero", StatusAbierta, "275", "0", "0", ErrValidation},
		{"monto negativo", StatusAbierta, "275", "0", "-50", ErrValidation},

		// Su dinero ya se decidió: cancelar repuso el stock, reembolsar devolvió el ingreso.
		{"un pedido cancelado", StatusCancelada, "275", "0", "275", ErrPedidoNoCobrable},
		{"un pedido reembolsado", StatusReembolsada, "275", "275", "10", ErrPedidoNoCobrable},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := ValidarCobro(c.estado, decimal.RequireFromString(c.total),
				decimal.RequireFromString(c.pagado), decimal.RequireFromString(c.monto))
			if !errors.Is(err, c.quiere) {
				t.Fatalf("ValidarCobro(%s, %s, %s, %s) = %v, quiere %v",
					c.estado, c.total, c.pagado, c.monto, err, c.quiere)
			}
		})
	}
}

func TestValidarPropina(t *testing.T) {
	casos := []struct {
		nombre         string
		total, propina string
		quiere         error
	}{
		{"sin propina", "250", "0", nil},
		{"una propina normal", "250", "40", nil},
		{"la propina del monto entero", "250", "250", nil},

		// EL CASO CARO, medido: un pedido de $250 aceptaba $9,999 de propina. Esa propina entra al
		// esperado del cajon (ExpectedByMethodForSession suma tip_amount) y a TipsByEmployee, asi
		// que un dedo gordo cierra el turno con un faltante de $9,999 que nadie sabe explicar.
		{"mas que la cuenta entera", "250", "9999", ErrPropinaExcede},
		{"un peso mas que la cuenta", "250", "251", ErrPropinaExcede},

		{"propina negativa", "250", "-10", ErrValidation},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := ValidarPropina(decimal.RequireFromString(c.total), decimal.RequireFromString(c.propina))
			if !errors.Is(err, c.quiere) {
				t.Fatalf("ValidarPropina(%s, %s) = %v, quiere %v", c.total, c.propina, err, c.quiere)
			}
		})
	}
}

// El predicado que cierra el pedido y el que dice si está pagado tienen que ser el mismo. Estaban
// escritos cuatro veces y dos no toleraban el centavo del redondeo: el pedido cerraba y la pantalla
// le seguía viendo deuda.
func TestPedidoSaldado(t *testing.T) {
	casos := []struct {
		nombre        string
		pagado, total string
		quiere        bool
	}{
		{"nada pagado", "0", "250", false},
		{"un abono", "100", "250", false},
		{"justo", "250", "250", true},
		{"de más", "300", "250", true},
		// Un centavo de menos ya no salda (spec 031, D16): el residuo de dividir lo absorbe el último
		// pago, y tolerarlo dejaba la venta y el corte un centavo distintos para siempre.
		{"un centavo de menos", "99.99", "100", false},
		{"dos centavos ya es deuda", "99.98", "100", false},
		// UN PEDIDO EN CERO SÍ ESTÁ SALDADO, y este caso decía lo contrario hasta la feature 022.
		//
		// La regla vieja era defendible mientras llegar a cero exigiera que cada producto de la
		// cuenta costara $0. El descuento lo pone a un toque: una cortesía del 100 % dejaba el
		// pedido con «Falta cobrar $0.00» en naranja para siempre, sin forma de saldarlo porque no
		// había nada que cobrar.
		{"un pedido en cero", "0", "0", true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := PedidoSaldado(decimal.RequireFromString(c.pagado), decimal.RequireFromString(c.total))
			if got != c.quiere {
				t.Fatalf("PedidoSaldado(%s, %s) = %v, quiere %v", c.pagado, c.total, got, c.quiere)
			}
			// Y lo que falta por cobrar tiene que ser consistente con él: si está saldado, no falta.
			falta := PorCobrar(decimal.RequireFromString(c.total), decimal.RequireFromString(c.pagado))
			if c.quiere && !falta.IsZero() {
				t.Fatalf("está saldado pero PorCobrar dice que faltan %s: dos predicados sobre la misma cifra", falta)
			}
		})
	}
}
