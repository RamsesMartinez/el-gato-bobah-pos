package domain

import (
	"errors"
	"testing"
)

// El reparto parejo entrega pesos enteros y deja el sobrante pendiente (decisión del 2026-10-09):
// nunca se reparten centavos ni se pierde el resto.
func TestSplitEvenGivesWholePesosAndLeavesTheRestPending(t *testing.T) {
	cases := []struct {
		name           string
		pending        string
		n              int
		each, leftover string
		err            error
	}{
		{"exacto", "186.00", 3, "62", "0", nil},
		{"sobra un peso", "187.00", 3, "62", "1", nil},
		{"centavos se quedan", "186.50", 3, "62", "0.5", nil},
		{"una persona con centavos", "96.40", 1, "96", "0.4", nil},
		{"no alcanza un peso por persona", "2.50", 3, "", "", ErrSplitBelowOnePeso},
		{"nadie", "100", 0, "", "", ErrNoRecipients},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			each, left, err := SplitEven(d(c.pending), c.n)
			if !errors.Is(err, c.err) {
				t.Fatalf("err = %v, quería %v", err, c.err)
			}
			if c.err != nil {
				return
			}
			if !each.Equal(d(c.each)) || !left.Equal(d(c.leftover)) {
				t.Fatalf("parejo de %s entre %d = %s c/u y sobran %s; quería %s y %s", c.pending, c.n, each, left, c.each, c.leftover)
			}
		})
	}
}

// Una entrega ajustada con centavos se rechaza en el servidor: la pantalla no es la barrera.
func TestValidatePayoutRejectsCentsDuplicatesAndExcess(t *testing.T) {
	cases := []struct {
		name    string
		pending string
		lines   []PayoutLine
		err     error
	}{
		{"ok", "100", []PayoutLine{{1, d("60")}, {2, d("40")}}, nil},
		{"centavos", "100", []PayoutLine{{1, d("40.50")}}, ErrWholePesosOnly},
		{"repetida", "100", []PayoutLine{{1, d("10")}, {1, d("10")}}, ErrDuplicateRecipient},
		{"vacía", "100", nil, ErrNoRecipients},
		{"cero", "100", []PayoutLine{{1, d("0")}}, ErrValidation},
		{"negativa", "100", []PayoutLine{{1, d("-5")}}, ErrValidation},
		{"absurda", "100", []PayoutLine{{1, d("99999999999")}}, ErrValidation},
		{"más que el pendiente", "100.50", []PayoutLine{{1, d("101")}}, ErrTipExceedsPending},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := ValidatePayout(d(c.pending), c.lines); !errors.Is(err, c.err) {
				t.Fatalf("err = %v, quería %v", err, c.err)
			}
		})
	}
}

// El origen de cada peso entregado se reparte: primero lo heredado, luego efectivo, luego lo demás
// en orden de cobro. Una fuente nunca da más de lo que le queda.
func TestAllocateTipSourcesInheritedThenCashThenRest(t *testing.T) {
	sources := []TipSource{
		{PaymentID: 10, Cash: false, Available: d("70")},
		{PaymentID: 11, Cash: true, Available: d("30")},
		{PaymentID: 0, Inherited: true, Available: d("5")},
		{PaymentID: 12, Cash: true, Available: d("0")},
	}
	got, err := AllocateTipSources(sources, d("50"))
	if err != nil {
		t.Fatal(err)
	}
	want := []TipAllocation{{PaymentID: 0, Amount: d("5")}, {PaymentID: 11, Amount: d("30")}, {PaymentID: 10, Amount: d("15")}}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i].PaymentID != want[i].PaymentID || !got[i].Amount.Equal(want[i].Amount) {
			t.Fatalf("asignación %d = %+v, quería %+v", i, got[i], want[i])
		}
	}
	if _, err := AllocateTipSources(sources, d("106")); !errors.Is(err, ErrTipExceedsPending) {
		t.Fatalf("pasarse del pendiente debe fallar, err = %v", err)
	}
}

// Un sobrante de centavos se hereda solo: preguntar por él obligaría a decidir algo que no se puede
// entregar (EB-45).
func TestTipsDecisionRequiredOnlyFromOnePeso(t *testing.T) {
	for _, c := range []struct {
		p    string
		want bool
	}{{"0", false}, {"0.99", false}, {"1", true}, {"186.50", true}} {
		if got := TipsDecisionRequired(d(c.p)); got != c.want {
			t.Errorf("pendiente %s: %v, quería %v", c.p, got, c.want)
		}
	}
}

func TestParseTipsDecision(t *testing.T) {
	if _, err := ParseTipsDecision("otra"); !errors.Is(err, ErrValidation) {
		t.Fatal("decisión desconocida debe ser 400")
	}
	if v, err := ParseTipsDecision(""); err != nil || v != TipsDecisionNone {
		t.Fatal("ausente = sin decisión")
	}
}
