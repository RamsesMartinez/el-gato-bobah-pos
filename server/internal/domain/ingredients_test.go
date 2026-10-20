package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateArticle(t *testing.T) {
	neg := dec("-1")
	big := dec("20000000")
	ok := dec("12.5")
	cases := []struct {
		name string
		in   ArticleInput
		bad  bool
	}{
		{"nombre normal", ArticleInput{Name: "Leche entera"}, false},
		{"nombre con espacios de más se limpia y vale", ArticleInput{Name: "  Leche   entera "}, false},
		{"nombre vacío", ArticleInput{Name: "   "}, true},
		{"nombre de 61 letras", ArticleInput{Name: strings.Repeat("a", 61)}, true},
		{"nombre de 60 letras con acentos", ArticleInput{Name: strings.Repeat("á", 60)}, false},
		{"mínimo negativo", ArticleInput{Name: "x", MinStock: &neg}, true},
		{"mínimo válido", ArticleInput{Name: "x", MinStock: &ok}, false},
		{"costo negativo", ArticleInput{Name: "x", Cost: &neg}, true},
		{"costo absurdo", ArticleInput{Name: "x", Cost: &big}, true},
		{"existencia inicial negativa", ArticleInput{Name: "x", InitialStock: &neg}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateArticle(c.in)
			if c.bad != (err != nil) {
				t.Fatalf("ValidateArticle(%+v) = %v; ¿debía fallar? %v", c.in, err, c.bad)
			}
			if err != nil && !errors.Is(err, ErrValidation) {
				t.Fatalf("un dato inválido debe ser 400 (ErrValidation), fue %v", err)
			}
		})
	}
}

// Solo los ajustes dejan cambiar la unidad en el mismo artículo (decisión del dueño, 2026-10-19):
// una compra, una venta o una merma ya se registraron en la unidad vieja y no se reescriben.
func TestUnitChangePath(t *testing.T) {
	cases := []struct {
		name  string
		types []string
		want  UnitPath
	}{
		{"sin movimientos", nil, UnitPathDirect},
		{"solo ajustes de conteo y existencia inicial", []string{"ajuste", "ajuste"}, UnitPathDirect},
		{"una compra obliga al reemplazo", []string{"ajuste", "compra"}, UnitPathReplace},
		{"una venta obliga al reemplazo", []string{"venta"}, UnitPathReplace},
		{"una merma obliga al reemplazo", []string{"merma"}, UnitPathReplace},
		{"una producción obliga al reemplazo", []string{"produccion"}, UnitPathReplace},
		{"una cancelación obliga al reemplazo", []string{"cancelacion"}, UnitPathReplace},
		{"un tipo desconocido no se da por ajuste", []string{"otro"}, UnitPathReplace},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := UnitChangePath(c.types); got != c.want {
				t.Fatalf("UnitChangePath(%v) = %s, quería %s", c.types, got, c.want)
			}
		})
	}
}

func TestValidUnitChange(t *testing.T) {
	cases := []struct {
		from, to string
		factor   string
		bad      bool
	}{
		{"pieza", "masa", "250", false},
		{"masa", "pieza", "0.004", false},
		{"volumen", "pieza", "0.001", false},
		{"longitud", "pieza", "0.01", false},
		{"masa", "masa", "1", true},    // entre g y kg no hay cambio de base (E10)
		{"masa", "volumen", "1", true}, // sin densidad no se convierte
		{"volumen", "masa", "1", true},
		{"pieza", "masa", "0", true},
		{"pieza", "masa", "-3", true},
		{"pieza", "masa", "2000000", true}, // más que el tope de una cantidad de stock
	}
	for _, c := range cases {
		err := ValidUnitChange(c.from, c.to, dec(c.factor))
		if c.bad != (err != nil) {
			t.Fatalf("ValidUnitChange(%s→%s, %s) = %v; ¿debía fallar? %v", c.from, c.to, c.factor, err, c.bad)
		}
		if err != nil && !errors.Is(err, ErrValidation) {
			t.Fatalf("debe ser ErrValidation: %v", err)
		}
	}
}

func TestConvertQty(t *testing.T) {
	if got := ConvertQty(dec("27000"), dec("0.0002")); !got.Equal(dec("5.4")) {
		t.Fatalf("27,000 g × 0.0002 = %s, quería 5.4 piezas", got)
	}
	// Redondea a 4 decimales, como la columna: 100 g con 1 pz = 150 g.
	if got := ConvertQty(dec("100"), dec("1").Div(dec("150"))); !got.Equal(dec("0.6667")) {
		t.Fatalf("100 / 150 = %s, quería 0.6667", got)
	}
	// El costo va al revés: $0.006 por g con 1 pz = 5,000 g es $30 por pieza.
	if got := ConvertCost(dec("0.006"), dec("0.0002")); !got.Equal(dec("30")) {
		t.Fatalf("costo convertido = %s, quería 30", got)
	}
}

func TestTrendPct(t *testing.T) {
	if got := TrendPct(dec("0.022"), dec("0.02")); got == nil || !got.Equal(dec("10")) {
		t.Fatalf("0.022 contra 0.02 = %v, quería 10", got)
	}
	if got := TrendPct(dec("0.018"), dec("0.02")); got == nil || !got.Equal(dec("-10")) {
		t.Fatalf("0.018 contra 0.02 = %v, quería -10", got)
	}
	if got := TrendPct(dec("1"), dec("0")); got != nil {
		t.Fatalf("sin referencia = %v, quería nil", got)
	}
}

// Una receta guarda la cantidad en su propia unidad (2 cucharadas, 0.5 kg): se pasa a la base vieja y
// de ahí a la nueva. Un empaque en piezas trae piezas enteras: el que no cuadra se archiva.
func TestConvertRecipeAndPresentation(t *testing.T) {
	if got := ConvertRecipeQty(dec("0.5"), dec("1000"), dec("0.004")); !got.Equal(dec("2")) {
		t.Fatalf("0.5 kg con 1 pz = 250 g = %s, quería 2 piezas", got)
	}
	if !PresentationFits("pieza", dec("3")) || PresentationFits("pieza", dec("2.5")) || !PresentationFits("masa", dec("2.5")) {
		t.Fatal("un empaque en piezas trae piezas enteras; en peso, cualquier cantidad mayor que cero")
	}
	if PresentationFits("masa", dec("0")) {
		t.Fatal("un empaque que se convierte en cero no cuadra")
	}
}
