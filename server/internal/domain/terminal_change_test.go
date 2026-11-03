package domain

import (
	"errors"
	"strings"
	"testing"
)

// Los bordes de corregir la terminal de un pago. El caso que lo motivó: un débito
// se capturó en Terminal A y se cobró en Terminal B, y hubo que corregirlo a mano en la base.
func TestValidateTerminalChange(t *testing.T) {
	heyBanco, termB, konfio := int64(2), int64(3), int64(4)
	activas := []TerminalOption{{ID: heyBanco, Name: "Terminal A"}, {ID: termB, Name: "Terminal B"}}
	base := func() TerminalChange {
		return TerminalChange{MethodKind: PaymentKindCard, OrderStatus: StatusEntregada, Current: &heyBanco,
			From: &heyBanco, To: termB, Active: activas, Reason: "Se cobró en otra terminal"}
	}
	cases := []struct {
		name string
		edit func(*TerminalChange)
		want error // nil = válido
	}{
		{"capturado en A, cobrado en B", func(*TerminalChange) {}, nil},
		{"efectivo no tiene terminal", func(c *TerminalChange) { c.MethodKind = "efectivo" }, ErrValidation},
		{"pago de plataforma", func(c *TerminalChange) { c.MethodKind = "plataforma" }, ErrValidation},
		{"pedido cancelado", func(c *TerminalChange) { c.OrderStatus = StatusCancelada }, ErrConflict},
		{"destino igual al actual no deja un evento vacío", func(c *TerminalChange) { c.To = heyBanco }, ErrValidation},
		{"destino archivado o de otra sucursal", func(c *TerminalChange) { c.To = konfio }, ErrValidation},
		{"otro cambio llegó antes", func(c *TerminalChange) { c.From = &termB }, ErrConflict},
		{"la pantalla creía «Sin terminal» y ya tiene una", func(c *TerminalChange) { c.From = nil }, ErrConflict},
		{"pago sin terminal se corrige", func(c *TerminalChange) { c.Current, c.From = nil, nil }, nil},
		{"la pantalla creía una terminal y no tiene", func(c *TerminalChange) { c.Current = nil }, ErrConflict},
		{"motivo vacío", func(c *TerminalChange) { c.Reason = "" }, ErrValidation},
		{"motivo de puros espacios", func(c *TerminalChange) { c.Reason = "   \t " }, ErrValidation},
		{"motivo de 2 letras", func(c *TerminalChange) { c.Reason = " ab " }, ErrValidation},
		{"motivo de 3 letras con acento", func(c *TerminalChange) { c.Reason = "ñáé" }, nil},
		{"motivo de 200 letras", func(c *TerminalChange) { c.Reason = strings.Repeat("é", 200) }, nil},
		{"motivo de 201 letras", func(c *TerminalChange) { c.Reason = strings.Repeat("é", 201) }, ErrValidation},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := base()
			c.edit(&in)
			got, reason, err := ValidateTerminalChange(in)
			if c.want == nil {
				if err != nil {
					t.Fatalf("rechazó un cambio válido: %v", err)
				}
				if got.ID != in.To {
					t.Errorf("terminal destino = %d, quiere %d", got.ID, in.To)
				}
				if reason != strings.TrimSpace(in.Reason) {
					t.Errorf("motivo = %q, quiere sin espacios a los lados", reason)
				}
				return
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, quiere %v", err, c.want)
			}
		})
	}
}

// El nombre que se guarda en el pago sale de la lista de activas, no de lo que mande la pantalla:
// una tableta con el nombre viejo en caché no debe reescribir el pasado con él.
func TestValidateTerminalChangeTakesTheNameFromTheActiveList(t *testing.T) {
	cur := int64(2)
	got, _, err := ValidateTerminalChange(TerminalChange{MethodKind: PaymentKindCard, OrderStatus: StatusEntregada,
		Current: &cur, From: &cur, To: 3, Reason: "Se cobró en otra terminal",
		Active: []TerminalOption{{ID: 2, Name: "Terminal A"}, {ID: 3, Name: "Terminal B"}}})
	if err != nil || got.Name != "Terminal B" {
		t.Fatalf("got %+v, %v; quiere Terminal B", got, err)
	}
}

// El tipo de método viene de la base (payment_methods.kind) como texto. Un typo en la constante
// rechazaría TODO cambio de terminal con «solo un pago con tarjeta tiene terminal».
func TestPaymentKindCardIsTheDatabaseValue(t *testing.T) {
	if PaymentKindCard != "tarjeta" {
		t.Fatalf("PaymentKindCard = %q; la base guarda «tarjeta»", PaymentKindCard)
	}
}
