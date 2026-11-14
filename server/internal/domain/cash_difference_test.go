package domain

import "testing"

// La diferencia de un corte se calcula con una sola regla para la lista, el detalle y el cierre
// (#290): en noviembre de 2026 la lista decía −$120 y el diálogo «Caja cerrada» decía $0 del mismo
// corte. Cubre las tres épocas de cortes que viven en producción.
func TestCutDifference(t *testing.T) {
	cases := []struct {
		name      string
		methods   []MethodDiff
		drawer    *DrawerCount
		wantDraw  *string
		wantOther string
		wantTotal string
		covered   bool
	}{
		{
			name: "desde la 0067: el cajón corto vive en el arqueo y el efectivo de la conciliación es cero",
			methods: []MethodDiff{
				{Expected: d("4120"), Declared: d("4120"), TouchesDrawer: true},
				{Expected: d("2480"), Declared: d("2480")},
			},
			drawer:   &DrawerCount{Expected: dp("3330"), Counted: d("3210")},
			wantDraw: strp("-120"), wantOther: "0", wantTotal: "-120", covered: true,
		},
		{
			name: "otro método con diferencia suma a Otros, no al cajón",
			methods: []MethodDiff{
				{Expected: d("100"), Declared: d("100"), TouchesDrawer: true},
				{Expected: d("2480"), Declared: d("2468")},
			},
			drawer:   &DrawerCount{Expected: dp("500"), Counted: d("535.50")},
			wantDraw: strp("35.5"), wantOther: "-12", wantTotal: "23.5", covered: true,
		},
		{
			name: "entre la 0066 y la 0067: conteo sin esperado, el efectivo declarado cuenta en la conciliación",
			methods: []MethodDiff{
				{Expected: d("1000"), Declared: d("980"), TouchesDrawer: true},
			},
			drawer:   &DrawerCount{Expected: nil, Counted: d("980")},
			wantDraw: nil, wantOther: "-20", wantTotal: "-20", covered: false,
		},
		{
			name: "antes de la 0066: sin conteo, todo es conciliación",
			methods: []MethodDiff{
				{Expected: d("1000"), Declared: d("1010"), TouchesDrawer: true},
				{Expected: d("300"), Declared: d("300")},
			},
			drawer:   nil,
			wantDraw: nil, wantOther: "10", wantTotal: "10", covered: false,
		},
		{
			name:     "redondea a centavos",
			methods:  []MethodDiff{{Expected: d("10.004"), Declared: d("10.001")}},
			drawer:   &DrawerCount{Expected: dp("100.005"), Counted: d("100")},
			wantDraw: strp("-0.01"), wantOther: "0", wantTotal: "-0.01", covered: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CutDifference(c.methods, c.drawer)
			if (got.Drawer == nil) != (c.wantDraw == nil) || (got.Drawer != nil && !got.Drawer.Equal(d(*c.wantDraw))) {
				t.Errorf("Drawer = %v; quería %v", got.Drawer, c.wantDraw)
			}
			if !got.OtherMethods.Equal(d(c.wantOther)) {
				t.Errorf("OtherMethods = %s; quería %s", got.OtherMethods, c.wantOther)
			}
			if !got.Total.Equal(d(c.wantTotal)) {
				t.Errorf("Total = %s; quería %s", got.Total, c.wantTotal)
			}
			if got.DrawerCovered != c.covered {
				t.Errorf("DrawerCovered = %v; quería %v", got.DrawerCovered, c.covered)
			}
		})
	}
}

// Si un método del cajón trae una diferencia en la conciliación Y el corte tiene arqueo, esa
// diferencia ya está dentro del arqueo: sumar las dos reporta el faltante dos veces. Es justo lo que
// hacía la suma de ListSessions (todas las diferencias por método + la del conteo).
func TestElEfectivoNoSeCuentaDosVecesEnLaDiferencia(t *testing.T) {
	got := CutDifference(
		[]MethodDiff{{Expected: d("3330"), Declared: d("3210"), TouchesDrawer: true}},
		&DrawerCount{Expected: dp("3330"), Counted: d("3210")},
	)
	if !got.Total.Equal(d("-120")) {
		t.Fatalf("el faltante del cajón se contó dos veces: Total = %s; el cajón solo faltó −120 "+
			"(una vez en el arqueo y otra en el renglón de efectivo de la conciliación)", got.Total)
	}
}

func strp(s string) *string { return &s }
