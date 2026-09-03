package domain

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func TestValidarEntrega(t *testing.T) {
	casos := []struct {
		nombre   string
		linea    LineaEntrega
		cantidad decimal.Decimal
		quiere   error
	}{
		{"entregar todo lo que falta", LineaEntrega{Cantidad: dec("5")}, dec("5"), nil},
		{"entregar una parte", LineaEntrega{Cantidad: dec("5")}, dec("3"), nil},
		{"completar lo que faltaba", LineaEntrega{Cantidad: dec("5"), Entregado: dec("3")}, dec("2"), nil},
		{"fracción de kilo", LineaEntrega{Cantidad: dec("1.5")}, dec("0.75"), nil},
		// El caso que motiva todo esto: de 5 alitas salieron 3, y alguien vuelve a marcar 3.
		// Sin este tope el renglón diría 6 de 5 y el pedido se cerraría con comida sin entregar.
		{"más de lo que falta", LineaEntrega{Cantidad: dec("5"), Entregado: dec("3")}, dec("3"), ErrEntregaExcede},
		{"más de lo pedido", LineaEntrega{Cantidad: dec("5")}, dec("6"), ErrEntregaExcede},
		{"un renglón ya completo", LineaEntrega{Cantidad: dec("5"), Entregado: dec("5")}, dec("1"), ErrEntregaExcede},
		{"cero no es entregar", LineaEntrega{Cantidad: dec("5")}, dec("0"), ErrEntregaInvalida},
		{"negativo no deshace", LineaEntrega{Cantidad: dec("5"), Entregado: dec("3")}, dec("-1"), ErrEntregaInvalida},
		// Un renglón cancelado no se entrega: la comida no se hizo.
		{"renglón cancelado", LineaEntrega{Cantidad: dec("5"), Cancelada: true}, dec("1"), ErrLineaCancelada},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := ValidarEntrega(c.linea, c.cantidad)
			if !errors.Is(err, c.quiere) {
				t.Fatalf("ValidarEntrega(%v, %s) = %v, quiere %v", c.linea, c.cantidad, err, c.quiere)
			}
		})
	}
}
func TestTodoEntregado(t *testing.T) {
	casos := []struct {
		nombre string
		lineas []LineaEntrega
		quiere bool
	}{
		{"nada entregado", []LineaEntrega{{Cantidad: dec("2")}}, false},
		{"todo entregado", []LineaEntrega{{Cantidad: dec("2"), Entregado: dec("2")}}, true},
		{"falta un renglón", []LineaEntrega{
			{Cantidad: dec("2"), Entregado: dec("2")},
			{Cantidad: dec("1")},
		}, false},
		{"falta parte de un renglón", []LineaEntrega{
			{Cantidad: dec("5"), Entregado: dec("3")},
		}, false},
		// Lo cancelado no se entrega nunca, así que no puede impedir que el pedido se cierre. Sin
		// esta regla un renglón cancelado dejaría el pedido abierto para siempre.
		{"lo que falta está cancelado", []LineaEntrega{
			{Cantidad: dec("2"), Entregado: dec("2")},
			{Cantidad: dec("1"), Cancelada: true},
		}, true},
		// Un pedido sin renglones vivos no está "entregado": no hay nada que se le haya dado a
		// nadie. Cerrarlo por vacío convertiría un pedido cancelado renglón a renglón en una venta.
		{"todo cancelado", []LineaEntrega{{Cantidad: dec("1"), Cancelada: true}}, false},
		{"sin renglones", nil, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := TodoEntregado(c.lineas); got != c.quiere {
				t.Fatalf("TodoEntregado(%v) = %v, quiere %v", c.lineas, got, c.quiere)
			}
		})
	}
}

// Cancelar un pedido repone el stock de TODAS sus líneas. Si algo ya salió a la calle, reponerlo
// es inventar inventario: el sistema creería tener comida que ya se comieron.
func TestHayEntregaParcial(t *testing.T) {
	casos := []struct {
		nombre string
		lineas []LineaEntrega
		quiere bool
	}{
		{"nada salió", []LineaEntrega{{Cantidad: dec("5")}, {Cantidad: dec("2")}}, false},
		{"salió parte de un renglón", []LineaEntrega{{Cantidad: dec("5"), Entregado: dec("1")}}, true},
		{"salió un renglón entero", []LineaEntrega{
			{Cantidad: dec("5"), Entregado: dec("5")},
			{Cantidad: dec("2")},
		}, true},
		{"lo entregado estaba cancelado", []LineaEntrega{
			{Cantidad: dec("5"), Entregado: dec("5"), Cancelada: true},
		}, true}, // salió de la cocina aunque después se cancelara el renglón
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := HayEntregaParcial(c.lineas); got != c.quiere {
				t.Fatalf("HayEntregaParcial(%v) = %v, quiere %v", c.lineas, got, c.quiere)
			}
		})
	}
}
