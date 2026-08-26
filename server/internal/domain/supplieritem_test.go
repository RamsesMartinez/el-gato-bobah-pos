package domain

import "testing"

func TestNormalizeItemName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"minúsculas", "AGUA MINERA", "agua minera"},
		{"acentos fuera", "Piña Colada", "pina colada"},
		{"diéresis y eñe", "Ensueño argán", "ensueno argan"},
		{"espacios colapsados", "  PAN   DE    MUERTO  ", "pan de muerto"},
		{"signos fuera", "Smucker's (cereza)!", "smuckers cereza"},
		{"conserva dígitos: distinguen presentación", "CATSUP200/8G", "catsup200 8g"},
		{"conserva el punto decimal", "3.8 CATSUP", "3.8 catsup"},
		{"guiones y slashes son separadores", "6/186GR CHAM", "6 186gr cham"},
		{"vacío", "", ""},
		{"solo signos", "--- ***", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeItemName(tt.in); got != tt.want {
				t.Errorf("got %q, quiero %q", got, tt.want)
			}
		})
	}
}

// El punto del normalizador: dos proveedores escriben el mismo producto distinto y la llave
// debe converger.
func TestNormalizeItemNameConvergeEntreProveedores(t *testing.T) {
	pairs := [][2]string{
		{"COCA COLA 600ML", "Coca Cola 600ML"},
		{"Suavizante Ensueño 5.1 l", "SUAVIZANTE ENSUENO 5.1 L"},
		{"mermelada  de   cereza", "Mermelada de Cereza"},
	}
	for _, p := range pairs {
		if a, b := NormalizeItemName(p[0]), NormalizeItemName(p[1]); a != b {
			t.Errorf("%q y %q deben dar la misma llave; tengo %q vs %q", p[0], p[1], a, b)
		}
	}
}

func TestSupplierItemKey(t *testing.T) {
	// Con código útil, el código manda: sobrevive a que el proveedor cambie el texto impreso.
	if got := SupplierItemKey("242883", "AGUA MINERA"); got != "242883" {
		t.Errorf("got %q, quiero el código", got)
	}
	// Sin código (un pedido web), la llave es el nombre normalizado.
	if got := SupplierItemKey("", "Harina para pastel Great Value vainilla 432 g"); got != "harina para pastel great value vainilla 432 g" {
		t.Errorf("got %q", got)
	}
	// Un código que quedó en blanco tras descartarlo por ambiguo cae al nombre.
	if got := SupplierItemKey("   ", "PAN NOVIAS 1 PZA"); got != "pan novias 1 pza" {
		t.Errorf("got %q", got)
	}
}

func TestValidSupplierItemStatus(t *testing.T) {
	for _, s := range []string{SupplierItemPendiente, SupplierItemMapeado, SupplierItemIgnorado} {
		if !ValidSupplierItemStatus(s) {
			t.Errorf("%q debe ser válido", s)
		}
	}
	for _, s := range []string{"", "MAPEADO", "borrado", "pending"} {
		if ValidSupplierItemStatus(s) {
			t.Errorf("%q no debe ser válido", s)
		}
	}
}
