package domain

import (
	"strings"

	"github.com/shopspring/decimal"
)

// Extracción de documentos de compra: ticket, factura o pedido, de cualquier proveedor.
//
// El tipo es deliberadamente laxo. Cada proveedor imprime lo que quiere (Soriana trae
// IMPUESTO y DONATIVO, Walmart Descuento y Envío a domicilio, Sam's solo SUBTOTAL/TOTAL),
// así que los conceptos que no son mercancía van como LISTA ETIQUETADA (Charges) en vez de
// una columna por concepto, y lo que no modelamos cae en Extra. Agregar un proveedor nuevo
// no debe requerir tocar esta estructura ni el esquema de la BD.
//
// Los importes viajan como string con el texto exacto que devolvió el extractor: convertir
// a decimal aquí perdería la evidencia de un valor ilegible, y el operador tiene que poder
// ver "lo que decía el papel" cuando algo no cuadra. ParseAmount hace la conversión y
// Reconcile reporta lo que no pudo leer.

// PurchaseDocKind es libre a propósito (no un enum): el extractor clasifica lo que ve y un
// tipo de documento nuevo no debe romper la ingesta.
type PurchaseDoc struct {
	Kind     string `json:"kind"`     // ticket | factura | pedido | otro
	Supplier string `json:"supplier"` // nombre tal como aparece en el documento
	Folio    string `json:"folio"`    // folio/pedido/ticket #, "" si no trae
	// IssuedOn es YYYY-MM-DD, o "" cuando el documento no la imprime (el ticket de Sam's) o
	// la trae incompleta (Walmart dice "16 de julio", sin año). Nunca la inventamos: el
	// operador la confirma.
	IssuedOn string            `json:"issuedOn"`
	Currency string            `json:"currency"` // MXN si el documento no lo dice
	Lines    []PurchaseLine    `json:"lines"`
	Charges  []PurchaseCharge  `json:"charges"`
	Payments []PurchasePayment `json:"payments"`
	Subtotal string            `json:"subtotal"`
	Total    string            `json:"total"`
	// Extra: cualquier dato del documento que no modelamos (RFC, sucursal, terminal, socio,
	// cajero…). Se guarda tal cual para no perder información entre versiones del extractor.
	// Es lista y no mapa porque los structured outputs prohíben claves arbitrarias
	// (additionalProperties:false) — y de paso conserva el orden en que venía en el papel.
	Extra []DocField `json:"extra"`
	// Warnings: lo que el extractor no pudo leer con confianza. No es error: la UI lo muestra
	// y el operador decide.
	Warnings []string `json:"warnings"`
}

// DocField es un dato suelto del documento que la estructura no modela.
type DocField struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type PurchaseLine struct {
	// RawCode: código del proveedor (SKU de Sam's). "" cuando el documento no lo trae
	// (Walmart) o cuando es un código de departamento reutilizado entre artículos distintos
	// (Soriana repite 005 en dos panes) — en ese caso el extractor lo deja fuera porque no
	// identifica al artículo y envenenaría el mapeo aprendido.
	RawCode string `json:"rawCode"`
	RawName string `json:"rawName"` // texto del documento, tal cual, sin normalizar
	// Qty puede ser fraccionaria: los artículos a granel se venden por peso
	// (0.280 kg de aguacate).
	Qty  string `json:"qty"`
	Unit string `json:"unit"` // como lo dice el documento: pieza, kg, g, l, ml…
	// UnitPrice/Amount: lo COBRADO, no el precio de lista. Los documentos traen varias
	// columnas (Soriana: NORMAL/PROMOCIÓN/FINAL; Walmart: lista con IVA y unitario sin IVA)
	// y solo una es lo que salió del bolsillo.
	UnitPrice string `json:"unitPrice"`
	Amount    string `json:"amount"`
	// UnitPriceAlt/AmountAlt: el OTRO precio del renglón cuando el documento imprime dos.
	// Elegir entre ellos es aritmética (¿cuál suma al subtotal impreso?), no lectura, así que
	// el extractor transcribe ambos y PickPriceColumn decide. Pedirle al modelo que verifique
	// la suma de veinte renglones es lento y falla; sumar es lo que el código hace exacto.
	UnitPriceAlt string `json:"unitPriceAlt"`
	AmountAlt    string `json:"amountAlt"`
	// Status: un pedido puede traer líneas que no llegaron. Walmart marca "No disponible"
	// (no llegó) y "Peso ajustado" (llegó con otra cantidad). Sin esto, una línea pedida y
	// no surtida entraría al almacén.
	Status PurchaseLineStatus `json:"status"`
	// PackQty/PackUnit: contenido de UNA unidad de compra, cuando el nombre lo dice
	// ("Harina … 432 g" → 432 g; "MM 2K FRESA" → 2 kg). Es lo que convierte "4 piezas" a
	// gramos de almacén; sin él una pieza no se puede descontar de un ingrediente en g/ml.
	PackQty  string `json:"packQty"`
	PackUnit string `json:"packUnit"`
	// SuggestedName: nombre limpio y buscable derivado de RawName, para proponer el alta del
	// artículo cuando no hay coincidencia en el catálogo.
	SuggestedName string `json:"suggestedName"`
	Note          string `json:"note"`
}

// PurchaseLineStatus refleja lo que el documento dice del renglón. Vacío = el documento no
// lo indica (un ticket de mostrador: si está impreso, te lo llevaste).
type PurchaseLineStatus string

const (
	LineComprado     PurchaseLineStatus = "comprado"
	LineNoDisponible PurchaseLineStatus = "no_disponible"
	LineAjustado     PurchaseLineStatus = "ajustado"
)

// Received indica si la línea debe tocar el almacén. Solo no_disponible lo impide.
func (s PurchaseLineStatus) Received() bool { return s != LineNoDisponible }

// EffectiveAmount devuelve el importe del renglón: el impreso si lo hay, y si no cantidad ×
// unitario. Existe porque hay documentos que solo imprimen el precio "c/u" y nunca el total
// del renglón — un renglón de 3 piezas a 17.00 no trae 51.00 en ninguna parte del papel.
func (l PurchaseLine) EffectiveAmount() (decimal.Decimal, bool) {
	return lineAmount(l.Amount, l.UnitPrice, l.Qty)
}

// EffectiveAmountAlt es lo mismo sobre la segunda columna de precios (ver PickPriceColumn).
func (l PurchaseLine) EffectiveAmountAlt() (decimal.Decimal, bool) {
	return lineAmount(l.AmountAlt, l.UnitPriceAlt, l.Qty)
}

func lineAmount(amount, unitPrice, qty string) (decimal.Decimal, bool) {
	if v, ok := ParseAmount(amount); ok {
		return v, true
	}
	p, okP := ParseAmount(unitPrice)
	if !okP {
		return decimal.Zero, false
	}
	// Sin cantidad impresa la línea es de una unidad: los tickets omiten el "1".
	q := decimal.NewFromInt(1)
	if v, ok := ParseQty(qty); ok && !v.IsZero() {
		q = v
	}
	return Round2(p.Mul(q)), true
}

// PurchaseCharge es todo importe del documento que NO es mercancía: impuestos, envío,
// descuentos (negativos), bolsa, propina, donativo. Label es el texto del documento para
// que el operador reconozca la línea sin traducirla.
type PurchaseCharge struct {
	Label  string `json:"label"`
	Amount string `json:"amount"`
	// AffectsTotal distingue un cargo que MUEVE el total (descuento, envío, bolsa) de un
	// DESGLOSE de algo ya contenido en los importes de línea. Un ticket de mostrador imprime
	// "TOTAL IVA" como información — los precios ya lo traen dentro — y sumarlo cuadraría el
	// documento con el impuesto contado dos veces.
	AffectsTotal bool `json:"affectsTotal"`
}

// PurchasePayment: un documento puede traer varios (Soriana cobra 640.86 a tarjeta y 0.01
// en efectivo en el mismo ticket).
type PurchasePayment struct {
	Method    string `json:"method"` // texto del documento: TARJETA CREDITO, EFECTIVO, VISA…
	Amount    string `json:"amount"`
	Reference string `json:"reference"` // últimos dígitos, autorización… "" si no trae
}

// Reconciliation compara el documento contra sí mismo. Es informativo, NUNCA bloqueante:
// en un ticket de mostrador la suma de líneas da el total exacto, pero en un pedido con
// descuento y envío a nivel documento no puede dar — y las dos situaciones son válidas.
type Reconciliation struct {
	LinesSum decimal.Decimal // suma de los importes de línea legibles (sin las no surtidas)
	// ChargesSum: solo los cargos que mueven el total (descuento, envío…).
	ChargesSum decimal.Decimal
	// BreakdownSum: los desglosados que ya venían dentro de las líneas (IVA/IEPS de un ticket
	// con precios impuesto-incluido). Se reporta para que el operador lo vea, pero NO entra al
	// cuadre: sumarlo contaría el impuesto dos veces.
	BreakdownSum decimal.Decimal
	Total        decimal.Decimal // total declarado por el documento
	Diff         decimal.Decimal // Total − (LinesSum + ChargesSum)
	PaymentsSum  decimal.Decimal
	// Subtotal/SubtotalDiff: cuando el documento declara subtotal, comparar contra él es un
	// check MÁS FILOSO que el del total, porque no lo enturbian descuentos ni envío. Un
	// documento que imprime dos precios por renglón (con y sin impuesto) se detecta aquí: la
	// diferencia sale exactamente igual al impuesto agregado de todas las líneas.
	Subtotal     decimal.Decimal
	HasSubtotal  bool
	SubtotalDiff decimal.Decimal // Subtotal − LinesSum
	// Unreadable: importes que no se pudieron interpretar, con su texto original, para que la
	// UI señale el renglón exacto en vez de mostrar un total silenciosamente incompleto.
	Unreadable []string
}

// Balanced indica que líneas + cargos explican el total (tolerancia de un centavo por el
// redondeo del propio documento).
func (r Reconciliation) Balanced() bool { return r.Diff.Abs().LessThanOrEqual(oneCent) }

// PaymentsMatchTotal indica que los pagos declarados cubren el total.
func (r Reconciliation) PaymentsMatchTotal() bool {
	return r.PaymentsSum.Sub(r.Total).Abs().LessThanOrEqual(oneCent)
}

var oneCent = decimal.New(1, -2)

// Reconcile suma lo legible y reporta lo que no. Función pura: es el check que corre sobre
// la extracción antes de mostrarla, sin BD ni red.
func (d PurchaseDoc) Reconcile() Reconciliation {
	// Unreadable arranca no-nil para que serialice como [] y no como null: el consumidor la
	// recorre siempre, y un null revienta el spread en el front.
	r := Reconciliation{Unreadable: []string{}}
	for _, l := range d.Lines {
		// Una línea que no llegó no suma al documento pagado.
		if !l.Status.Received() {
			continue
		}
		if v, ok := l.EffectiveAmount(); ok {
			r.LinesSum = r.LinesSum.Add(v)
		} else if strings.TrimSpace(l.Amount) != "" || strings.TrimSpace(l.UnitPrice) != "" {
			r.Unreadable = append(r.Unreadable,
				"importe de «"+l.RawName+"»: "+strings.TrimSpace(l.Amount+" "+l.UnitPrice))
		}
	}
	for _, c := range d.Charges {
		v, ok := ParseAmount(c.Amount)
		if !ok {
			if strings.TrimSpace(c.Amount) != "" {
				r.Unreadable = append(r.Unreadable, c.Label+": "+c.Amount)
			}
			continue
		}
		if c.AffectsTotal {
			r.ChargesSum = r.ChargesSum.Add(v)
		} else {
			r.BreakdownSum = r.BreakdownSum.Add(v)
		}
	}
	for _, p := range d.Payments {
		if v, ok := ParseAmount(p.Amount); ok {
			r.PaymentsSum = r.PaymentsSum.Add(v)
		} else if strings.TrimSpace(p.Amount) != "" {
			r.Unreadable = append(r.Unreadable, "pago "+p.Method+": "+p.Amount)
		}
	}
	if v, ok := ParseAmount(d.Total); ok {
		r.Total = v
	} else if strings.TrimSpace(d.Total) != "" {
		r.Unreadable = append(r.Unreadable, "total: "+d.Total)
	}
	if v, ok := ParseAmount(d.Subtotal); ok {
		r.Subtotal, r.HasSubtotal = v, true
	}
	r.LinesSum = Round2(r.LinesSum)
	r.ChargesSum = Round2(r.ChargesSum)
	r.BreakdownSum = Round2(r.BreakdownSum)
	r.PaymentsSum = Round2(r.PaymentsSum)
	r.Diff = Round2(r.Total.Sub(r.LinesSum.Add(r.ChargesSum)))
	r.SubtotalDiff = Round2(r.Subtotal.Sub(r.LinesSum))
	return r
}

// LinesMatchSubtotal indica que los importes de línea explican el subtotal impreso. Cuando el
// documento no declara subtotal devuelve true (no hay nada que contradecir).
//
// Tolera un caso legítimo: hay tickets cuyo SUBTOTAL es pre-impuesto mientras los precios de
// renglón ya lo incluyen. Ahí la diferencia es exactamente el impuesto desglosado, y tratarlo
// como discrepancia sería una falsa alarma en cada compra de ese proveedor.
func (r Reconciliation) LinesMatchSubtotal() bool {
	if !r.HasSubtotal {
		return true
	}
	if r.SubtotalDiff.Abs().LessThanOrEqual(oneCent) {
		return true
	}
	// Se comparan magnitudes porque el desfase puede ir en cualquier sentido: subtotal
	// pre-impuesto con renglones impuesto-incluido, o al revés.
	return r.SubtotalDiff.Abs().Sub(r.BreakdownSum.Abs()).Abs().LessThanOrEqual(oneCent)
}

// NormalizeAmounts reescribe cada campo numérico a su forma canónica ("$ 1,386.93" → "1386.93").
//
// El extractor transcribe el importe TAL COMO SE IMPRIME, y ese texto viaja al front, que lo mete
// en <input type="number"> y lo multiplica. Un separador de miles ahí no es cosmético: JS hace
// parseFloat("1,386.93") = 1 y el gasto se guarda con un peso en vez de mil trescientos. Se
// normaliza una sola vez aquí, en el borde, para que el resto del sistema —Go, el wire y el
// front— vea el mismo número.
//
// Lo que no se puede leer queda en "" CON una advertencia: vaciarlo en silencio lo borraría de
// la reconciliación (que solo reporta ilegible lo que no está vacío) y el documento cuadraría
// fingiendo que ese cargo no existía.
func (d *PurchaseDoc) NormalizeAmounts() {
	clean := func(s *string, what string, parse func(string) (decimal.Decimal, bool)) {
		if *s == "" {
			return
		}
		if v, ok := parse(*s); ok {
			*s = v.String()
			return
		}
		d.Warnings = append(d.Warnings, "no se pudo leer "+what+": «"+*s+"»")
		*s = ""
	}
	amount := func(s *string, what string) { clean(s, what, ParseAmount) }
	qty := func(s *string, what string) { clean(s, what, ParseQty) }

	amount(&d.Subtotal, "el subtotal")
	amount(&d.Total, "el total")
	for i := range d.Lines {
		l := &d.Lines[i]
		of := " de «" + l.RawName + "»"
		qty(&l.Qty, "la cantidad"+of)
		qty(&l.PackQty, "el contenido"+of)
		amount(&l.UnitPrice, "el precio unitario"+of)
		amount(&l.Amount, "el importe"+of)
		amount(&l.UnitPriceAlt, "el precio alterno"+of)
		amount(&l.AmountAlt, "el importe alterno"+of)
	}
	for i := range d.Charges {
		amount(&d.Charges[i].Amount, "el cargo «"+d.Charges[i].Label+"»")
	}
	for i := range d.Payments {
		amount(&d.Payments[i].Amount, "el pago «"+d.Payments[i].Method+"»")
	}
}

// EnsureLists deja todas las colecciones no-nil, para que la respuesta JSON traiga [] y nunca
// null: el consumidor las recorre sin comprobar, y un null rompe el spread en el front (pasó).
func (d *PurchaseDoc) EnsureLists() {
	if d.Lines == nil {
		d.Lines = []PurchaseLine{}
	}
	if d.Charges == nil {
		d.Charges = []PurchaseCharge{}
	}
	if d.Payments == nil {
		d.Payments = []PurchasePayment{}
	}
	if d.Extra == nil {
		d.Extra = []DocField{}
	}
	if d.Warnings == nil {
		d.Warnings = []string{}
	}
}

// PickPriceColumn elige, entre los dos precios que puede traer un renglón, el que hace que las
// líneas expliquen el subtotal impreso — y si es el alterno, los intercambia.
//
// Existe porque un documento puede imprimir precio con y sin impuesto por renglón y solo uno
// suma al subtotal. Decidirlo comparando sumas es exacto e instantáneo; pedírselo al extractor
// es lento y se equivoca. Devuelve true si hubo intercambio.
//
// Sin subtotal impreso no hace nada: no habría con qué decidir, y cambiar precios a ciegas es
// peor que dejar el que el extractor eligió.
func (d *PurchaseDoc) PickPriceColumn() bool {
	r := d.Reconcile()
	if !r.HasSubtotal || r.LinesMatchSubtotal() {
		return false
	}
	// Suma los alternos, cayendo al principal cuando un renglón no trae alterno (una línea con
	// un solo precio cuenta igual en las dos hipótesis).
	var alt decimal.Decimal
	for _, l := range d.Lines {
		if !l.Status.Received() {
			continue
		}
		if v, ok := l.EffectiveAmountAlt(); ok {
			alt = alt.Add(v)
			continue
		}
		if v, ok := l.EffectiveAmount(); ok {
			alt = alt.Add(v)
		}
	}
	if Round2(r.Subtotal.Sub(alt)).Abs().GreaterThan(oneCent) {
		return false // el alterno tampoco cuadra: no hay razón para tocar nada
	}
	for i := range d.Lines {
		l := &d.Lines[i]
		if l.AmountAlt == "" {
			continue
		}
		l.Amount, l.AmountAlt = l.AmountAlt, l.Amount
		l.UnitPrice, l.UnitPriceAlt = l.UnitPriceAlt, l.UnitPrice
	}
	d.Warnings = append(d.Warnings,
		"se usó la segunda columna de precios del documento: es la que suma al subtotal impreso")
	return true
}

// DropAmbiguousCodes vacía los rawCode que aparecen en renglones con artículos DISTINTOS
// dentro del mismo documento: son códigos de departamento, no de artículo, y usarlos como
// llave del mapeo por proveedor fusionaría dos productos diferentes en una sola fila.
//
// Es un guard determinista y no una instrucción al extractor a propósito: el modelo puede
// detectarlo y aun así rellenar el campo, y el daño de un código mal usado no se ve hasta que
// el inventario ya está mal.
func (d *PurchaseDoc) DropAmbiguousCodes() {
	names := map[string]string{} // rawCode → primer rawName visto
	ambiguous := map[string]bool{}
	for _, l := range d.Lines {
		if l.RawCode == "" {
			continue
		}
		if prev, seen := names[l.RawCode]; seen && prev != l.RawName {
			ambiguous[l.RawCode] = true
		} else if !seen {
			names[l.RawCode] = l.RawName
		}
	}
	for code := range ambiguous {
		d.Warnings = append(d.Warnings,
			"el código "+code+" identifica artículos distintos (es de departamento, no de artículo): se ignora para el mapeo")
	}
	for i := range d.Lines {
		if ambiguous[d.Lines[i].RawCode] {
			d.Lines[i].RawCode = ""
		}
	}
}

// ParseAmount lee un importe tal como lo imprime un documento mexicano: con símbolo,
// separador de miles y paréntesis o guion para negativos ("$ 3,847.40", "-199.00",
// "(199.00)"). Devuelve false si no queda un número, para que el renglón se marque
// ilegible en vez de contar como cero y descuadrar el total en silencio.
func ParseAmount(s string) (decimal.Decimal, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero, false
	}
	neg := strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")")
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r == '.':
			b.WriteRune(r)
		case r == '-' && b.Len() == 0:
			b.WriteRune(r)
		}
	}
	v, err := decimal.NewFromString(b.String())
	if err != nil {
		return decimal.Zero, false
	}
	if neg {
		v = v.Neg()
	}
	return v, true
}

// ParseQty lee una cantidad (puede ser fraccionaria: 0.280 kg). Separada de ParseAmount
// porque una cantidad no lleva símbolo de moneda y no admite negativos.
func ParseQty(s string) (decimal.Decimal, bool) {
	v, ok := ParseAmount(s)
	if !ok || v.IsNegative() {
		return decimal.Zero, false
	}
	return v, true
}
