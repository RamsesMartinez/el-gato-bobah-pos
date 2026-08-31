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
