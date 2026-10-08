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

// Sin productos vivos no hay nada que entregar: cerrarlo como entregado sería una venta de $0.
func TestCanDeliverAllNeedsALiveProduct(t *testing.T) {
	cases := []struct {
		name  string
		lines []LineaEntrega
		want  error
	}{
		{"sin renglones", nil, ErrNoProducts},
		{"todos quitados", []LineaEntrega{{ID: 1, Cantidad: d("1"), Cancelada: true}}, ErrNoProducts},
		{"uno vivo", []LineaEntrega{{ID: 1, Cantidad: d("1"), Cancelada: true}, {ID: 2, Cantidad: d("2")}}, nil},
	}
	for _, c := range cases {
		if err := CanDeliverAll(c.lines); !errors.Is(err, c.want) || (c.want == nil && err != nil) {
			t.Errorf("%s: CanDeliverAll = %v, quiere %v", c.name, err, c.want)
		}
	}
}

// Qué hace «Quitar lo que falta» según el pedido. Un pedido vacío con pagos no se cancela: ese
// dinero saldría del corte sin rastro; uno sin nada pendiente no tiene qué quitar.
func TestPlanCancelPending(t *testing.T) {
	vivo := LineaEntrega{ID: 1, Cantidad: d("2"), Entregado: d("2")}
	pendiente := LineaEntrega{ID: 2, Cantidad: d("2"), Entregado: d("1")}
	quitado := LineaEntrega{ID: 3, Cantidad: d("1"), Cancelada: true}
	cases := []struct {
		name      string
		lines     []LineaEntrega
		paid      string
		wantEmpty bool
		wantErr   error
	}{
		{"con pendientes", []LineaEntrega{vivo, pendiente}, "0", false, nil},
		{"con pendientes y pagos", []LineaEntrega{pendiente}, "50", false, nil},
		{"todo lo vivo entregado", []LineaEntrega{vivo, quitado}, "0", false, ErrNothingPending},
		{"sin productos y sin pagos", []LineaEntrega{quitado}, "0", true, nil},
		{"sin renglones", nil, "0", true, nil},
		{"sin productos y con pagos", []LineaEntrega{quitado}, "0.01", false, ErrOrderHasPayments},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			empty, err := PlanCancelPending(c.lines, d(c.paid))
			if !errors.Is(err, c.wantErr) || (c.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, quiere %v", err, c.wantErr)
			}
			if empty != c.wantEmpty {
				t.Fatalf("cerrar vacío = %v, quiere %v", empty, c.wantEmpty)
			}
		})
	}
}

// Quitar no puede dejar el total por debajo de lo ya cobrado: ese dinero se quedaría sin venta que
// lo explique, y el corte lo contaría como ingreso de un pedido que ya no lo vale.
func TestRemovalCannotLeaveTheOrderOverpaid(t *testing.T) {
	if err := RemovalKeepsPayments(d("70"), d("70")); err != nil {
		t.Fatalf("total igual a lo pagado = %v, quiere nil", err)
	}
	if err := RemovalKeepsPayments(d("70"), d("0")); err != nil {
		t.Fatalf("sin pagos = %v, quiere nil", err)
	}
	if err := RemovalKeepsPayments(d("69.99"), d("70")); !errors.Is(err, ErrOrderWouldBeOverpaid) {
		t.Fatalf("total bajo lo pagado = %v, quiere ErrOrderWouldBeOverpaid", err)
	}
}

// El motivo de quitar es obligatorio, y su falta se dice en palabras de quien opera.
func TestReasonToRemove(t *testing.T) {
	if _, err := ReasonToRemove(" \u200b "); !errors.Is(err, ErrValidation) || err.Error() != "datos inválidos: Elige por qué se quitan" {
		t.Fatalf("motivo vacío = %v, quiere «Elige por qué se quitan»", err)
	}
	if m, err := ReasonToRemove("  Ya no lo quiere "); err != nil || m != "Ya no lo quiere" {
		t.Fatalf("motivo = %q, %v", m, err)
	}
}
