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
	// Efectivo: esperado 175 = fondo 100 + neto 25 + ventas 50. Tarjeta: 40 de ventas.
	methods := []methodExpected{
		{name: "Efectivo", expected: mustDec("175"), affectsCash: true},
		{name: "Tarjeta", expected: mustDec("40"), affectsCash: false},
	}

	b := corteBreakdown(mustDec("100"), methods, moves)

	if !b.IngresosTotal.Equal(mustDec("140")) {
		t.Fatalf("ingresosTotal = %s, want 140", b.IngresosTotal)
	}
	if !b.EgresosTotal.Equal(mustDec("25")) {
		t.Fatalf("egresosTotal = %s, want 25", b.EgresosTotal)
	}
	if len(b.Ingresos) != 2 {
		t.Fatalf("ingresos por método = %d, want 2", len(b.Ingresos))
	}

	ef := b.Ingresos[0]
	if ef.Method != "Efectivo" || !ef.Total.Equal(mustDec("100")) {
		t.Fatalf("efectivo total = %s (%s), want 100", ef.Total, ef.Method)
	}
	wantItems := map[string]string{"Ventas": "50", "Entradas": "20", "Traspasos recibidos": "30"}
	if len(ef.Items) != len(wantItems) {
		t.Fatalf("efectivo items = %d, want %d", len(ef.Items), len(wantItems))
	}
	for _, it := range ef.Items {
		w, ok := wantItems[it.Concept]
		if !ok || !it.Amount.Equal(mustDec(w)) {
			t.Fatalf("efectivo item %q = %s, want %s", it.Concept, it.Amount, w)
		}
	}

	if b.Ingresos[1].Method != "Tarjeta" || !b.Ingresos[1].Total.Equal(mustDec("40")) {
		t.Fatalf("tarjeta = %s (%s), want 40", b.Ingresos[1].Total, b.Ingresos[1].Method)
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
