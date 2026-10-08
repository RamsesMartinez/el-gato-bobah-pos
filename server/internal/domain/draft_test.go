package domain

import (
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/shopspring/decimal"
)

// Lo que entra a una cuenta en captura se valida en la frontera, igual que un renglón de pedido:
// la cuenta se guarda desde el primer toque y un valor absurdo guardado ahí revienta después, al
// mandarla a cocina, con la mesa esperando.
func TestValidateDraftLine(t *testing.T) {
	d := decimal.RequireFromString
	nota := func(n int) string { return strings.Repeat("a", n) }
	casos := []struct {
		nombre string
		in     DraftLineInput
		ok     bool
	}{
		{"uno sencillo", DraftLineInput{ProductID: 1, Qty: d("1")}, true},
		{"cantidad cero: un renglón que no pide nada", DraftLineInput{ProductID: 1, Qty: d("0")}, false},
		{"cantidad negativa", DraftLineInput{ProductID: 1, Qty: d("-1")}, false},
		{"un sub-centavo que redondea a cero", DraftLineInput{ProductID: 1, Qty: d("0.001")}, false},
		{"más que MaxOrderQty", DraftLineInput{ProductID: 1, Qty: MaxOrderQty.Add(d("1"))}, false},
		{"exponente absurdo (CPU en el redondeo)", DraftLineInput{ProductID: 1, Qty: d("1e100000000")}, false},
		{"sin producto", DraftLineInput{ProductID: 0, Qty: d("1")}, false},
		{"modificador sin opción", DraftLineInput{ProductID: 1, Qty: d("1"),
			Modifiers: []DraftModifier{{OptionID: 0, Qty: 1}}}, false},
		{"modificador con cantidad cero", DraftLineInput{ProductID: 1, Qty: d("1"),
			Modifiers: []DraftModifier{{OptionID: 3, Qty: 0}}}, false},
		{"modificador con una mitad que no existe", DraftLineInput{ProductID: 1, Qty: d("1"),
			Modifiers: []DraftModifier{{OptionID: 3, Qty: 1, Portion: "C"}}}, false},
		{"modificador que haría wrap de int16", DraftLineInput{ProductID: 1, Qty: d("1"),
			Modifiers: []DraftModifier{{OptionID: 3, Qty: 40000}}}, false},
		{"mitad A", DraftLineInput{ProductID: 1, Qty: d("1"),
			Modifiers: []DraftModifier{{OptionID: 3, Qty: 1, Portion: "A"}}}, true},
		{"nota de 200: el tope exacto pasa", DraftLineInput{ProductID: 1, Qty: d("1"), Notes: nota(200)}, true},
		{"nota de 201", DraftLineInput{ProductID: 1, Qty: d("1"), Notes: nota(201)}, false},
		// Los caracteres cuentan como letras, no como bytes: «ñ» ocupa dos y una nota de 200 eñes
		// es una nota de 200.
		{"200 eñes", DraftLineInput{ProductID: 1, Qty: d("1"), Notes: strings.Repeat("ñ", 200)}, true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := ValidateDraftLine(c.in)
			if (err == nil) != c.ok {
				t.Fatalf("ValidateDraftLine = %v, quería ok=%v", err, c.ok)
			}
		})
	}
}

// ValidateDraftLine devuelve la cantidad ya redondeada: lo que se guarda es lo que se validó.
func TestValidateDraftLineRoundsTheQuantity(t *testing.T) {
	got, err := ValidateDraftLine(DraftLineInput{ProductID: 1, Qty: decimal.RequireFromString("1.005")})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Qty.Equal(decimal.RequireFromString("1.01")) {
		t.Fatalf("qty = %s, quería 1.01", got.Qty)
	}
}

func TestMergeTarget(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	lines := []DraftLine{
		{ID: a, ProductID: 1},
		{ID: b, ProductID: 2, Notes: "sin hielo"},
		{ID: c, ProductID: 3, Modifiers: []DraftModifier{{OptionID: 7, Qty: 1}}},
	}
	casos := []struct {
		nombre string
		add    DraftLineInput
		want   uuid.UUID
		merge  bool
	}{
		{"igual y pelado: se suma al renglón", DraftLineInput{ProductID: 1}, a, true},
		{"con nota: renglón propio (cocina lo prepara distinto)", DraftLineInput{ProductID: 1, Notes: "tibio"}, uuid.UUID{}, false},
		{"con modificador: renglón propio", DraftLineInput{ProductID: 1, Modifiers: []DraftModifier{{OptionID: 7, Qty: 1}}}, uuid.UUID{}, false},
		{"producto distinto", DraftLineInput{ProductID: 9}, uuid.UUID{}, false},
		// El renglón igual pero CON nota no recibe al pelado: sumarle uno le pondría la nota a un
		// café que nadie pidió así.
		{"el igual con nota no recibe la fusión", DraftLineInput{ProductID: 2}, uuid.UUID{}, false},
		{"el igual con modificador no recibe la fusión", DraftLineInput{ProductID: 3}, uuid.UUID{}, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, ok := MergeTarget(lines, c.add)
			if ok != c.merge || (ok && got != c.want) {
				t.Fatalf("MergeTarget = (%v, %v), quería (%v, %v)", got, ok, c.want, c.merge)
			}
		})
	}
}

func TestDraftExpired(t *testing.T) {
	now := time.Date(2026, 10, 8, 20, 0, 0, 0, time.UTC)
	casos := []struct {
		nombre string
		hace   time.Duration
		vence  bool
	}{
		{"11h59m59s: sigue viva", 12*time.Hour - time.Second, false},
		{"12 horas exactas: vence", 12 * time.Hour, true},
		{"13 horas", 13 * time.Hour, true},
		// Un reloj de la tableta adelantado no debe matar una cuenta recién tocada.
		{"en el futuro", -time.Minute, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := DraftExpired(now.Add(-c.hace), now); got != c.vence {
				t.Fatalf("DraftExpired = %v, quería %v", got, c.vence)
			}
		})
	}
	if DraftIdleLimit != 12*time.Hour {
		t.Fatalf("DraftIdleLimit = %v; la regla del dueño (D-8) es 12 horas", DraftIdleLimit)
	}
}

func TestMaxDraftLines(t *testing.T) {
	if MaxDraftLines != 200 {
		t.Fatalf("MaxDraftLines = %d; el contrato dice 200", MaxDraftLines)
	}
}
