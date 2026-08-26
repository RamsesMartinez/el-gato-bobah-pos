package domain

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func decPtr(s string) *decimal.Decimal {
	v := dec(s)
	return &v
}

// Los tres casos vienen de los documentos reales: peso a granel, paquete por piezas y conteo.
func TestBaseQty(t *testing.T) {
	tests := []struct {
		name string
		qty  string
		unit PurchaseUnit
		want string
		err  error
	}{
		{
			// Soriana: 0.280 kg de aguacate hass → 280 g en almacén.
			name: "kg a gramos",
			qty:  "0.280",
			unit: PurchaseUnit{Kind: "masa", ToBase: dec("1000"), BaseKind: "masa"},
			want: "280",
		},
		{
			// Walmart: 2 piezas de "Harina para pastel … 432 g" → 864 g.
			name: "piezas de un formato conocido",
			qty:  "2",
			unit: PurchaseUnit{Kind: "pieza", ToBase: dec("1"), BaseKind: "masa", PackQtyInBase: decPtr("432")},
			want: "864",
		},
		{
			// Walmart: 3 lechugas italianas por pieza; el artículo también se lleva por pieza.
			name: "pieza a pieza",
			qty:  "3",
			unit: PurchaseUnit{Kind: "pieza", ToBase: dec("1"), BaseKind: "pieza"},
			want: "3",
		},
		{
			// Suavizante 5.1 l → 5100 ml.
			name: "litros a mililitros",
			qty:  "5.1",
			unit: PurchaseUnit{Kind: "volumen", ToBase: dec("1000"), BaseKind: "volumen"},
			want: "5100",
		},
		{
			name: "misma unidad base no multiplica",
			qty:  "250",
			unit: PurchaseUnit{Kind: "masa", ToBase: dec("1"), BaseKind: "masa"},
			want: "250",
		},
		{
			// Sam's "K. DELICE 15": se compra por pieza pero el ingrediente se lleva en gramos y
			// nadie sabe cuánto pesa una. La línea NO puede tocar stock hasta registrar el formato.
			name: "piezas sin formato conocido",
			qty:  "1",
			unit: PurchaseUnit{Kind: "pieza", ToBase: dec("1"), BaseKind: "masa"},
			err:  ErrPackUnknown,
		},
		{
			name: "masa contra volumen exige densidad",
			qty:  "1",
			unit: PurchaseUnit{Kind: "masa", ToBase: dec("1000"), BaseKind: "volumen"},
			err:  ErrUnitKindMismatch,
		},
		{
			name: "cantidad cero",
			qty:  "0",
			unit: PurchaseUnit{Kind: "masa", ToBase: dec("1000"), BaseKind: "masa"},
			err:  ErrValidation,
		},
		{
			name: "cantidad negativa",
			qty:  "-2",
			unit: PurchaseUnit{Kind: "pieza", ToBase: dec("1"), BaseKind: "pieza"},
			err:  ErrValidation,
		},
		{
			name: "formato en cero se trata como desconocido",
			qty:  "2",
			unit: PurchaseUnit{Kind: "pieza", ToBase: dec("1"), BaseKind: "masa", PackQtyInBase: decPtr("0")},
			err:  ErrPackUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BaseQty(dec(tt.qty), tt.unit)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("err = %v, quiero %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err inesperado: %v", err)
			}
			if want := dec(tt.want); !got.Equal(want) {
				t.Errorf("got = %s, quiero %s", got, want)
			}
		})
	}
}

// Los errores de conversión deben llegar como 400/422, no como 500: son datos que faltan o
// unidades mal elegidas, no fallas del servidor.
func TestBaseQtyErroresSonDeValidacion(t *testing.T) {
	for _, err := range []error{ErrPackUnknown, ErrUnitKindMismatch} {
		if !errors.Is(err, ErrValidation) {
			t.Errorf("%v debe envolver ErrValidation para mapear a 4xx", err)
		}
	}
}

func TestUnitCost(t *testing.T) {
	tests := []struct {
		name            string
		amount, baseQty string
		want            string
	}{
		// Un kilo de fresa a 179.02 → 0.17902 por gramo. Redondear a centavos lo haría 0.18/g,
		// es decir $180 el kilo: por eso el costo unitario lleva 6 decimales.
		{"por gramo", "179.02", "1000", "0.17902"},
		{"por pieza", "24.00", "3", "8"},
		{"sin base no divide entre cero", "100.00", "0", "0"},
		{"base negativa tampoco", "100.00", "-5", "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UnitCost(dec(tt.amount), dec(tt.baseQty))
			if want := dec(tt.want); !got.Equal(want) {
				t.Errorf("got = %s, quiero %s", got, want)
			}
		})
	}
}
