package domain

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

// dec y decPtr son los helpers que el paquete ya tiene (purchaseqty_test.go).

func TestResolverDescuento(t *testing.T) {
	casos := []struct {
		nombre     string
		subtotal   string
		monto      *decimal.Decimal
		porcentaje *decimal.Decimal
		quiero     string
		quieroErr  error
	}{
		{nombre: "sin descuento capturado", subtotal: "385.00", quiero: "0"},
		{nombre: "monto tal cual", subtotal: "385.00", monto: decPtr("50"), quiero: "50"},
		{nombre: "porcentaje a pesos", subtotal: "385.00", porcentaje: decPtr("20"), quiero: "77"},
		{
			// El residuo del porcentaje se redondea a centavos y manda el MONTO: si se guardara la
			// fórmula, el ticket que el cliente ya tiene en la mano dejaría de cuadrar en cuanto
			// alguien le agregue algo al pedido.
			nombre: "porcentaje que no divide exacto", subtotal: "100.05", porcentaje: decPtr("33"),
			quiero: "33.02",
		},
		{nombre: "descuento del 100 por ciento deja el total en cero", subtotal: "385.00", porcentaje: decPtr("100"), quiero: "385"},
		{
			// Los dos juntos no es "gana uno": es ambigüedad, y una ambigüedad sobre dinero se
			// rechaza. Dejarla pasar hace que el operador vea una cifra y el servidor cobre otra.
			nombre: "monto y porcentaje juntos", subtotal: "385.00", monto: decPtr("50"), porcentaje: decPtr("20"),
			quieroErr: ErrValidation,
		},
		{nombre: "porcentaje mayor a 100", subtotal: "385.00", porcentaje: decPtr("120"), quieroErr: ErrValidation},
		{nombre: "porcentaje negativo", subtotal: "385.00", porcentaje: decPtr("-5"), quieroErr: ErrValidation},
		{nombre: "monto negativo", subtotal: "385.00", monto: decPtr("-1"), quieroErr: ErrValidation},
		{nombre: "monto absurdo", subtotal: "385.00", monto: decPtr("99999999999"), quieroErr: ErrValidation},
		{
			// 422 y no 400: el dato está bien formado, lo que no se puede es descontar más de lo
			// que se vendió. El mensaje tiene que decir cuál es el máximo, o el operador adivina.
			nombre: "monto mayor que el subtotal", subtotal: "385.00", monto: decPtr("400"),
			quieroErr: ErrDescuentoMayorQueLaVenta,
		},
		{nombre: "monto igual al subtotal", subtotal: "385.00", monto: decPtr("385"), quiero: "385"},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := ResolverDescuento(dec(c.subtotal), c.monto, c.porcentaje)
			if c.quieroErr != nil {
				if !errors.Is(err, c.quieroErr) {
					t.Fatalf("quería %v, obtuve %v (descuento %s)", c.quieroErr, err, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("no esperaba error: %v", err)
			}
			if !got.Equal(dec(c.quiero)) {
				t.Fatalf("descuento: quería %s, obtuve %s", c.quiero, got)
			}
		})
	}
}

// El orden importa: el descuento se aplica a los productos y el envío se suma DESPUÉS. Al revés,
// una promoción del menú le recortaría al repartidor lo que se le paga por llevar la comida.
func TestElDescuentoNoSeComeElEnvio(t *testing.T) {
	o := BuiltOrder{Subtotal: dec("385.00"), Total: dec("385.00")}

	o, err := AplicarDescuento(o, dec("50"))
	if err != nil {
		t.Fatalf("aplicar descuento: %v", err)
	}
	if !o.Total.Equal(dec("335")) {
		t.Fatalf("total tras el descuento: quería 335, obtuve %s", o.Total)
	}

	o, err = ApplyDeliveryFee(o, dec("35"), true)
	if err != nil {
		t.Fatalf("aplicar envío: %v", err)
	}
	if !o.Total.Equal(dec("370")) {
		t.Fatalf("el envío se sumó sobre el total descontado: quería 370, obtuve %s. "+
			"Si dio 420, ApplyDeliveryFee volvió a partir del subtotal y BORRÓ el descuento", o.Total)
	}
	if !o.Discount.Equal(dec("50")) {
		t.Fatalf("el descuento se perdió al sumar el envío: %s", o.Discount)
	}
}

// Sin descuento, nada cambia: es el pedido de todos los días y no puede pagar el costo de una
// feature que no usa.
func TestSinDescuentoElTotalEsElDeSiempre(t *testing.T) {
	o := BuiltOrder{Subtotal: dec("385.00"), Total: dec("385.00")}
	o, err := AplicarDescuento(o, decimal.Zero)
	if err != nil {
		t.Fatalf("aplicar descuento cero: %v", err)
	}
	if !o.Total.Equal(dec("385")) || !o.Discount.IsZero() {
		t.Fatalf("total %s, descuento %s", o.Total, o.Discount)
	}
}

// El piso en cero es la defensa de último momento: el servidor ya rechaza un descuento mayor que el
// subtotal al capturarlo, pero cancelar un renglón después puede dejar el subtotal por debajo.
// Un total negativo cobraría al revés y ningún reporte lo diría.
func TestElTotalNuncaEsNegativo(t *testing.T) {
	o := BuiltOrder{Subtotal: dec("20.00"), Total: dec("20.00")}
	o, err := AplicarDescuento(o, dec("50"))
	if err != nil {
		t.Fatalf("aplicar descuento: %v", err)
	}
	if !o.Total.IsZero() {
		t.Fatalf("total con piso en cero: quería 0, obtuve %s", o.Total)
	}
	if !o.Discount.Equal(dec("50")) {
		t.Fatalf("el descuento registrado no se recorta, solo el total: %s", o.Discount)
	}
}

// UN PEDIDO DESCONTADO AL 100% TIENE QUE PODER QUEDAR SALDADO.
//
// `PedidoSaldado` exigía que el total fuera positivo, y con eso una cortesía completa dejaba el
// pedido con el ícono naranja y «Falta cobrar $0.00» para siempre: no había forma de saldarlo
// porque no había nada que cobrar. Era defendible mientras llegar a cero exigiera que cada producto
// costara $0; el descuento lo pone a un toque de distancia.
func TestUnPedidoDescontadoPorCompletoQuedaSaldado(t *testing.T) {
	o := BuiltOrder{Subtotal: dec("385.00"), Total: dec("385.00")}
	o, err := AplicarDescuento(o, dec("385"))
	if err != nil {
		t.Fatalf("aplicar descuento: %v", err)
	}
	if !o.Total.IsZero() {
		t.Fatalf("total tras descontar todo: %s", o.Total)
	}
	if !PedidoSaldado(decimal.Zero, o.Total) {
		t.Fatal("el pedido descontado al 100% NO quedó saldado: el mostrador lo ve como " +
			"«Falta cobrar $0.00» y no hay manera de cerrarlo, porque no hay nada que cobrar")
	}
	if !PorCobrar(o.Total, decimal.Zero).IsZero() {
		t.Fatal("y además dice que falta por cobrar: la lista y el resumen se contradicen")
	}
}
