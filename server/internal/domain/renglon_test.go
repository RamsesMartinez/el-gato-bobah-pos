package domain

import (
	"errors"
	"testing"
	"time"
)

// EL INSUMO VUELVE SOLO SI LA COMIDA NO SE HIZO, Y LO DECIDE EL SISTEMA.
//
// No se le pregunta al cajero: `order_lines.enviado_a_cocina_at` ya responde. NULL = no salió, el
// insumo vuelve. Con fecha = ya está en la plancha, y reponer inventariaría algo que se consumió.
//
// Preguntárselo al operador sería pedirle que decida en dos segundos algo que el sistema ya sabe, y
// la respuesta equivocada descuadra el almacén sin que nadie se entere.
func TestReponeInventarioSoloSiNoSalioACocina(t *testing.T) {
	salio := time.Date(2026, 9, 3, 20, 0, 0, 0, time.UTC)

	if !ReponeInventario(nil) {
		t.Fatal("un renglón que nunca salió a cocina debe reponer: la comida no se hizo")
	}
	if ReponeInventario(&salio) {
		t.Fatal("un renglón que ya salió a cocina NO debe reponer: el insumo se consumió")
	}
}

// UN RENGLÓN YA ENTREGADO NO SE CANCELA.
//
// Cancelarlo bajaría el total de un pedido del que el cliente ya se llevó la comida. Lo que se hace
// con lo entregado es devolver el dinero, no borrar el renglón — son dos operaciones distintas y
// confundirlas es cómo se pierde el rastro de lo que sí salió.
func TestPuedeCancelarRenglon(t *testing.T) {
	casos := []struct {
		nombre              string
		estado              string
		cantidad, entregado string
		quiere              bool
	}{
		{"pendiente en un pedido abierto", StatusAbierta, "2", "0", true},
		{"pendiente en un pedido listo", StatusLista, "2", "0", true},
		{"ya entregado del todo", StatusAbierta, "2", "2", false},
		{"entregado a medias", StatusAbierta, "2", "1", false},
		// Un pedido que ya terminó no admite que se le muevan renglones: su dinero ya se clasificó.
		{"en un pedido cancelado", StatusCancelada, "2", "0", false},
		{"en un pedido reembolsado", StatusReembolsada, "2", "0", false},
		{"en un pedido entregado", StatusEntregada, "2", "0", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := PuedeCancelarRenglon(c.estado, d(c.cantidad), d(c.entregado))
			if c.quiere && err != nil {
				t.Fatalf("debía poderse cancelar, fue %v", err)
			}
			if !c.quiere && err == nil {
				t.Fatal("debía rechazarse y pasó")
			}
		})
	}
}

// El renglón entregado se rechaza con SU error, para que la pantalla pueda decir qué hacer en vez de
// un "no se pudo" que manda a adivinar.
func TestUnRenglonEntregadoSeRechazaConSuPropioError(t *testing.T) {
	if err := PuedeCancelarRenglon(StatusAbierta, d("2"), d("2")); !errors.Is(err, ErrRenglonYaEntregado) {
		t.Fatalf("err = %v, quiere ErrRenglonYaEntregado", err)
	}
}
