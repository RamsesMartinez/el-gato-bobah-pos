package domain

import (
	"fmt"
	"net/mail"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Reglas puras de caja y tarjeta de la spec 032 (puntos 3 a 10 de las decisiones del dueño).

// CashReverso es el movimiento que corrige una salida (punto 4). Suma al cajón lo que la salida
// restó; su signo sale de lo que revierte.
const CashReverso = "reverso"

var (
	ErrConceptRequired       = fmt.Errorf("%w: elige o escribe el concepto de la salida", ErrValidation)
	ErrAlreadyReversed       = fmt.Errorf("%w: esa salida ya se corrigió", ErrConflict)
	ErrNotReversible         = fmt.Errorf("%w: solo una salida capturada a mano se corrige aquí", ErrValidation)
	ErrOpeningReasonRequired = fmt.Errorf("%w: lo contado no coincide con el cierre anterior: elige el motivo", ErrValidation)
	ErrRefundFolioRequired   = fmt.Errorf("%w: escribe el folio que imprimió la terminal", ErrValidation)
	ErrCardTerminalRequired  = fmt.Errorf("%w: elige la terminal con la que se cobró", ErrValidation)
	ErrTerminalCountRequired = fmt.Errorf("%w: escribe el total del corte de cada terminal", ErrValidation)
)

// NormalizeConceptName recorta y colapsa espacios. Mayúsculas se comparan en la base.
func NormalizeConceptName(s string) (string, error) {
	n := strings.Join(strings.Fields(s), " ")
	if n == "" {
		return "", ErrConceptRequired
	}
	if len([]rune(n)) > 60 {
		return "", fmt.Errorf("%w: el concepto es demasiado largo", ErrValidation)
	}
	return n, nil
}

// ReversibleMovement es lo que hace falta saber de un movimiento para decidir si se corrige.
type ReversibleMovement struct {
	Kind            string
	IsExpense       bool
	IsTransfer      bool
	IsRefund        bool
	AlreadyReversed bool
}

// CanReverse: solo una salida capturada a mano, una sola vez. Gastos, traspasos, devoluciones y
// propinas tienen su propio camino y su propio rastro.
func CanReverse(m ReversibleMovement) error {
	if m.Kind != CashSalida || m.IsExpense || m.IsTransfer || m.IsRefund {
		return ErrNotReversible
	}
	if m.AlreadyReversed {
		return ErrAlreadyReversed
	}
	return nil
}

// ExpenseDay: el día del turno abierto manda (una salida pasada la medianoche es del turno de ayer);
// sin turno, el día que elija quien captura, y si no eligió, hoy.
func ExpenseDay(openShiftDay *time.Time, today time.Time, chosen *time.Time) time.Time {
	if openShiftDay != nil {
		return *openShiftDay
	}
	if chosen != nil {
		return *chosen
	}
	return today
}

// Motivos de una apertura que no coincide con el cierre anterior (punto 6).
var openingReasons = map[string]bool{
	"last_count_wrong": true, "float_changed": true, "unrecorded_withdrawal": true, "other": true,
}

// OpeningNeedsReason: hay cierre anterior y lo contado difiere de él al centavo.
func OpeningNeedsReason(prevClose *decimal.Decimal, counted decimal.Decimal) bool {
	return prevClose != nil && !Round2(*prevClose).Equal(Round2(counted))
}

// ValidOpeningReason: motivo de la lista; «otro» exige texto.
func ValidOpeningReason(reason, note string) error {
	if !openingReasons[reason] {
		return ErrOpeningReasonRequired
	}
	n := strings.TrimSpace(note)
	if reason == "other" && n == "" {
		return fmt.Errorf("%w: explica el motivo", ErrValidation)
	}
	if len([]rune(n)) > 200 {
		return fmt.Errorf("%w: el motivo es demasiado largo", ErrValidation)
	}
	return nil
}

// CardCountMode es cómo se arquea la tarjeta en una sucursal (punto 9).
type CardCountMode string

const (
	CardCountAuto        CardCountMode = "auto"
	CardCountPerTerminal CardCountMode = "per_terminal"
)

// ParseCardCountMode rechaza cualquier valor desconocido.
func ParseCardCountMode(s string) (CardCountMode, error) {
	switch CardCountMode(s) {
	case CardCountAuto, CardCountPerTerminal:
		return CardCountMode(s), nil
	}
	return "", fmt.Errorf("%w: modo de arqueo de tarjeta desconocido", ErrValidation)
}

// TerminalsToDeclare: con arqueo por terminal, las terminales que cobraron algo en el turno,
// archivadas o no; con automático, ninguna.
func TerminalsToDeclare(mode CardCountMode, collected map[int64]decimal.Decimal) []int64 {
	if mode != CardCountPerTerminal {
		return []int64{}
	}
	out := []int64{}
	for id, v := range collected {
		if v.IsPositive() {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// NormalizeRefundFolio: el folio de la devolución que imprime la terminal, sin espacios extremos.
func NormalizeRefundFolio(s string) (string, error) {
	f := strings.TrimSpace(s)
	if f == "" {
		return "", ErrRefundFolioRequired
	}
	if len([]rune(f)) > 60 {
		return "", fmt.Errorf("%w: el folio es demasiado largo", ErrValidation)
	}
	return f, nil
}

// MaxSummaryEmails es el tope del check de la base.
const MaxSummaryEmails = 10

// NormalizeSummaryEmails valida y normaliza los correos del resumen diario (decisión del
// 2026-10-09). Lista vacía = sin resumen.
func NormalizeSummaryEmails(in []string) ([]string, error) {
	if len(in) > MaxSummaryEmails {
		return nil, fmt.Errorf("%w: a lo más %d correos", ErrValidation, MaxSummaryEmails)
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		e := strings.ToLower(strings.TrimSpace(raw))
		a, err := mail.ParseAddress(e)
		if e == "" || err != nil || a.Address != e || !strings.Contains(e[strings.LastIndex(e, "@"):], ".") {
			return nil, fmt.Errorf("%w: «%s» no es un correo válido", ErrValidation, strings.TrimSpace(raw))
		}
		if seen[e] {
			return nil, fmt.Errorf("%w: «%s» está repetido", ErrValidation, e)
		}
		seen[e] = true
		out = append(out, e)
	}
	return out, nil
}
