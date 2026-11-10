package domain

import (
	"slices"
	"testing"

	"github.com/shopspring/decimal"
)

func TestMissingCloseDeclarations(t *testing.T) {
	efectivo := CloseMethod{ID: 1, Name: "Efectivo", InDrawer: true}
	tarjeta := CloseMethod{ID: 2, Name: "Tarjeta"}
	transfer := CloseMethod{ID: 3, Name: "Transferencia"}
	didi := CloseMethod{ID: 4, Name: "Didi", AutoDeclare: true}
	req := CloseRequirements{DrawerNeedsCount: true, Methods: []CloseMethod{efectivo, tarjeta, transfer, didi}}
	cases := []struct {
		name  string
		req   CloseRequirements
		given CloseDeclarations
		want  []string
	}{
		{"todo dictado", req, CloseDeclarations{DrawerCounted: true, Declared: map[int]bool{2: true, 3: true}}, nil},
		{"sin el cajón: el agente no puede cerrar con lo que el sistema espera", req,
			CloseDeclarations{Declared: map[int]bool{2: true, 3: true}}, []string{"el cajón (efectivo contado)"}},
		{"un método sin cifra se nombra, en el orden del turno", req,
			CloseDeclarations{DrawerCounted: true, Declared: map[int]bool{3: true}}, []string{"Tarjeta"}},
		{"faltan dos", req, CloseDeclarations{DrawerCounted: true}, []string{"Tarjeta", "Transferencia"}},
		{"el cajón que no pide conteo no se exige", CloseRequirements{Methods: []CloseMethod{efectivo, tarjeta}},
			CloseDeclarations{Declared: map[int]bool{2: true}}, nil},
		{"la terminal que se arquea se pide", CloseRequirements{Terminals: []CloseTerminal{{ID: 9, Name: "Clip"}}},
			CloseDeclarations{}, []string{"la terminal Clip"}},
		{"la terminal dictada no se pide", CloseRequirements{Terminals: []CloseTerminal{{ID: 9, Name: "Clip"}}},
			CloseDeclarations{Terminals: map[int64]bool{9: true}}, nil},
		{"un método del cajón no se declara aparte ni se pide", CloseRequirements{DrawerNeedsCount: true, Methods: []CloseMethod{efectivo}},
			CloseDeclarations{DrawerCounted: true}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MissingCloseDeclarations(c.req, c.given); !slices.Equal(got, c.want) {
				t.Fatalf("faltan %v, esperaba %v", got, c.want)
			}
		})
	}
}

// Un «0.00» dictado es una cifra: el método no vendió nada. Confundirlo con «no dictado» rechazaría
// el cierre de una caja sin ventas con tarjeta.
func TestADeclaredZeroCountsAsDeclared(t *testing.T) {
	d := DeclarationsFrom(false, map[string]string{"2": "0.00"}, nil)
	if !d.Declared[2] {
		t.Fatal("un cero dictado no contó como declarado")
	}
	if d := DeclarationsFrom(false, map[string]string{"x": "10"}, nil); len(d.Declared) != 0 {
		t.Fatal("una llave que no es método no cuenta")
	}
}

func TestCancelSummary(t *testing.T) {
	d := func(s string) decimal.Decimal { return decimal.RequireFromString(s) }
	if s := CancelSummary(nil); s.NeedsRefund || s.NeedsCardFolio || len(s.ByMethod) != 0 {
		t.Fatalf("sin cobros no hay devolución: %+v", s)
	}
	s := CancelSummary([]CancelPayment{{Method: "Efectivo", Amount: d("100")}, {Method: "Efectivo", Amount: d("20.50")}})
	if !s.NeedsRefund || s.NeedsCardFolio || len(s.ByMethod) != 1 || !s.ByMethod[0].Amount.Equal(d("120.50")) {
		t.Fatalf("efectivo: %+v", s)
	}
	s = CancelSummary([]CancelPayment{{Method: "Efectivo", Amount: d("50")}, {Method: "Tarjeta", Amount: d("80"), IsCard: true}})
	if !s.NeedsCardFolio || len(s.ByMethod) != 2 || s.ByMethod[1].Method != "Tarjeta" || !s.Total.Equal(d("130")) {
		t.Fatalf("con tarjeta pide folio: %+v", s)
	}
}
