package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// El importe de un gasto editado (spec 053, FR-009). Cada caso es una forma en que una regla más
// simple reporta un gasto que no ocurrió.
func TestExpenseAmountAfterEdit(t *testing.T) {
	cases := []struct {
		name                                   string
		before, removed, adjusted, extra, live string
		want                                   string
	}{
		// El caso del 1-nov: lo agregado explica lo que ya estaba pagado; sumarlo duplicaría la compra.
		{"lo agregado ocupa lo sin desglosar", "67.50", "0", "0", "0", "67.50", "67.50"},
		{"lo que excede sube el importe", "67.50", "0", "0", "0", "80.00", "80.00"},
		// Sustitución: el proveedor cambió un producto por otro del mismo precio.
		{"borrar y agregar lo mismo no cambia", "100", "100", "0", "0", "100", "100"},
		{"borrar baja por su importe", "100", "40", "0", "0", "60", "60"},
		{"borrar respeta lo sin desglosar", "100", "60", "0", "0", "0", "40"},
		{"ajustar a lo recibido baja", "300", "0", "45", "0", "255", "255"},
		// Lo de más se pagó aparte (regla de la 038): sube aunque haya algo sin desglosar.
		{"lo de más sube aunque haya sin desglosar", "100", "0", "0", "20", "70", "120"},
		{"redondea a centavos", "10", "0", "0", "0", "10.005", "10.01"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ExpenseAmountAfterEdit(d(c.before), d(c.removed), d(c.adjusted), d(c.extra), d(c.live))
			if err != nil {
				t.Fatal(err)
			}
			if !got.Equal(d(c.want)) {
				t.Fatalf("importe = %s, quiere %s", got, c.want)
			}
		})
	}
	// Un gasto en $0 viola el check de la base (amount > 0) y respondería 500; quitar el gasto entero
	// es anularlo.
	if _, err := ExpenseAmountAfterEdit(d("50"), d("50"), d("0"), d("0"), d("0")); !errors.Is(err, ErrValidation) {
		t.Fatalf("un importe que queda en 0 debió rechazarse como validación, fue %v", err)
	}
	if _, err := ExpenseAmountAfterEdit(d("99999999"), d("0"), d("0"), d("0"), d("99999999.99")); !errors.Is(err, ErrValidation) {
		t.Fatalf("un importe fuera de la columna debió rechazarse, fue %v", err)
	}
}

// FR-011: el estado solo se mueve si cambió el importe, y un gasto pagado de antes de que existieran
// los pagos (sin ninguno) no se vuelve deuda por editarlo (revisión M5).
func TestPaymentStatusAfterEdit(t *testing.T) {
	cases := []struct {
		name         string
		current      string
		changed      bool
		paid, amount string
		hasPayments  bool
		want         string
	}{
		{"sin cambio de importe no se toca", ExpensePagada, false, "10", "100", true, ExpensePagada},
		{"sube sobre lo pagado: se debe", ExpensePagada, true, "67.50", "80", true, ExpensePendiente},
		{"baja a lo pagado: queda pagada", ExpensePendiente, true, "100", "100", true, ExpensePagada},
		{"pagado de más sigue pagada", ExpensePagada, true, "100", "60", true, ExpensePagada},
		{"pagada sin pagos no se degrada", ExpensePagada, true, "0", "120", false, ExpensePagada},
		{"pendiente sin pagos sigue pendiente", ExpensePendiente, true, "0", "120", false, ExpensePendiente},
		{"anulada no cambia", ExpenseCancelada, true, "0", "1", false, ExpenseCancelada},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := PaymentStatusAfterEdit(c.current, c.changed, d(c.paid), d(c.amount), c.hasPayments)
			if got != c.want {
				t.Fatalf("estado = %s, quiere %s", got, c.want)
			}
		})
	}
}

// FR-017: la preselección cuando llega menos. Una caja que no llegó sigue pendiente; un kilo de
// menos casi siempre se cobró de menos.
func TestShortfallDefault(t *testing.T) {
	cases := []struct {
		kind string
		pack bool
		want Shortfall
	}{
		{"pieza", false, ShortfallPending},
		{"masa", true, ShortfallPending}, // un bulto de 25 kg es un empaque: llega o no llega
		{"masa", false, ShortfallAdjust},
		{"volumen", false, ShortfallAdjust},
		{"longitud", false, ShortfallAdjust},
	}
	for _, c := range cases {
		if got := ShortfallDefault(c.kind, c.pack); got != c.want {
			t.Errorf("ShortfallDefault(%s, empaque=%v) = %s, quiere %s", c.kind, c.pack, got, c.want)
		}
	}
}

// Un valor de frontera desconocido se rechaza; nunca cae a un default (constitución V).
func TestEditDecisionsRejectUnknown(t *testing.T) {
	if _, err := ParseShortfall("pendiente"); !errors.Is(err, ErrValidation) {
		t.Error("un shortfall desconocido se aceptó")
	}
	if _, err := ParseCountDecision(""); !errors.Is(err, ErrValidation) {
		t.Error("una decisión de conteo vacía se aceptó")
	}
	if _, err := ParseMoneyDecision("refund"); !errors.Is(err, ErrValidation) {
		t.Error("una decisión de dinero desconocida se aceptó")
	}
	for _, s := range []string{"pending", "adjust"} {
		if _, err := ParseShortfall(s); err != nil {
			t.Errorf("ParseShortfall(%q): %v", s, err)
		}
	}
	for _, s := range []string{"adjust", "keep"} {
		if _, err := ParseCountDecision(s); err != nil {
			t.Errorf("ParseCountDecision(%q): %v", s, err)
		}
	}
	for _, s := range []string{"pay", "leave"} {
		if _, err := ParseMoneyDecision(s); err != nil {
			t.Errorf("ParseMoneyDecision(%q): %v", s, err)
		}
	}
}

// FR-018: ajustar a lo recibido conserva el precio por unidad.
func TestAdjustLineToReceived(t *testing.T) {
	q, a := AdjustLineToReceived(d("10"), d("8.5"), d("300"))
	if !q.Equal(d("8.5")) || !a.Equal(d("255")) {
		t.Fatalf("10 kg a $300 con 8.5 recibidos = %s por $%s, quiere 8.5 por $255", q, a)
	}
	q, a = AdjustLineToReceived(d("3"), d("1"), d("100"))
	if !q.Equal(d("1")) || !a.Equal(d("33.33")) {
		t.Fatalf("redondeo: %s por $%s", q, a)
	}
}

// FR-015: una entrega no es futura ni anterior a la compra.
func TestValidDeliveryDate(t *testing.T) {
	day := func(s string) time.Time { v, _ := time.Parse("2006-01-02", s); return v }
	today, exp := day("2026-11-01"), day("2026-10-03")
	for _, ok := range []string{"2026-10-03", "2026-10-20", "2026-11-01"} {
		if err := ValidDeliveryDate(day(ok), exp, today); err != nil {
			t.Errorf("%s debió valer: %v", ok, err)
		}
	}
	for _, bad := range []string{"2026-11-02", "2026-10-02"} {
		if err := ValidDeliveryDate(day(bad), exp, today); !errors.Is(err, ErrValidation) {
			t.Errorf("%s debió rechazarse", bad)
		}
	}
}

// R7 / revisión M2: borrar un renglón revierte cada entrega con su fecha. Una corrección negativa
// posterior (la de «Corregir lo que llegó») se descuenta de las entregas más recientes: emitir +20
// «hoy» desde un borrado sería una compra positiva que nadie hizo.
func TestPlanDeliveryReversal(t *testing.T) {
	day := func(s string) time.Time { v, _ := time.Parse("2006-01-02", s); return v }
	sum := func(ds []DeliveryDay) decimal.Decimal {
		s := decimal.Zero
		for _, x := range ds {
			s = s.Add(x.Qty)
		}
		return s
	}
	cases := []struct {
		name string
		in   []DeliveryDay
		want map[string]string
	}{
		{"una entrega", []DeliveryDay{{Day: day("2026-10-10"), Qty: d("50")}}, map[string]string{"2026-10-10": "-50"}},
		{"dos entregas", []DeliveryDay{{Day: day("2026-10-10"), Qty: d("3")}, {Day: day("2026-10-12"), Qty: d("2")}},
			map[string]string{"2026-10-10": "-3", "2026-10-12": "-2"}},
		{"corrección posterior se netea", []DeliveryDay{{Day: day("2026-10-10"), Qty: d("50")}, {Day: day("2026-11-01"), Qty: d("-20")}},
			map[string]string{"2026-10-10": "-30"}},
		{"corrección que alcanza a la más reciente", []DeliveryDay{
			{Day: day("2026-10-10"), Qty: d("3")}, {Day: day("2026-10-12"), Qty: d("2")}, {Day: day("2026-11-01"), Qty: d("-4")}},
			map[string]string{"2026-10-10": "-1"}},
		{"nada aplicado", []DeliveryDay{{Day: day("2026-10-10"), Qty: d("5")}, {Day: day("2026-10-11"), Qty: d("-5")}}, map[string]string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := PlanDeliveryReversal(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("contra-movimientos = %v, quiere %v", got, c.want)
			}
			for _, g := range got {
				if !g.Qty.IsNegative() {
					t.Fatalf("un borrado emitió %s el %s: una compra positiva que nadie hizo", g.Qty, g.Day.Format("2006-01-02"))
				}
				if w := c.want[g.Day.Format("2006-01-02")]; !g.Qty.Equal(d(w)) {
					t.Fatalf("el %s = %s, quiere %s", g.Day.Format("2006-01-02"), g.Qty, w)
				}
			}
			if applied := sum(c.in); applied.IsPositive() && !sum(got).Equal(applied.Neg()) {
				t.Fatalf("la reversión suma %s; lo aplicado era %s", sum(got), applied)
			}
		})
	}
}

// R5: un conteo es posterior a un movimiento si se capturó después del día de la entrega o después
// de registrarla. Recibir HOY nunca es anterior a un conteo ya cerrado.
func TestCountPosteriorCutoff(t *testing.T) {
	mx, _ := time.LoadLocation("America/Mexico_City")
	day, _ := time.ParseInLocation("2006-01-02", "2026-10-10", mx)
	registered := time.Date(2026, 11, 1, 12, 0, 0, 0, mx)
	if got := CountPosteriorCutoff(registered, day, mx); !got.Equal(time.Date(2026, 10, 11, 0, 0, 0, 0, mx)) {
		t.Fatalf("una entrega del 10-oct registrada el 1-nov corta al fin del 10-oct, fue %v", got)
	}
	sameDay := time.Date(2026, 10, 10, 9, 0, 0, 0, mx)
	if got := CountPosteriorCutoff(sameDay, day, mx); !got.Equal(sameDay) {
		t.Fatalf("registrada el mismo día corta en su registro (un conteo de la tarde ya la vio), fue %v", got)
	}
}

// R2: la huella cambia con cualquier cifra o decisión, y no con el orden de un mapa.
func TestPreviewFingerprint(t *testing.T) {
	a := map[string]any{"x": "1", "y": []string{"a"}}
	b := map[string]any{"y": []string{"a"}, "x": "1"}
	if PreviewFingerprint(a) != PreviewFingerprint(b) {
		t.Fatal("la huella depende del orden del mapa")
	}
	if PreviewFingerprint(a) == PreviewFingerprint(map[string]any{"x": "2", "y": []string{"a"}}) {
		t.Fatal("la huella no cambió con la cifra")
	}
}
