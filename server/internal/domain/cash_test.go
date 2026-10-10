package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestValidCashKind(t *testing.T) {
	for _, k := range []string{CashEntrada, CashSalida} {
		if !ValidCashKind(k) {
			t.Errorf("ValidCashKind(%q) = false, want true", k)
		}
	}
	for _, k := range []string{"", "deposito", "Entrada", "salidas"} {
		if ValidCashKind(k) {
			t.Errorf("ValidCashKind(%q) = true, want false", k)
		}
	}
}

func TestValidTransfer(t *testing.T) {
	ok := decimal.NewFromInt(100)
	cases := []struct {
		name     string
		from, to int64
		amount   decimal.Decimal
		want     bool
	}{
		{"ok", 1, 2, ok, true},
		{"same register", 1, 1, ok, false},
		{"zero amount", 1, 2, decimal.Zero, false},
		{"negative amount", 1, 2, decimal.NewFromInt(-5), false},
		{"over cap", 1, 2, MaxMoney.Add(decimal.NewFromInt(1)), false},
		{"bad from id", 0, 2, ok, false},
		{"bad to id", 1, 0, ok, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidTransfer(c.from, c.to, c.amount); got != c.want {
				t.Errorf("ValidTransfer(%d,%d,%s) = %v, want %v", c.from, c.to, c.amount, got, c.want)
			}
		})
	}
}

func TestResolveDeclared(t *testing.T) {
	expected := decimal.NewFromInt(500)
	clientDeclared := decimal.NewFromInt(320)

	cases := []struct {
		name        string
		autoDeclare bool
		want        decimal.Decimal
	}{
		{"auto-declare ignores client value, uses expected", true, expected},
		{"manual keeps client value", false, clientDeclared},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveDeclared(c.autoDeclare, expected, clientDeclared)
			if !got.Equal(c.want) {
				t.Errorf("ResolveDeclared(%v, %s, %s) = %s, want %s", c.autoDeclare, expected, clientDeclared, got, c.want)
			}
		})
	}
}

// UNA DEVOLUCIÓN SE CLASIFICA IGUAL SALGA O NO DEL CAJÓN (spec 029).
//
// El corte mandaba la de efectivo a «Salidas de efectivo» y la de tarjeta a «Devoluciones» dentro de
// Ingresos, y contaba la propina devuelta dentro de lo devuelto mientras Ventas la dejaba fuera. Las
// dos pantallas decían cifras distintas del mismo dinero.
func TestUnaDevolucionSeClasificaIgualSalgaONoDelCajon(t *testing.T) {
	r := MethodRefunds{
		OffDrawer: decimal.RequireFromString("35"), OffDrawerTips: decimal.RequireFromString("5"),
		Drawer: decimal.RequireFromString("110"), DrawerTips: decimal.RequireFromString("10"),
	}
	if got := r.Sale(); !got.Equal(decimal.RequireFromString("130")) {
		t.Fatalf("devolución de venta = %s, quiere 130: la propina devuelta se contó como venta devuelta", got)
	}
	if got := r.Tips(); !got.Equal(decimal.RequireFromString("15")) {
		t.Fatalf("propina devuelta = %s, quiere 15", got)
	}
	// Venta + propina devueltas = todo lo que salió, ni un peso más ni uno menos.
	if !r.Sale().Add(r.Tips()).Equal(r.OffDrawer.Add(r.Drawer)) {
		t.Fatal("la clasificación inventó o perdió dinero")
	}
}

// UN MEDIO EN NEGATIVO DICE POR QUÉ.
func TestUnMedioEnNegativoDicePorQue(t *testing.T) {
	if NegativeMethodNote(decimal.RequireFromString("-300")) == "" {
		t.Fatal("un medio en negativo sin nota deja al cajero buscando un faltante que no existe")
	}
	if NegativeMethodNote(decimal.Zero) != "" || NegativeMethodNote(decimal.RequireFromString("10")) != "" {
		t.Fatal("solo el negativo lleva nota")
	}
}
