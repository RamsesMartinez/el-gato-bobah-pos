package domain

import (
	"testing"
)

func TestSuggestPurchase(t *testing.T) {
	cajaDe100 := Pack{Name: "Caja de 100", QtyInBase: d("100"), UnitCost: dp("1.50")}
	paquete25 := Pack{Name: "Paquete de 25", QtyInBase: d("25"), UnitCost: dp("1.80")}
	barra := Pack{Name: "Barra 1 kg", QtyInBase: d("1000")}
	cases := []struct {
		name  string
		in    PurchaseInput
		packs int
		pack  string
		qty   string
		why   string
	}{
		{"redondea hacia arriba al empaque de menor costo por unidad",
			PurchaseInput{DailyUse: dp("22"), Measured: true, OnHand: d("0"), Unit: "pieza", Packs: []Pack{paquete25, cajaDe100}},
			4, "Caja de 100", "400", "se usan ~22 pzas al día, quedan 0 pzas, objetivo 14 días"},
		{"en kg se dice en kg",
			PurchaseInput{DailyUse: dp("400"), Measured: true, OnHand: d("1200"), Unit: "g", Packs: []Pack{barra}},
			5, "Barra 1 kg", "5000", "se usan ~400 g al día, quedan 1.2 kg, objetivo 14 días"},
		{"días de cobertura propios",
			PurchaseInput{DailyUse: dp("10"), Measured: true, OnHand: d("0"), Unit: "pieza", CoverageDays: 7, Packs: []Pack{paquete25}},
			3, "Paquete de 25", "75", "se usan ~10 pzas al día, quedan 0 pzas, objetivo 7 días"},
		{"sin empaques, en la unidad base redondeada hacia arriba",
			PurchaseInput{DailyUse: dp("0.5"), Measured: true, OnHand: d("0.2"), Unit: "pieza"},
			0, "", "7", "se usan ~0.5 pzas al día, quedan 0.2 pzas, objetivo 14 días"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := SuggestPurchase(c.in)
			if s.Suggested == nil {
				t.Fatalf("no sugirió: %q", s.Why)
			}
			if s.Suggested.Packs != c.packs || s.Suggested.PackName != c.pack || !s.Suggested.Qty.Equal(d(c.qty)) || s.Why != c.why {
				t.Fatalf("= %+v %q", *s.Suggested, s.Why)
			}
		})
	}

	t.Run("sin consumo medido no sugiere y lo dice", func(t *testing.T) {
		s := SuggestPurchase(PurchaseInput{OnHand: d("3"), Unit: "g"})
		if s.Suggested != nil || s.Why != "sin consumo medido: no está en ninguna receta o casi no se ha vendido" {
			t.Fatalf("%+v", s)
		}
	})
	t.Run("con la existencia en negativo pide conteo antes de comprar", func(t *testing.T) {
		s := SuggestPurchase(PurchaseInput{DailyUse: dp("3"), Measured: true, OnHand: d("-1"), Unit: "g"})
		if s.Suggested != nil || !s.NeedsCount {
			t.Fatalf("%+v", s)
		}
	})
	t.Run("si alcanza, no sugiere comprar", func(t *testing.T) {
		s := SuggestPurchase(PurchaseInput{DailyUse: dp("1"), Measured: true, OnHand: d("30"), Unit: "pieza"})
		if s.Suggested != nil || s.Why != "alcanza para 30 días" {
			t.Fatalf("%+v", s)
		}
	})
}

func TestHumanQty(t *testing.T) {
	for in, want := range map[[2]string]string{
		{"1200", "g"}: "1.2 kg", {"500", "g"}: "500 g", {"1500", "ml"}: "1.5 L", {"1", "pieza"}: "1 pza",
		{"12", "pieza"}: "12 pzas", {"0.333333", "g"}: "0.33 g", {"250", "cm"}: "2.5 m",
	} {
		if got := HumanQty(d(in[0]), in[1]); got != want {
			t.Errorf("%v = %q, quiere %q", in, got, want)
		}
	}
}
