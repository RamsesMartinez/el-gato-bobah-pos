package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// «Hielo» y « hielo  » son el mismo concepto (EB-20); un nombre vacío no es concepto (EB-18).
func TestNormalizeConceptName(t *testing.T) {
	for in, want := range map[string]string{"  Hielo ": "Hielo", "Pago   de  gas": "Pago de gas"} {
		got, err := NormalizeConceptName(in)
		if err != nil || got != want {
			t.Errorf("%q → %q, %v; quería %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "   ", string(make([]byte, 61))} {
		if _, err := NormalizeConceptName(bad); !errors.Is(err, ErrConceptRequired) && !errors.Is(err, ErrValidation) {
			t.Errorf("%q debía rechazarse", bad)
		}
	}
}

// Solo una salida capturada a mano se corrige, una sola vez (EB-15, EB-16). Un gasto, un traspaso,
// una devolución, una propina o un reverso tienen su propio camino.
func TestCanReverse(t *testing.T) {
	cases := []struct {
		name string
		m    ReversibleMovement
		ok   bool
	}{
		{"salida a mano", ReversibleMovement{Kind: CashSalida}, true},
		{"ya corregida", ReversibleMovement{Kind: CashSalida, AlreadyReversed: true}, false},
		{"reverso", ReversibleMovement{Kind: CashReverso}, false},
		{"entrada", ReversibleMovement{Kind: CashEntrada}, false},
		{"gasto", ReversibleMovement{Kind: CashSalida, IsExpense: true}, false},
		{"traspaso", ReversibleMovement{Kind: CashSalida, IsTransfer: true}, false},
		{"devolución", ReversibleMovement{Kind: CashSalida, IsRefund: true}, false},
		{"propina", ReversibleMovement{Kind: CashPropina}, false},
	}
	for _, c := range cases {
		if err := CanReverse(c.m); (err == nil) != c.ok {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
	if err := CanReverse(ReversibleMovement{Kind: CashSalida, AlreadyReversed: true}); !errors.Is(err, ErrAlreadyReversed) {
		t.Errorf("doble corrección: err = %v, quería ErrAlreadyReversed", err)
	}
}

// El día del gasto es el del turno abierto aunque ya sea otro día en el reloj (EB-21); sin turno,
// hoy o lo que elija quien captura. La fecha del documento nunca lo mueve (EB-22).
func TestExpenseDay(t *testing.T) {
	ayer := time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)
	hoy := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	elegido := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if got := ExpenseDay(&ayer, hoy, &elegido); !got.Equal(ayer) {
		t.Errorf("con turno abierto manda el turno: %v", got)
	}
	if got := ExpenseDay(nil, hoy, &elegido); !got.Equal(elegido) {
		t.Errorf("sin turno manda lo elegido: %v", got)
	}
	if got := ExpenseDay(nil, hoy, nil); !got.Equal(hoy) {
		t.Errorf("sin turno ni elección, hoy: %v", got)
	}
}

// La apertura pide motivo solo si lo contado difiere del cierre anterior de la misma caja; el
// primer turno no tiene contra qué compararse (EB-26).
func TestOpeningReason(t *testing.T) {
	prev := d("1500")
	if OpeningNeedsReason(nil, d("1500.00")) {
		t.Error("primer turno no pide motivo")
	}
	if OpeningNeedsReason(&prev, d("1500.00")) {
		t.Error("igual no pide motivo")
	}
	if !OpeningNeedsReason(&prev, d("1499.50")) {
		t.Error("distinto pide motivo")
	}
	for _, c := range []struct {
		reason, note string
		ok           bool
	}{
		{"cambio_de_fondo", "", true},
		{"otro", "", false},
		{"otro", "se llevó cambio el gerente", true},
		{"", "", false},
		{"inventado", "x", false},
	} {
		if err := ValidOpeningReason(c.reason, c.note); (err == nil) != c.ok {
			t.Errorf("%q/%q: err = %v", c.reason, c.note, err)
		}
	}
}

// Con arqueo por terminal se declara cada terminal que cobró, aunque se haya archivado (EB-32); con
// arqueo automático no se pide nada y la diferencia es cero.
func TestTerminalCount(t *testing.T) {
	cobrado := map[int64]decimal.Decimal{1: d("500"), 2: d("0")}
	if got := TerminalsToDeclare(CardCountAuto, cobrado); len(got) != 0 {
		t.Errorf("automático no pide terminales: %v", got)
	}
	got := TerminalsToDeclare(CardCountPerTerminal, cobrado)
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("por terminal pide solo las que cobraron: %v", got)
	}
	if _, err := ParseCardCountMode("x"); !errors.Is(err, ErrValidation) {
		t.Error("modo desconocido debe ser 400")
	}
}

// El folio que imprime la terminal es obligatorio y no se acepta en blanco (EB-34, EB-36).
func TestRefundFolio(t *testing.T) {
	if f, err := NormalizeRefundFolio("  A123 "); err != nil || f != "A123" {
		t.Errorf("folio = %q, %v", f, err)
	}
	if _, err := NormalizeRefundFolio("   "); !errors.Is(err, ErrRefundFolioRequired) {
		t.Errorf("folio en blanco: %v", err)
	}
}

// Correos del resumen: válidos, sin duplicados, a lo más 10 (EB-47).
func TestValidSummaryEmails(t *testing.T) {
	ok, err := NormalizeSummaryEmails([]string{" Dueno@Ejemplo.com", "conta@ejemplo.mx"})
	if err != nil || len(ok) != 2 || ok[0] != "dueno@ejemplo.com" {
		t.Fatalf("%v %v", ok, err)
	}
	for name, in := range map[string][]string{
		"mal formado": {"no-es-correo"},
		"duplicado":   {"a@b.mx", "A@b.mx"},
		"vacío":       {""},
		"demasiados":  {"a1@b.mx", "a2@b.mx", "a3@b.mx", "a4@b.mx", "a5@b.mx", "a6@b.mx", "a7@b.mx", "a8@b.mx", "a9@b.mx", "a10@b.mx", "a11@b.mx"},
	} {
		if _, err := NormalizeSummaryEmails(in); !errors.Is(err, ErrValidation) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if got, err := NormalizeSummaryEmails(nil); err != nil || got == nil || len(got) != 0 {
		t.Errorf("lista vacía es válida (sin correo): %v %v", got, err)
	}
}
