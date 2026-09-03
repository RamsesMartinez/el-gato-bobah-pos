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
		// El residuo de una división en tres: 33.33 x 3 = 99.99. Quien CIERRA el pedido tolera ese
		// centavo (PagosCubren), así que quien muestra la deuda tiene que tolerarlo también, o el
		// tablero le sigue viendo un centavo a un pedido que ya cerró — y al día siguiente el
		// pedido desaparece de la vista con la deuda abierta. Dos predicados sobre la misma cifra
		// es exactamente lo que el corolario del principio III prohibe.
		{"100", "99.99", "0"},
		// Un centavo más de diferencia SÍ es deuda: la tolerancia es del redondeo, no una condona.
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
		// El residuo de dividir en tres: 33.33 x 3 = 99.99.
		{"un centavo de menos por el redondeo", "99.99", "100", true},
		{"dos centavos ya es deuda", "99.98", "100", false},
		// Un pedido de $0 no está pagado: no tiene nada que pagar.
		{"un pedido en cero", "0", "0", false},
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
