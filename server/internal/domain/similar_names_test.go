package domain

import (
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

func TestSimilarNames(t *testing.T) {
	cands := []NamedItem{
		{ID: 1, Name: "Leche entera"},
		{ID: 2, Name: "Azúcar estándar"},
		{ID: 3, Name: "Vaso 16 oz"},
		{ID: 4, Name: "Té"},
		{ID: 5, Name: "Tapa 16 oz"},
	}
	cases := []struct {
		name string
		in   string
		want []int64
	}{
		{"la marca agregada al nombre", "leche entera lala 1L", []int64{1}},
		{"una palabra suelta no basta: «Queso» está en el inicio de quesos que no son lo mismo", "Leche", nil},
		{"el nuevo es el inicio del existente, con dos palabras o más", "Vaso 16", []int64{3}},
		{"contenido en medio no cuenta: «Frutos rojos» está en concentrados y tés distintos", "Concentrado leche entera", nil},
		{"dos letras invertidas", "Lehce entera", []int64{1}},
		{"una letra cambiada", "Leche entara", []int64{1}},
		{"acentos, mayúsculas y signos no cuentan", "AZUCAR, estandar.", []int64{2}},
		{"con paréntesis", "Vaso 16 oz (caja)", []int64{3}},
		{"nada que ver", "Aceite de oliva", nil},
		{"un nombre corto no arrastra a todos los que lo contienen", "Té verde", nil},
		{"medidas distintas no son una letra de diferencia cuando el nombre es corto", "Vaso 12 oz", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := []int64{}
			for _, m := range SimilarNames(c.in, cands, 5) {
				got = append(got, m.ID)
			}
			if len(c.want) == 0 && len(got) == 0 {
				return
			}
			if !slices.Equal(got, c.want) {
				t.Fatalf("SimilarNames(%q) = %v; quiere %v", c.in, got, c.want)
			}
		})
	}
	t.Run("hasta el tope, los más parecidos primero", func(t *testing.T) {
		many := []NamedItem{{ID: 1, Name: "Leche entera lala"}, {ID: 2, Name: "Leche entera"}, {ID: 3, Name: "Leche entera alpura"}}
		got := SimilarNames("Leche entera", many[:1], 1)
		if len(got) != 1 {
			t.Fatalf("%v", got)
		}
		got = SimilarNames("Leche entara", many, 5)
		if len(got) == 0 || got[0].ID != 2 {
			t.Fatalf("el de una letra de diferencia va primero: %v", got)
		}
	})
}

func TestDuplicateGroups(t *testing.T) {
	items := []NamedItem{
		{ID: 7, Name: "Queso"}, {ID: 8, Name: "Queso panela"}, {ID: 9, Name: "Queso manchego"},
		{ID: 1, Name: "Leche entera"}, {ID: 2, Name: "Leche entera Marca B"}, {ID: 3, Name: "Leche entera marca B 1L"},
		{ID: 4, Name: "Vaso 16 oz"}, {ID: 5, Name: "Vaso 16 oz (caja)"},
		{ID: 6, Name: "Azúcar"},
	}
	got := dupGroups(items, nil)
	// «Queso» no junta a todos los quesos: una palabra suelta no basta.
	if len(got) != 2 || !slices.Equal(ids(got[0]), []int64{1, 2, 3}) || !slices.Equal(ids(got[1]), []int64{4, 5}) {
		t.Fatalf("grupos = %v; quiere {1,2,3} por transitividad y {4,5}", got)
	}
	t.Run("un par descartado parte el grupo", func(t *testing.T) {
		got := dupGroups(items, []DistinctPair{{Low: 4, High: 5}})
		if len(got) != 1 {
			t.Fatalf("grupos = %v; el de vasos ya no debe salir", got)
		}
	})
	t.Run("descartar un par de un grupo de tres deja los otros", func(t *testing.T) {
		got := dupGroups(items[3:6], []DistinctPair{{Low: 1, High: 2}})
		if len(got) != 1 || len(got[0]) != 3 {
			t.Fatalf("grupos = %v; 1~3 y 2~3 siguen juntando a los tres", got)
		}
	})
	if got := dupGroups(nil, nil); got == nil {
		t.Fatal("sin insumos es una lista vacía, no nil: el JSON prometió un arreglo")
	}
}

func ids(g []NamedItem) []int64 {
	out := []int64{}
	for _, it := range g {
		out = append(out, it.ID)
	}
	return out
}

// Un catálogo grande no puede costar segundos de CPU por cada vez que se abre la hoja: la VM de
// producción tiene un núcleo y atiende a todas las empresas (revisión de seguridad, M2). Medido antes
// del arreglo: 2,000 nombres de 60 letras tardaban 24 s.
func TestDuplicateGroupsStaysCheapOnABigCatalog(t *testing.T) {
	if testing.Short() {
		t.Skip("mide tiempo")
	}
	rng := rand.New(rand.NewPCG(1, 2))
	letters := []rune("abcdefghijklmnopqrstuvwxyz ")
	items := make([]NamedItem, 2000)
	for i := range items {
		name := make([]rune, 60)
		for j := range name {
			name[j] = letters[rng.IntN(len(letters))]
		}
		items[i] = NamedItem{ID: int64(i + 1), Name: string(name)}
	}
	start := time.Now()
	if _, ok := DuplicateGroups(items, nil, func() bool { return false }); !ok {
		t.Fatal("se cortó sin que nadie lo pidiera")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("2,000 nombres de 60 letras tardaron %s; el presupuesto es 2 s", d)
	}
}

// Si el request se cancela, la búsqueda se detiene en vez de seguir quemando CPU.
func TestDuplicateGroupsStopsWhenAsked(t *testing.T) {
	items := []NamedItem{{ID: 1, Name: "Leche entera"}, {ID: 2, Name: "Leche entera lala"}}
	if _, ok := DuplicateGroups(items, nil, func() bool { return true }); ok {
		t.Fatal("no se detuvo")
	}
}

// dupGroups es DuplicateGroups sin corte, para las pruebas de la regla.
func dupGroups(items []NamedItem, distinct []DistinctPair) [][]NamedItem {
	g, _ := DuplicateGroups(items, distinct, func() bool { return false })
	return g
}
