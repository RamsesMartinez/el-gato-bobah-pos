package domain

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

var (
	// ErrExpenseOutsideCount: un pago en efectivo de una caja que se cuenta no entraría a ningún
	// corte (la caja no tiene turno, o el pago es de otro día). Guardarlo así es válido —es como se
	// capturan gastos pasados— pero solo con la confirmación explícita de quien captura: en silencio,
	// el efectivo salió de una caja que se arquea y ningún arqueo lo ve.
	ErrExpenseOutsideCount = fmt.Errorf("%w: este gasto no entrará a ningún corte", ErrConflict)
	// ErrRegisterNotCounted: se intentó abrir turno en una caja que no lleva arqueo.
	ErrRegisterNotCounted = fmt.Errorf("%w: esta caja no lleva arqueo", ErrConflict)
	// ErrRegisterHasOpenShift: se intentó quitar el arqueo a una caja con turno abierto, que
	// dejaría un corte a medias.
	ErrRegisterHasOpenShift = fmt.Errorf("%w: cierra el turno de esta caja primero", ErrConflict)
	// ErrPrimaryRegisterCounts: la caja principal vende en efectivo; sin arqueo sus ventas no
	// tendrían corte.
	ErrPrimaryRegisterCounts = fmt.Errorf("%w: la caja principal siempre lleva arqueo", ErrConflict)
	// ErrExpenseBeforeLastClose: un pago incluido en el arqueo cuya hora cae en o antes del último
	// cierre de su caja (o antes de la apertura del turno abierto): ningún corte puede tomarlo. Solo
	// se puede guardar sin incluir (spec 048).
	ErrExpenseBeforeLastClose = fmt.Errorf("%w: este gasto es de antes del último corte de la caja; solo se puede registrar sin incluir en el arqueo", ErrConflict)
	// ErrPaymentInClosedCount: se intentó cambiar la casilla de un pago cuyo corte ya se cerró.
	ErrPaymentInClosedCount = fmt.Errorf("%w: el corte de este pago ya se cerró", ErrConflict)
	// ErrInvalidOpeningTime: la hora de apertura movida es futura o no es posterior al último cierre.
	ErrInvalidOpeningTime = fmt.Errorf("%w: hora de apertura inválida", ErrValidation)
	// ErrPaymentNotLinked: un pago en un corte cuya salida de caja es de antes de la 0100 y no dice
	// de qué pago es: no hay forma segura de saber cuál borrar.
	ErrPaymentNotLinked = fmt.Errorf("%w: Este pago es de antes del cambio: no se puede quitar del arqueo", ErrConflict)
	// ErrDrawerExtraTooHigh: lo que salió de la caja pasa del importe más lo excluido del gasto que
	// otros pagos no han usado (spec 050), o un renglón excluido en uso se quiso volver a incluir.
	ErrDrawerExtraTooHigh = fmt.Errorf("%w: lo que salió de la caja pasa de lo que no es del negocio en este gasto", ErrValidation)
)

// ValidDrawerExtra decide si un pago de `amount` puede declarar que del cajón salieron además
// `extra` pesos (spec 050). El tope es por GASTO: la suma de sus renglones excluidos menos lo que
// otros pagos ya usaron; así dos pagos no sacan dos veces la misma parte que no es del negocio.
// Con total del ticket (`ticketRoom` = total del documento − importe del gasto), lo usado tampoco
// pasa de lo que el ticket deja: no salió del cajón más de lo que decía el papel. Sin él queda solo
// el de excluidos. La salida completa (importe + extra) tampoco puede pasar de MaxMoney.
func ValidDrawerExtra(amount, extra, excludedSum, alreadyUsed decimal.Decimal, ticketRoom *decimal.Decimal) error {
	if !ValidMoney(extra, true) || !ValidMoney(amount.Add(extra), true) {
		return ErrValidation
	}
	limit := excludedSum
	if ticketRoom != nil && ticketRoom.LessThan(limit) {
		limit = *ticketRoom
	}
	if extra.GreaterThan(limit.Sub(alreadyUsed)) {
		return ErrDrawerExtraTooHigh
	}
	return nil
}

// PaymentRegister es lo que importa de una caja para decidir a dónde va un pago de gasto.
type PaymentRegister struct {
	CountsCash bool
	// OpenShiftDay es el día de negocio del turno abierto; nil si la caja está cerrada.
	OpenShiftDay *time.Time
	// OpenShiftOpenedAt es la hora de apertura del turno abierto (spec 048); nil si está cerrada.
	OpenShiftOpenedAt *time.Time
	// LastClose es la hora del último cierre de la caja; nil si nunca cerró.
	LastClose *time.Time
}

// PaymentChoice es lo que decidió quien captura. IncludeInCount nil es una tableta con la versión
// de la 047, que no conoce la casilla: se decide con las reglas de entonces.
type PaymentChoice struct {
	IncludeInCount *bool
	// OutsideCount confirma el aviso: con la casilla, «Registrar» (pendiente); sin ella, «fuera».
	OutsideCount bool
}

// PaymentTarget es a dónde va un pago de gasto respecto al arqueo.
type PaymentTarget int

const (
	// TargetOutside: fuera de todo corte.
	TargetOutside PaymentTarget = iota
	// TargetShift: al turno abierto de su caja.
	TargetShift
	// TargetPending: incluido, sin corte todavía; lo toma el siguiente corte cuya ventana cubra su
	// hora (spec 048).
	TargetPending
)

// PaymentTime es la hora del pago: la del reloj si se paga el día de negocio de hoy; si no, la
// medianoche de su día en la zona del negocio. No existe la hora real de un pago capturado días
// después, y la medianoche lo deja casi siempre antes del último cierre, fuera de todo corte.
func PaymentTime(paidOn, now time.Time, loc *time.Location) time.Time {
	if BusinessDate(now, loc).Equal(paidOn) {
		return now
	}
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := paidOn.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// ExpensePaymentTarget dice a dónde va un pago de gasto.
//
// Tarjeta y transferencia no mueven el cajón: se atribuyen al turno abierto si lo hay, como siempre.
// El efectivo de una caja sin arqueo queda fuera sin preguntar. El de una caja con arqueo:
//   - desmarcado → fuera, sin preguntar;
//   - con turno abierto y hora en o después de su apertura → al corte;
//   - con hora en o antes del último cierre, o antes de la apertura del turno abierto → no hay corte
//     que lo pueda tomar: ErrExpenseBeforeLastClose;
//   - si no, pendiente, pero solo tras confirmar el aviso.
//
// Sin casilla (IncludeInCount nil) rigen las reglas de la 047 tal cual: solo el día del turno entra,
// y confirmar es «fuera». Así una tableta vieja durante la subida nunca deja un pendiente.
func ExpensePaymentTarget(isCash bool, reg *PaymentRegister, paidOn, paidAt time.Time, choice PaymentChoice) (PaymentTarget, error) {
	if reg == nil {
		if isCash {
			return TargetOutside, ErrPaymentNeedsRegister
		}
		return TargetOutside, nil
	}
	if !isCash {
		if reg.OpenShiftDay != nil {
			return TargetShift, nil
		}
		return TargetOutside, nil
	}
	if !reg.CountsCash {
		return TargetOutside, nil
	}
	if choice.IncludeInCount == nil {
		if reg.OpenShiftDay != nil && reg.OpenShiftDay.Equal(paidOn) {
			return TargetShift, nil
		}
		if choice.OutsideCount {
			return TargetOutside, nil
		}
		return TargetOutside, ErrExpenseOutsideCount
	}
	if !*choice.IncludeInCount {
		return TargetOutside, nil
	}
	// Con la casilla decide SOLO la hora: dos cortes del mismo día comparten el día, y el atajo por
	// día metería al segundo un pago que salió del cajón del primero.
	if reg.OpenShiftOpenedAt != nil && !paidAt.Before(*reg.OpenShiftOpenedAt) {
		return TargetShift, nil
	}
	if reg.LastClose != nil && !paidAt.After(*reg.LastClose) {
		return TargetOutside, ErrExpenseBeforeLastClose
	}
	if reg.OpenShiftOpenedAt != nil && paidAt.Before(*reg.OpenShiftOpenedAt) {
		return TargetOutside, ErrExpenseBeforeLastClose
	}
	if !choice.OutsideCount {
		return TargetOutside, ErrExpenseOutsideCount
	}
	return TargetPending, nil
}

// MaxOpeningBackdate es el piso de la hora de apertura de una caja que nunca cerró: sin cierre
// anterior no hay otro límite, y un corte que empieza días atrás se llevaría gastos de otro arqueo.
const MaxOpeningBackdate = 24 * time.Hour

// ValidBackdatedOpening valida la hora de apertura movida hacia atrás: nunca futura y siempre
// después del último cierre de la caja, para que dos cortes no se repartan el mismo efectivo.
func ValidBackdatedOpening(openedAt, now time.Time, lastClose *time.Time) error {
	if lastClose == nil && openedAt.Before(now.Add(-MaxOpeningBackdate)) {
		return fmt.Errorf("%w: la hora de apertura no puede ser de hace más de 24 horas", ErrInvalidOpeningTime)
	}
	if openedAt.After(now) {
		return fmt.Errorf("%w: la hora de apertura no puede ser futura", ErrInvalidOpeningTime)
	}
	if lastClose != nil && !openedAt.After(*lastClose) {
		return fmt.Errorf("%w: la hora de apertura tiene que ser después del último cierre de la caja", ErrInvalidOpeningTime)
	}
	return nil
}
