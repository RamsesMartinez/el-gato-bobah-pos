package app

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

func mustDec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

// corteBreakdown descompone el corte en ingresos (por método → concepto) y egresos de efectivo.
// Cubre la derivación de ventas de efectivo (esperado − fondo − neto), la separación de
// entradas/traspasos/gastos y que los conceptos en cero se omiten.
func TestCorteBreakdown(t *testing.T) {
	tid, eid := int64(1), int64(1)
	moves := []db.ListCashMovementsRow{
		{Kind: domain.CashEntrada, Amount: mustDec("20")},                   // entrada manual
		{Kind: domain.CashSalida, Amount: mustDec("10")},                    // salida manual
		{Kind: domain.CashEntrada, Amount: mustDec("30"), TransferID: &tid}, // traspaso recibido
		{Kind: domain.CashSalida, Amount: mustDec("15"), ExpenseID: &eid},   // gasto en efectivo
	}
	// Esperado = ventas + propinas (+ fondo/neto en efectivo).
	// Efectivo: 180 = fondo 100 + neto 25 + ventas 50 + propina 5. Tarjeta: 50 = ventas 40 + propina 10.
	methods := []methodExpected{
		{name: "Efectivo", expected: mustDec("180"), tips: mustDec("5"), affectsCash: true},
		{name: "Tarjeta", expected: mustDec("50"), tips: mustDec("10"), affectsCash: false},
	}

	b := corteBreakdown(mustDec("100"), methods, moves)

	if !b.IngresosTotal.Equal(mustDec("155")) { // efectivo 105 + tarjeta 50
		t.Fatalf("ingresosTotal = %s, want 155", b.IngresosTotal)
	}
	if !b.EgresosTotal.Equal(mustDec("25")) {
		t.Fatalf("egresosTotal = %s, want 25", b.EgresosTotal)
	}
	if len(b.Ingresos) != 2 {
		t.Fatalf("ingresos por método = %d, want 2", len(b.Ingresos))
	}

	ef := b.Ingresos[0]
	if ef.Method != "Efectivo" || !ef.Total.Equal(mustDec("105")) {
		t.Fatalf("efectivo total = %s (%s), want 105", ef.Total, ef.Method)
	}
	wantItems := map[string]string{"Ventas": "50", "Propinas": "5", "Entradas": "20", "Traspasos recibidos": "30"}
	if len(ef.Items) != len(wantItems) {
		t.Fatalf("efectivo items = %d, want %d", len(ef.Items), len(wantItems))
	}
	for _, it := range ef.Items {
		w, ok := wantItems[it.Concept]
		if !ok || !it.Amount.Equal(mustDec(w)) {
			t.Fatalf("efectivo item %q = %s, want %s", it.Concept, it.Amount, w)
		}
	}

	// Tarjeta: Ventas 40 + Propinas 10 = 50.
	tj := b.Ingresos[1]
	if tj.Method != "Tarjeta" || !tj.Total.Equal(mustDec("50")) {
		t.Fatalf("tarjeta total = %s (%s), want 50", tj.Total, tj.Method)
	}
	wantTj := map[string]string{"Ventas": "40", "Propinas": "10"}
	for _, it := range tj.Items {
		w, ok := wantTj[it.Concept]
		if !ok || !it.Amount.Equal(mustDec(w)) {
			t.Fatalf("tarjeta item %q = %s, want %s", it.Concept, it.Amount, w)
		}
	}

	// Egresos: Gastos 15 + Salidas 10; "Traspasos enviados" (0) se omite.
	wantEg := map[string]string{"Gastos": "15", "Salidas de efectivo": "10"}
	if len(b.Egresos) != len(wantEg) {
		t.Fatalf("egresos = %d, want %d", len(b.Egresos), len(wantEg))
	}
	for _, it := range b.Egresos {
		w, ok := wantEg[it.Concept]
		if !ok || !it.Amount.Equal(mustDec(w)) {
			t.Fatalf("egreso %q = %s, want %s", it.Concept, it.Amount, w)
		}
	}
}
