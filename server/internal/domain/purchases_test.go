package domain

import (
	"errors"
	"testing"
)

// Longitud (spec 038) usa el mismo camino que peso y volumen: mismo tipo multiplica, otro tipo se
// rechaza; no se adivina una conversión entre metros y kilos.
func TestBaseQtyWithLength(t *testing.T) {
	tests := []struct {
		name string
		qty  string
		unit PurchaseUnit
		want string
		err  error
	}{
		{"metros a centímetros", "2.5", PurchaseUnit{Kind: "longitud", ToBase: dec("100"), BaseKind: "longitud"}, "250", nil},
		{"centímetros tal cual", "35", PurchaseUnit{Kind: "longitud", ToBase: dec("1"), BaseKind: "longitud"}, "35", nil},
		{"metros de algo que se pesa", "1", PurchaseUnit{Kind: "longitud", ToBase: dec("100"), BaseKind: "masa"}, "", ErrUnitKindMismatch},
		{"rollo de 100 m por pieza", "2", PurchaseUnit{Kind: "pieza", ToBase: dec("1"), BaseKind: "longitud", PackQtyInBase: decPtr("10000")}, "20000", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BaseQty(dec(tt.qty), tt.unit)
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, quiere %v", err, tt.err)
			}
			if tt.err == nil && !got.Equal(dec(tt.want)) {
				t.Errorf("BaseQty = %s, quiere %s", got, tt.want)
			}
		})
	}
}

// Un empaque se captura con número Y unidad: «25» a secas en un insumo en gramos sería un error de
// 1,000 veces que nadie ve (caso borde 1 del spec). Cada caso nombra la regla que cuida.
func TestPlanPresentation(t *testing.T) {
	units := []string{"g", "Gramo", "kg", "Kilogramo", "ml", "Mililitro", "l", "Litro", "pieza", "Pieza", "cm", "m", "Metro"}
	kg := PurchaseUnit{Kind: "masa", ToBase: dec("1000")}
	g := PurchaseUnit{Kind: "masa", ToBase: dec("1")}
	litro := PurchaseUnit{Kind: "volumen", ToBase: dec("1000")}
	pieza := PurchaseUnit{Kind: "pieza", ToBase: dec("1")}
	tests := []struct {
		name   string
		in     PresentationInput
		want   string
		approx bool
		err    bool
	}{
		{"bulto de 25 kg en un insumo en gramos (regla 1)", PresentationInput{Name: "Bulto", Qty: dec("25"), Unit: kg, BaseKind: "masa"}, "25000", false, false},
		{"bulto de 25 g es otra cosa y se respeta", PresentationInput{Name: "Bulto", Qty: dec("25"), Unit: g, BaseKind: "masa"}, "25", false, false},
		{"caja de 12 L", PresentationInput{Name: "Caja", Qty: dec("12"), Unit: litro, BaseKind: "volumen"}, "12000", false, false},
		{"paquete de 50 piezas", PresentationInput{Name: "Paquete", Qty: dec("50"), Unit: pieza, BaseKind: "pieza"}, "50", false, false},
		{"piezas con decimales (regla 16)", PresentationInput{Name: "Bolsa", Qty: dec("62.5"), Unit: pieza, BaseKind: "pieza"}, "", false, true},
		{"litros en un insumo que se pesa (regla 3)", PresentationInput{Name: "Bidón", Qty: dec("20"), Unit: litro, BaseKind: "masa"}, "", false, true},
		{"kilos en un insumo por pieza sin declararlo aproximado (regla 2)", PresentationInput{Name: "Bolsa", Qty: dec("1"), Unit: kg, BaseKind: "pieza"}, "", false, true},
		{"aproximado: por kilo ≈ 4 piezas (regla 18)", PresentationInput{Name: "Kilo", Qty: dec("4"), Unit: kg, BaseKind: "pieza", Approximate: true}, "4", true, false},
		{"aproximado del mismo tipo no existe", PresentationInput{Name: "Kilo", Qty: dec("1000"), Unit: kg, BaseKind: "masa", Approximate: true}, "", false, true},
		{"nombre de una unidad (regla 14)", PresentationInput{Name: " litro ", Qty: dec("1"), Unit: litro, BaseKind: "volumen"}, "", false, true},
		{"nombre vacío", PresentationInput{Name: "  ", Qty: dec("1"), Unit: litro, BaseKind: "volumen"}, "", false, true},
		{"nombre de más de 40", PresentationInput{Name: "Caja de cartón corrugado con separadores para botellas", Qty: dec("1"), Unit: litro, BaseKind: "volumen"}, "", false, true},
		{"contenido en cero", PresentationInput{Name: "Caja", Qty: dec("0"), Unit: litro, BaseKind: "volumen"}, "", false, true},
		{"contenido absurdo", PresentationInput{Name: "Caja", Qty: dec("99999999999"), Unit: litro, BaseKind: "volumen"}, "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.UnitNames = units
			got, err := PlanPresentation(tt.in)
			if tt.err {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("err = %v, quiere ErrValidation", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if !got.QtyInBase.Equal(dec(tt.want)) || got.Approximate != tt.approx {
				t.Errorf("PlanPresentation = %s (aprox %v), quiere %s (aprox %v)", got.QtyInBase, got.Approximate, tt.want, tt.approx)
			}
		})
	}
}

// El costo del insumo al comprar (R4). El costo es por unidad base; lo guardado es la suma de las
// sucursales.
func TestNextIngredientCost(t *testing.T) {
	tests := []struct {
		name                        string
		method                      CostingMethod
		onHand, cost, qty, unitCost string
		gift                        bool
		want                        string
	}{
		{"última compra", CostingLast, "10000", "0.020", "10000", "0.030", false, "0.030"},
		{"ponderado: 10 L a $20 y 10 L a $30 dan $25", CostingWeighted, "10000", "0.020", "10000", "0.030", false, "0.025"},
		{"ponderado sin nada guardado toma la compra", CostingWeighted, "0", "0.020", "10000", "0.030", false, "0.030"},
		{"ponderado con un negativo heredado toma la compra (FR-006b)", CostingWeighted, "-5000", "0.020", "10000", "0.030", false, "0.030"},
		{"un regalo no cambia el costo", CostingLast, "10000", "0.020", "10000", "0", true, "0.020"},
		{"un importe en cero no cambia el costo (0 = sin capturar)", CostingWeighted, "10000", "0.020", "10000", "0", false, "0.020"},
		{"ponderado redondea a 6 decimales", CostingWeighted, "3", "1", "7", "2", false, "1.7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextIngredientCost(tt.method, dec(tt.onHand), dec(tt.cost), dec(tt.qty), dec(tt.unitCost), tt.gift)
			if !got.Equal(dec(tt.want)) {
				t.Errorf("NextIngredientCost = %s, quiere %s", got, tt.want)
			}
		})
	}
}

func TestParseCostingMethod(t *testing.T) {
	for _, ok := range []string{"last", "weighted"} {
		if _, err := ParseCostingMethod(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "fifo", "LAST"} {
		if _, err := ParseCostingMethod(bad); !errors.Is(err, ErrValidation) {
			t.Errorf("%q debió rechazarse: %v", bad, err)
		}
	}
}
