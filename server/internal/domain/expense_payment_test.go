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
			got, err := ExpensePaymentTarget(c.isCash, c.reg, c.paidOn, c.paidOn, PaymentChoice{OutsideCount: c.outsideCount})
			if !errors.Is(err, c.wantErr) || (c.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if (got == TargetShift) != c.wantShift {
				t.Fatalf("entra al corte = %v, want %v", got, c.wantShift)
			}
		})
	}
}

// LA HORA DEL PAGO (spec 048). Hoy: la del reloj. Otro día: la medianoche de ese día en la zona del
// negocio; no hay hora real de un pago capturado días después. A las 18:00 de México ya es mañana en
// UTC: con la zona equivocada, un pago de hoy saldría con la hora de la medianoche.
func TestPaymentTime(t *testing.T) {
	mx := LoadBusinessLocation("America/Mexico_City")
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	now := time.Date(2026, 11, 1, 1, 30, 0, 0, time.UTC) // 31-oct 19:30 en México (UTC-6)
	cases := []struct {
		name   string
		paidOn time.Time
		want   time.Time
	}{
		{"hoy en el negocio es la hora del reloj aunque en UTC ya sea mañana", day(2026, 10, 31), now},
		{"otro día es su medianoche local", day(2026, 10, 30), time.Date(2026, 10, 30, 6, 0, 0, 0, time.UTC)},
		{"el día UTC de hoy no es hoy en el negocio", day(2026, 11, 1), time.Date(2026, 11, 1, 6, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PaymentTime(c.paidOn, now, mx); !got.Equal(c.want) {
				t.Fatalf("PaymentTime = %s, quiere %s", got.UTC(), c.want.UTC())
			}
		})
	}
}

// EL DESTINO DE UN PAGO CON LA CASILLA (spec 048): fuera, corte o pendiente, y los dos 409.
func TestExpensePaymentTargetWithCountBox(t *testing.T) {
	day := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	opened := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	lastClose := time.Date(2026, 10, 31, 4, 0, 0, 0, time.UTC)
	open := &PaymentRegister{CountsCash: true, OpenShiftDay: &day, OpenShiftOpenedAt: &opened, LastClose: &lastClose}
	closed := &PaymentRegister{CountsCash: true, LastClose: &lastClose}
	never := &PaymentRegister{CountsCash: true}
	external := &PaymentRegister{CountsCash: false}
	yes, no := true, false
	later := opened.Add(2 * time.Hour)
	beforeOpen := opened.Add(-2 * time.Hour)
	beforeClose := lastClose.Add(-time.Hour)
	nextDay := day.AddDate(0, 0, 1)
	cases := []struct {
		name    string
		isCash  bool
		reg     *PaymentRegister
		paidOn  time.Time
		paidAt  time.Time
		choice  PaymentChoice
		want    PaymentTarget
		wantErr error
	}{
		{"desmarcada con la caja abierta queda fuera sin preguntar", true, open, day, later, PaymentChoice{IncludeInCount: &no}, TargetOutside, nil},
		{"desmarcada con la caja cerrada queda fuera sin preguntar", true, closed, day, later, PaymentChoice{IncludeInCount: &no}, TargetOutside, nil},
		{"marcada del día del turno entra al corte", true, open, day, later, PaymentChoice{IncludeInCount: &yes}, TargetShift, nil},
		{"marcada del día del turno pero con hora antes de su apertura no entra: es de otro corte del mismo día", true, open, day, beforeOpen, PaymentChoice{IncludeInCount: &yes, OutsideCount: true}, TargetOutside, ErrExpenseBeforeLastClose},
		{"marcada de otro día con hora dentro del turno entra al corte", true, open, nextDay, later, PaymentChoice{IncludeInCount: &yes}, TargetShift, nil},
		{"marcada con hora antes de la apertura del turno abierto: ese corte ya tiene ventana", true, open, day.AddDate(0, 0, -1), beforeOpen, PaymentChoice{IncludeInCount: &yes, OutsideCount: true}, TargetOutside, ErrExpenseBeforeLastClose},
		{"marcada con la caja cerrada pide confirmación", true, closed, day, later, PaymentChoice{IncludeInCount: &yes}, TargetOutside, ErrExpenseOutsideCount},
		{"marcada y confirmada con la caja cerrada queda pendiente", true, closed, day, later, PaymentChoice{IncludeInCount: &yes, OutsideCount: true}, TargetPending, nil},
		{"marcada y confirmada en una caja que nunca cerró queda pendiente", true, never, day, beforeClose, PaymentChoice{IncludeInCount: &yes, OutsideCount: true}, TargetPending, nil},
		{"antes del último cierre no entra a ningún corte, aunque se confirme", true, closed, day, beforeClose, PaymentChoice{IncludeInCount: &yes, OutsideCount: true}, TargetOutside, ErrExpenseBeforeLastClose},
		{"justo a la hora del último cierre tampoco", true, closed, day, lastClose, PaymentChoice{IncludeInCount: &yes, OutsideCount: true}, TargetOutside, ErrExpenseBeforeLastClose},
		{"antes del último cierre y sin confirmar dice que no puede entrar", true, closed, day, beforeClose, PaymentChoice{IncludeInCount: &yes}, TargetOutside, ErrExpenseBeforeLastClose},
		{"legado 047: confirmado sin casilla es fuera", true, closed, day, later, PaymentChoice{OutsideCount: true}, TargetOutside, nil},
		{"legado 047: sin casilla ni confirmación pregunta como antes", true, closed, day, beforeClose, PaymentChoice{}, TargetOutside, ErrExpenseOutsideCount},
		{"legado 047: otro día con turno abierto no entra aunque su hora caiga en él", true, open, nextDay, later, PaymentChoice{}, TargetOutside, ErrExpenseOutsideCount},
		{"legado 047: el día del turno entra", true, open, day, later, PaymentChoice{}, TargetShift, nil},
		{"caja sin arqueo ignora la casilla", true, external, day, later, PaymentChoice{IncludeInCount: &yes}, TargetOutside, nil},
		{"tarjeta con turno abierto se atribuye al corte", false, open, day, later, PaymentChoice{IncludeInCount: &no}, TargetShift, nil},
		{"tarjeta con caja cerrada queda fuera sin preguntar", false, closed, day, later, PaymentChoice{IncludeInCount: &yes}, TargetOutside, nil},
		{"efectivo sin caja se rechaza", true, nil, day, later, PaymentChoice{IncludeInCount: &yes}, TargetOutside, ErrPaymentNeedsRegister},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ExpensePaymentTarget(c.isCash, c.reg, c.paidOn, c.paidAt, c.choice)
			if !errors.Is(err, c.wantErr) || (c.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, quiere %v", err, c.wantErr)
			}
			if got != c.want {
				t.Fatalf("destino = %v, quiere %v", got, c.want)
			}
		})
	}
}

// LA HORA DE APERTURA MOVIDA (spec 048): nunca futura, nunca en o antes del último cierre.
func TestValidBackdatedOpening(t *testing.T) {
	now := time.Date(2026, 11, 1, 18, 0, 0, 0, time.UTC)
	lastClose := now.Add(-10 * time.Hour)
	cases := []struct {
		name      string
		openedAt  time.Time
		lastClose *time.Time
		ok        bool
	}{
		{"una hora después del último cierre", lastClose.Add(time.Hour), &lastClose, true},
		{"ahora mismo", now, &lastClose, true},
		{"futura", now.Add(time.Minute), &lastClose, false},
		{"igual al último cierre", lastClose, &lastClose, false},
		{"antes del último cierre", lastClose.Add(-time.Minute), &lastClose, false},
		{"caja que nunca cerró acepta hasta 24 h atrás", now.Add(-23 * time.Hour), nil, true},
		{"caja que nunca cerró no acepta más de 24 h atrás", now.Add(-25 * time.Hour), nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidBackdatedOpening(c.openedAt, now, c.lastClose)
			if c.ok && err != nil {
				t.Fatalf("err = %v, quiere nil", err)
			}
			if !c.ok && !errors.Is(err, ErrInvalidOpeningTime) {
				t.Fatalf("err = %v, quiere ErrInvalidOpeningTime", err)
			}
			if !c.ok && !errors.Is(err, ErrValidation) {
				t.Fatal("una hora inválida tiene que ser 400, no otra cosa")
			}
		})
	}
}
