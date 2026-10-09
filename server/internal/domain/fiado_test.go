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
