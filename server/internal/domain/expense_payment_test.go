package domain

import (
	"errors"
	"testing"
	"time"
)

func TestExpensePaymentTarget(t *testing.T) {
	oct31 := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	oct15 := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	open := &PaymentRegister{CountsCash: true, OpenShiftDay: &oct31}
	closed := &PaymentRegister{CountsCash: true}
	external := &PaymentRegister{CountsCash: false}
	cases := []struct {
		name         string
		isCash       bool
		reg          *PaymentRegister
		paidOn       time.Time
		outsideCount bool
		wantShift    bool
		wantErr      error
	}{
		{"efectivo sin caja se rechaza", true, nil, oct31, false, false, ErrPaymentNeedsRegister},
		{"efectivo a caja sin arqueo no pide turno ni confirmación", true, external, oct15, false, false, nil},
		{"efectivo del día del turno entra al corte", true, open, oct31, false, true, nil},
		{"efectivo a caja con arqueo cerrada pide confirmación", true, closed, oct31, false, false, ErrExpenseOutsideCount},
		{"efectivo a caja con arqueo cerrada confirmado queda fuera", true, closed, oct31, true, false, nil},
		{"efectivo de otro día con turno abierto pide confirmación: no salió del cajón de hoy", true, open, oct15, false, false, ErrExpenseOutsideCount},
		{"efectivo de otro día confirmado no entra al turno abierto", true, open, oct15, true, false, nil},
		{"confirmar en el caso normal no lo saca del corte", true, open, oct31, true, true, nil},
		{"turno del 31-oct abierto pasada la medianoche: un pago del 1-nov no es de ese turno", true, open, oct31.AddDate(0, 0, 1), false, false, ErrExpenseOutsideCount},
		{"tarjeta sin caja sigue permitida", false, nil, oct31, false, false, nil},
		{"tarjeta con caja abierta se atribuye al corte como hoy", false, open, oct15, false, true, nil},
		{"tarjeta con caja cerrada no pide confirmación", false, closed, oct31, false, false, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ExpensePaymentTarget(c.isCash, c.reg, c.paidOn, c.outsideCount)
			if !errors.Is(err, c.wantErr) || (c.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if got != c.wantShift {
				t.Fatalf("entra al corte = %v, want %v", got, c.wantShift)
			}
		})
	}
}
