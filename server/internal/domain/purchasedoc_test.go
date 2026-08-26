package domain

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func TestParseAmount(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{"simple", "190.25", "190.25", true},
		{"con símbolo y espacio", "$      190.25", "190.25", true},
		{"separador de miles", "$ 3,847.40", "3847.40", true},
		{"negativo con guion", "-199.00", "-199", true},
		{"negativo entre paréntesis", "(199.00)", "-199", true},
		{"cantidad fraccionaria", "0.280", "0.280", true},
		{"vacío", "", "0", false},
		{"solo espacios", "   ", "0", false},
		{"ilegible", "N/D", "0", false},
		// El código fiscal que Sam's pega al importe ("190.25A") no debe contaminar el número.
		{"sufijo fiscal", "190.25A", "190.25", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseAmount(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, quiero %v", ok, tt.ok)
			}
			if want := decimal.RequireFromString(tt.want); !got.Equal(want) {
				t.Errorf("got = %s, quiero %s", got, want)
			}
		})
	}
}

func TestParseQtyRechazaNegativos(t *testing.T) {
	if _, ok := ParseQty("-3"); ok {
		t.Error("una cantidad negativa debe rechazarse; un descuento es un Charge, no una línea")
	}
}

// Los tres documentos reales de docs/tickets/ tienen estructuras distintas y los tres deben
// cuadrar con el MISMO Reconcile. Si un proveedor nuevo rompe esto, el test lo dice.
func TestReconcileDocumentosReales(t *testing.T) {
	t.Run("ticket de mostrador: las líneas explican el total", func(t *testing.T) {
		// Sam's: 17 renglones que suman exactamente el TOTAL impreso, sin cargos aparte.
		amounts := []string{
			"190.25A", "179.02T", "285.41T", "121.74C", "188.22C", "342.71T", "163.68C",
			"734.51T", "117.64T", "163.68T", "351.90T", "142.19T", "111.50T", "93.09T",
			"325.31T", "99.23A", "237.32T",
		}
		doc := PurchaseDoc{Total: "$ 3,847.40"}
		for _, a := range amounts {
			doc.Lines = append(doc.Lines, PurchaseLine{RawName: "x", Amount: a})
		}
		r := doc.Reconcile()
		if !r.Balanced() {
			t.Errorf("debería cuadrar; diff = %s (líneas %s)", r.Diff, r.LinesSum)
		}
		if want := decimal.RequireFromString("3847.40"); !r.LinesSum.Equal(want) {
			t.Errorf("suma de líneas = %s, quiero %s", r.LinesSum, want)
		}
	})

	t.Run("pedido: descuento y envío a nivel documento también cuadran", func(t *testing.T) {
		// Walmart: subtotal 1541.83 − descuento 199.00 + envío 44.10 = total 1386.93.
		// Un check que solo comparara líneas contra total daría falsa alarma aquí.
		doc := PurchaseDoc{
			Lines: []PurchaseLine{
				{RawName: "Zanahoria por kilo", Amount: "32.38", Status: LineAjustado},
				{RawName: "Pepino por kilo", Amount: "107.24", Status: LineAjustado},
				{RawName: "Pasta dental", Amount: "65.00", Status: LineComprado},
				{RawName: "Resto del pedido", Amount: "1337.21", Status: LineComprado},
			},
			Charges: []PurchaseCharge{
				{Label: "Descuento", Amount: "-199.00", AffectsTotal: true},
				{Label: "Costo de envío", Amount: "0.00", AffectsTotal: true},
				{Label: "Envío a domicilio", Amount: "44.10", AffectsTotal: true},
			},
			Total: "$1386.93",
		}
		r := doc.Reconcile()
		if !r.Balanced() {
			t.Errorf("debería cuadrar; diff = %s (líneas %s, cargos %s)", r.Diff, r.LinesSum, r.ChargesSum)
		}
	})

	t.Run("una línea no surtida no suma al total pagado", func(t *testing.T) {
		// Walmart cobró 1541.83 de subtotal con una línea "No disponible" de 4 × $41.00 = 164.00
		// impresa en el documento. Contarla descuadra el pedido por exactamente esos 164.
		lines := []PurchaseLine{
			{RawName: "Salsa Hunts", Amount: "164.00", Status: LineNoDisponible},
			{RawName: "Resto del pedido", Amount: "1541.83", Status: LineComprado},
		}
		doc := PurchaseDoc{Lines: lines, Total: "1541.83"}
		if r := doc.Reconcile(); !r.Balanced() {
			t.Errorf("la línea no surtida no debe sumar; diff = %s", r.Diff)
		}

		doc.Lines[0].Status = LineComprado
		r := doc.Reconcile()
		if r.Balanced() {
			t.Error("marcada como comprada sí debe descuadrar (si no, el check no sirve de nada)")
		}
		if want := decimal.RequireFromString("-164.00"); !r.Diff.Equal(want) {
			t.Errorf("diff = %s, quiero %s", r.Diff, want)
		}
	})

	t.Run("el impuesto desglosado no se suma dos veces", func(t *testing.T) {
		// Sam's imprime precios CON impuesto y luego lista TOTAL IVA 39.93 / TOTAL IEPS 35.08
		// como desglose. Si se sumaran, el ticket aparecería descuadrado por esos 75.01 aunque
		// las líneas den el total exacto.
		doc := PurchaseDoc{
			Lines: []PurchaseLine{{RawName: "mercancía", Amount: "3847.40"}},
			Charges: []PurchaseCharge{
				{Label: "TOTAL IVA", Amount: "39.93", AffectsTotal: false},
				{Label: "TOTAL IEPS", Amount: "35.08", AffectsTotal: false},
			},
			Total: "3847.40",
		}
		r := doc.Reconcile()
		if !r.Balanced() {
			t.Errorf("el desglose no debe romper el cuadre; diff = %s", r.Diff)
		}
		if want := decimal.RequireFromString("75.01"); !r.BreakdownSum.Equal(want) {
			t.Errorf("desglose = %s, quiero %s (debe reportarse, no sumarse)", r.BreakdownSum, want)
		}
	})

	t.Run("pago partido en un solo documento", func(t *testing.T) {
		// Soriana: TARJETA CREDITO 640.86 + EFECTIVO 0.01 = TOTAL 640.87.
		doc := PurchaseDoc{
			Total: "640.87",
			Payments: []PurchasePayment{
				{Method: "TARJETA CREDITO", Amount: "640.86"},
				{Method: "EFECTIVO", Amount: "0.01"},
			},
		}
		if r := doc.Reconcile(); !r.PaymentsMatchTotal() {
			t.Errorf("los pagos deben cubrir el total; pagos %s vs total %s", r.PaymentsSum, r.Total)
		}
	})
}

// El subtotal impreso delata una extracción que tomó la columna de precio equivocada, incluso
// cuando descuentos y envío hacen que el total sí "cuadre" por casualidad.
func TestLinesMatchSubtotal(t *testing.T) {
	doc := PurchaseDoc{
		// Walmart imprime unitario con IVA y sin IVA. Tomando el sin-IVA las líneas dan 1064.68
		// en vez del subtotal impreso 1541.83: exactamente el impuesto de todo el pedido.
		Lines:    []PurchaseLine{{RawName: "pedido con precios sin IVA", Amount: "1064.68"}},
		Subtotal: "1541.83",
		Total:    "1386.93",
	}
	r := doc.Reconcile()
	if r.LinesMatchSubtotal() {
		t.Error("las líneas no explican el subtotal impreso; debe detectarse")
	}
	if want := decimal.RequireFromString("477.15"); !r.SubtotalDiff.Equal(want) {
		t.Errorf("SubtotalDiff = %s, quiero %s", r.SubtotalDiff, want)
	}

	doc.Lines[0].Amount = "1541.83"
	if r := doc.Reconcile(); !r.LinesMatchSubtotal() {
		t.Errorf("con la columna correcta debe cuadrar; diff = %s", r.SubtotalDiff)
	}

	// Sin subtotal impreso no hay nada que contradecir (un ticket de mostrador).
	sinSubtotal := PurchaseDoc{Lines: []PurchaseLine{{Amount: "100.00"}}, Total: "100.00"}
	if !sinSubtotal.Reconcile().LinesMatchSubtotal() {
		t.Error("sin subtotal declarado no se puede reportar discrepancia")
	}
}

// Hay documentos que solo imprimen el precio "c/u" y nunca el importe del renglón. Sin la
// multiplicación, un pedido con renglones de varias piezas queda corto por exactamente las
// unidades extra (en el pedido real de Walmart: 173.14).
func TestEffectiveAmount(t *testing.T) {
	tests := []struct {
		name                   string
		amount, unitPrice, qty string
		want                   string
		ok                     bool
	}{
		{"importe impreso manda", "51.00", "17.00", "3", "51.00", true},
		{"solo unitario: multiplica", "", "17.00", "3", "51.00", true},
		{"sin cantidad es una unidad", "", "22.00", "", "22.00", true},
		{"cantidad fraccionaria por peso", "", "59.50", "0.280", "16.66", true},
		{"sin importe ni unitario", "", "", "2", "0", false},
		{"cantidad cero se trata como una unidad", "", "10.00", "0", "10.00", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := PurchaseLine{Amount: tt.amount, UnitPrice: tt.unitPrice, Qty: tt.qty}
			got, ok := l.EffectiveAmount()
			if ok != tt.ok {
				t.Fatalf("ok = %v, quiero %v", ok, tt.ok)
			}
			if want := decimal.RequireFromString(tt.want); !got.Equal(want) {
				t.Errorf("got = %s, quiero %s", got, want)
			}
		})
	}
}

func TestPickPriceColumn(t *testing.T) {
	// Walmart imprime por renglón el unitario con IVA (el que suma al subtotal) y el unitario
	// sin IVA. Si el extractor puso el sin-IVA como principal, el código debe voltearlos.
	t.Run("intercambia cuando el alterno es el que cuadra", func(t *testing.T) {
		doc := PurchaseDoc{
			Lines: []PurchaseLine{
				{RawName: "Pasta dental", Amount: "56.03", AmountAlt: "65.00", UnitPrice: "56.03", UnitPriceAlt: "65.00"},
				{RawName: "Shampoo", Amount: "84.48", AmountAlt: "120.00", UnitPrice: "84.48", UnitPriceAlt: "120.00"},
			},
			Subtotal: "185.00",
		}
		if !doc.PickPriceColumn() {
			t.Fatal("debía intercambiar: 65.00 + 120.00 = 185.00 es el subtotal impreso")
		}
		if doc.Lines[0].Amount != "65.00" || doc.Lines[0].UnitPrice != "65.00" {
			t.Errorf("importe y unitario deben quedar en la columna que cuadra, tengo %q/%q",
				doc.Lines[0].Amount, doc.Lines[0].UnitPrice)
		}
		if doc.Lines[0].AmountAlt != "56.03" {
			t.Error("el precio descartado debe conservarse en el campo alterno")
		}
		if !doc.Reconcile().LinesMatchSubtotal() {
			t.Error("después del intercambio las líneas deben explicar el subtotal")
		}
	})

	t.Run("no toca nada si el principal ya cuadra", func(t *testing.T) {
		doc := PurchaseDoc{
			Lines:    []PurchaseLine{{Amount: "65.00", AmountAlt: "56.03"}},
			Subtotal: "65.00",
		}
		if doc.PickPriceColumn() {
			t.Error("no debe intercambiar cuando el principal ya cuadra")
		}
	})

	t.Run("no toca nada si ninguna columna cuadra", func(t *testing.T) {
		// Cambiar precios a ciegas es peor que dejar la lectura del extractor: el operador ve
		// la discrepancia y corrige el renglón.
		doc := PurchaseDoc{
			Lines:    []PurchaseLine{{Amount: "10.00", AmountAlt: "20.00"}},
			Subtotal: "99.00",
		}
		if doc.PickPriceColumn() {
			t.Error("sin una columna que cuadre no debe modificar importes")
		}
		if doc.Lines[0].Amount != "10.00" {
			t.Error("los importes deben quedar intactos")
		}
	})

	t.Run("sin subtotal impreso no hay con qué decidir", func(t *testing.T) {
		doc := PurchaseDoc{Lines: []PurchaseLine{{Amount: "10.00", AmountAlt: "20.00"}}, Total: "10.00"}
		if doc.PickPriceColumn() {
			t.Error("sin subtotal no debe intercambiar")
		}
	})
}

// Hay tickets cuyo SUBTOTAL es pre-impuesto mientras los renglones ya lo incluyen: la
// diferencia es exactamente el impuesto desglosado y no es un error de extracción.
func TestLinesMatchSubtotalToleraSubtotalPreImpuesto(t *testing.T) {
	doc := PurchaseDoc{
		Lines:    []PurchaseLine{{RawName: "mercancía", Amount: "640.06"}},
		Charges:  []PurchaseCharge{{Label: "IMPUESTO", Amount: "6.50", AffectsTotal: false}},
		Subtotal: "633.56",
		Total:    "640.06",
	}
	r := doc.Reconcile()
	if !r.LinesMatchSubtotal() {
		t.Errorf("la diferencia (%s) es el impuesto desglosado (%s): no es discrepancia",
			r.SubtotalDiff, r.BreakdownSum)
	}
}

func TestDropAmbiguousCodes(t *testing.T) {
	// Soriana repite el código 009 en dos panes distintos: es departamento, no artículo.
	doc := PurchaseDoc{Lines: []PurchaseLine{
		{RawCode: "009", RawName: "PAN FRAN BOLILLO 1"},
		{RawCode: "009", RawName: "PAN NOVIAS 1 PZA"},
		{RawCode: "862", RawName: "BOLSA REUTILIZABLE"},
		{RawCode: "300", RawName: "JAMON PAVO REAL SAN"},
	}}
	doc.DropAmbiguousCodes()

	for i, l := range doc.Lines[:2] {
		if l.RawCode != "" {
			t.Errorf("línea %d: el código ambiguo debe quedar vacío, tengo %q", i, l.RawCode)
		}
	}
	if doc.Lines[2].RawCode != "862" || doc.Lines[3].RawCode != "300" {
		t.Error("los códigos que identifican un solo artículo deben conservarse")
	}
	if len(doc.Warnings) != 1 {
		t.Errorf("quiero 1 advertencia sobre el código ambiguo, tengo %d: %v", len(doc.Warnings), doc.Warnings)
	}
}

func TestDropAmbiguousCodesConservaRepetidoDelMismoArticulo(t *testing.T) {
	// El mismo artículo en dos renglones (compra fraccionada) NO vuelve ambiguo al código.
	doc := PurchaseDoc{Lines: []PurchaseLine{
		{RawCode: "242883", RawName: "AGUA MINERA"},
		{RawCode: "242883", RawName: "AGUA MINERA"},
	}}
	doc.DropAmbiguousCodes()
	if doc.Lines[0].RawCode != "242883" {
		t.Error("el mismo nombre repetido no es ambigüedad: el código debe conservarse")
	}
	if len(doc.Warnings) != 0 {
		t.Errorf("no debe advertir nada: %v", doc.Warnings)
	}
}

func TestReconcileReportaImportesIlegibles(t *testing.T) {
	doc := PurchaseDoc{
		Lines:   []PurchaseLine{{RawName: "AGUACATE HASS", Amount: "ilegible"}},
		Charges: []PurchaseCharge{{Label: "IMPUESTO", Amount: "??"}},
		Total:   "640.87",
	}
	r := doc.Reconcile()
	if len(r.Unreadable) != 2 {
		t.Fatalf("quiero 2 renglones ilegibles, tengo %d: %v", len(r.Unreadable), r.Unreadable)
	}
	// Un importe ilegible NO puede contar como cero: eso daría un total "cuadrado" y falso.
	if r.Balanced() {
		t.Error("con renglones ilegibles el documento no puede reportarse cuadrado")
	}
}

func TestLineStatusReceived(t *testing.T) {
	tests := []struct {
		status PurchaseLineStatus
		want   bool
	}{
		{LineComprado, true},
		{LineAjustado, true}, // llegó, con otra cantidad
		{LineNoDisponible, false},
		{"", true}, // el documento no dice nada: un ticket impreso ya se lo llevaron
	}
	for _, tt := range tests {
		if got := tt.status.Received(); got != tt.want {
			t.Errorf("%q.Received() = %v, quiero %v", tt.status, got, tt.want)
		}
	}
}

// Un slice nil de Go serializa como null y revienta al consumidor que lo recorre; el front ya
// falló así con reconciliation.unreadable.
func TestListasNuncaSonNil(t *testing.T) {
	var d PurchaseDoc
	d.EnsureLists()
	if d.Lines == nil || d.Charges == nil || d.Payments == nil || d.Extra == nil || d.Warnings == nil {
		t.Error("EnsureLists debe dejar todas las colecciones no-nil (JSON [] y no null)")
	}
	if r := (PurchaseDoc{}).Reconcile(); r.Unreadable == nil {
		t.Error("Reconciliation.Unreadable debe ser no-nil aunque no haya nada ilegible")
	}
}

// El front mete estos strings en <input type="number"> y los multiplica: parseFloat("1,386.93")
// es 1, no 1386.93. Un separador de miles sin normalizar guarda un peso en vez de mil.
func TestNormalizeAmounts(t *testing.T) {
	d := PurchaseDoc{
		Subtotal: "$ 3,847.40",
		Total:    "$4,093.33",
		Lines: []PurchaseLine{{
			Qty: "2", PackQty: "2.26", UnitPrice: "$ 1,234.50", Amount: "2,469.00",
			UnitPriceAlt: "1,111.11", AmountAlt: "(199.00)",
		}},
		Charges:  []PurchaseCharge{{Label: "Descuento", Amount: "-$199.00"}, {Label: "Envío", Amount: "$44.10"}},
		Payments: []PurchasePayment{{Method: "VISA", Amount: "$ 1,386.93"}},
	}
	d.NormalizeAmounts()

	for _, tt := range []struct{ got, want, campo string }{
		{d.Subtotal, "3847.4", "subtotal"},
		{d.Total, "4093.33", "total"},
		{d.Lines[0].UnitPrice, "1234.5", "unitPrice"},
		{d.Lines[0].Amount, "2469", "amount"},
		{d.Lines[0].UnitPriceAlt, "1111.11", "unitPriceAlt"},
		{d.Lines[0].AmountAlt, "-199", "amountAlt (paréntesis = negativo)"},
		{d.Charges[0].Amount, "-199", "cargo negativo"},
		{d.Charges[1].Amount, "44.1", "cargo"},
		{d.Payments[0].Amount, "1386.93", "pago"},
	} {
		if tt.got != tt.want {
			t.Errorf("%s = %q, quiero %q", tt.campo, tt.got, tt.want)
		}
	}

	// Toda salida tiene que ser parseable por el front sin ayuda: es la garantía del método.
	for _, s := range []string{d.Subtotal, d.Total, d.Lines[0].UnitPrice, d.Lines[0].Amount,
		d.Charges[0].Amount, d.Payments[0].Amount} {
		if _, err := decimal.NewFromString(s); err != nil {
			t.Errorf("%q no es un número canónico: %v", s, err)
		}
	}
}

func TestNormalizeAmountsCamposIlegibles(t *testing.T) {
	// Basura y vacío no deben convertirse en un número inventado ni en NaN al otro lado.
	d := PurchaseDoc{
		Total: "ver reverso",
		Lines: []PurchaseLine{{Qty: "s/n", PackQty: "", Amount: "—", UnitPrice: "$"}},
	}
	d.NormalizeAmounts()
	for campo, got := range map[string]string{
		"total": d.Total, "qty": d.Lines[0].Qty, "packQty": d.Lines[0].PackQty,
		"amount": d.Lines[0].Amount, "unitPrice": d.Lines[0].UnitPrice,
	} {
		if got != "" {
			t.Errorf("%s = %q, quiero \"\" (ilegible, que lo llene el operador)", campo, got)
		}
	}

	// Una cantidad negativa no existe en una compra: se descarta en vez de restar stock.
	neg := PurchaseDoc{Lines: []PurchaseLine{{Qty: "-3"}}}
	neg.NormalizeAmounts()
	if neg.Lines[0].Qty != "" {
		t.Errorf("qty negativa = %q, quiero \"\"", neg.Lines[0].Qty)
	}
}

// Vaciar un importe ilegible sin avisar lo borra de la reconciliación —que solo reporta ilegible
// lo que NO está vacío— y el documento cuadraría fingiendo que ese cargo nunca existió.
func TestNormalizeAmountsAvisaLoQueNoPudoLeer(t *testing.T) {
	d := PurchaseDoc{
		Charges: []PurchaseCharge{{Label: "Costo de envío", Amount: "GRATIS", AffectsTotal: true}},
		Lines:   []PurchaseLine{{RawName: "PAN BOLILLO", Amount: "s/i", Status: LineComprado}},
	}
	d.NormalizeAmounts()
	if len(d.Warnings) != 2 {
		t.Fatalf("advertencias = %v, quiero una por cada campo ilegible", d.Warnings)
	}
	for _, want := range []string{"Costo de envío", "GRATIS", "PAN BOLILLO"} {
		if !strings.Contains(strings.Join(d.Warnings, " | "), want) {
			t.Errorf("las advertencias no mencionan %q: %v", want, d.Warnings)
		}
	}
	// Y la segunda pasada no vuelve a avisar de lo mismo (ya está vacío).
	d.NormalizeAmounts()
	if len(d.Warnings) != 2 {
		t.Errorf("advertencias duplicadas al re-normalizar: %v", d.Warnings)
	}
}

// Normalizar corre ANTES de la aritmética: si el número quedara sucio, ParseAmount lo descartaría
// y el documento "no cuadraría" por un separador de miles.
func TestNormalizeAmountsEsIdempotente(t *testing.T) {
	d := PurchaseDoc{Total: "$1,000.50", Lines: []PurchaseLine{{Qty: "1", Amount: "1,000.50"}}}
	d.NormalizeAmounts()
	once := d.Total + "|" + d.Lines[0].Amount
	d.NormalizeAmounts()
	if got := d.Total + "|" + d.Lines[0].Amount; got != once {
		t.Errorf("segunda pasada cambió el resultado: %q → %q", once, got)
	}
	if r := d.Reconcile(); !r.Balanced() {
		t.Errorf("tras normalizar el documento debe cuadrar, diff = %s", r.Diff)
	}
}
