package domain

import (
	"errors"
	"strings"
	"testing"
)

// NO HAY FIADOS (decisión del dueño, 2026-10-09). La caja no cierra con un pedido entregado que
// debe, y el mensaje nombra cada uno con lo que debe: un «hay pedidos sin cobrar» a secas manda al
// operador a recorrer la fila comparando, justo cuando cierra y con prisa.
func TestNoOwingOrders(t *testing.T) {
	if err := NoOwingOrders(nil); err != nil {
		t.Fatalf("sin pedidos que deban, la caja cierra: err = %v", err)
	}

	err := NoOwingOrders([]OwingOrder{
		{ID: 1, Number: 12, Name: "Persa", Total: d("120"), Paid: d("55")},
		{ID: 2, Number: 13, Total: d("20")},
	})
	if !errors.Is(err, ErrUnpaidOrders) {
		t.Fatalf("err = %v, quiere ErrUnpaidOrders", err)
	}
	for _, want := range []string{"Persa (#12) debe $65.00", "#13 debe $20.00", "Cóbralos o cancélalos"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("el mensaje %q no dice %q", err.Error(), want)
		}
	}
	if errors.Is(err, ErrConflict) {
		t.Error("no envuelve ErrConflict: su mensaje saldría con el prefijo «conflicto» si alguien lo encadena")
	}

	// Un pedido cubierto al centavo no debe nada: el MISMO predicado que lo da por saldado.
	if err := NoOwingOrders([]OwingOrder{{Number: 14, Total: d("50"), Paid: d("50")}}); err != nil {
		t.Fatalf("un pedido saldado no bloquea: err = %v", err)
	}
}

// «CANCELAR LO QUE FALTA» (dueño, 2026-10-09, opción A). Un entregado pagado a medias cuyo cliente se
// fue: lo pagado se queda como venta y lo que falta se da por perdido con su motivo. Solo con pagos
// (sin pagos se cancela el pedido entero) y solo si debe algo.
func TestWriteOffRemainder(t *testing.T) {
	casos := []struct {
		nombre      string
		status      string
		total, paid string
		want        string
		err         error
	}{
		{"pagó una parte", StatusEntregada, "100", "40", "60", nil},
		{"sin pagos se cancela entero", StatusEntregada, "100", "0", "", ErrWriteOffWithoutPayments},
		{"pagado no debe nada", StatusEntregada, "100", "100", "", ErrCancelDeliveredPaid},
		{"en cocina todavía no", StatusLista, "100", "40", "", ErrWriteOffNotDelivered},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := WriteOffRemainder(c.status, d(c.total), d(c.paid))
			if c.err != nil {
				if !errors.Is(err, c.err) {
					t.Fatalf("err = %v, quiere %v", err, c.err)
				}
				return
			}
			if err != nil || !got.Equal(d(c.want)) {
				t.Fatalf("= %s, %v; quiere %s", got, err, c.want)
			}
		})
	}
}

// Lo perdido deja de deberse: un pedido con su resto dado por perdido no bloquea el cierre. Pero NO
// es un cobro: Owed lo resta de lo que falta y nada más.
func TestOwedSubtractsTheWriteOff(t *testing.T) {
	if got := Owed(d("100"), d("40"), d("60")); !got.IsZero() {
		t.Fatalf("Owed = %s, quiere 0", got)
	}
	if got := Owed(d("100"), d("40"), d("0")); !got.Equal(d("60")) {
		t.Fatalf("Owed = %s, quiere 60", got)
	}
	if err := NoOwingOrders([]OwingOrder{{Number: 1, Total: d("100"), Paid: d("40"), WrittenOff: d("60")}}); err != nil {
		t.Fatalf("un pedido con su resto perdido bloqueó el cierre: %v", err)
	}
}

// Cancelar un entregado que debe es la salida de «se fue sin pagar». Solo sin pagos: con un cobro
// adentro, cancelarlo sacaría de Ventas un dinero que sí está en el cajón.
func TestCanCancelDelivered(t *testing.T) {
	casos := []struct {
		nombre      string
		total, paid string
		want        error
	}{
		{"debe todo", "100", "0", nil},
		{"pagó una parte", "100", "40", ErrCancelDeliveredWithPayments},
		{"pagado", "100", "100", ErrCancelDeliveredPaid},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := CanCancelDelivered(d(c.total), d(c.paid))
			if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
				t.Fatalf("err = %v, quiere %v", err, c.want)
			}
		})
	}
}
