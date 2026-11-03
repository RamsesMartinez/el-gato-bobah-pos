package domain

import (
	"errors"
	"testing"
)

// Un filtro de la lista de recetas mal escrito se rechaza: volverlo un valor por omisión mostraría
// una lista que parece la pedida y no lo es (constitución V).
func TestRecipeFilterValidate(t *testing.T) {
	ok := RecipeFilter{Kind: RecipeKindProduct, Status: RecipeStatusPending, Sort: RecipeSortSales, Limit: 25}
	if err := ok.Validate(); err != nil {
		t.Fatalf("un filtro bueno: %v", err)
	}
	cases := map[string]RecipeFilter{
		"tipo desconocido":   {Kind: "combo", Status: RecipeStatusPending, Sort: RecipeSortSales, Limit: 25},
		"estado desconocido": {Kind: RecipeKindExtra, Status: "listo", Sort: RecipeSortSales, Limit: 25},
		"orden desconocido":  {Kind: RecipeKindExtra, Status: RecipeStatusDone, Sort: "precio", Limit: 25},
		"sin tamaño":         {Kind: RecipeKindPrep, Status: RecipeStatusDone, Sort: RecipeSortAZ, Limit: 0},
		"tamaño absurdo":     {Kind: RecipeKindPrep, Status: RecipeStatusDone, Sort: RecipeSortAZ, Limit: 1000},
		"desde negativo":     {Kind: RecipeKindPrep, Status: RecipeStatusDone, Sort: RecipeSortAZ, Limit: 25, Offset: -1},
		// Más allá de int32 llegaba a Postgres como negativo (500) o como 0 (la primera página).
		"desde desbordado": {Kind: RecipeKindPrep, Status: RecipeStatusDone, Sort: RecipeSortAZ, Limit: 25, Offset: 1 << 32},
		// Un NUL llega intacto desde la URL y Postgres lo rechaza: salía como 500.
		"búsqueda con NUL": {Kind: RecipeKindProduct, Status: RecipeStatusDone, Sort: RecipeSortAZ, Limit: 25, Query: "ca\x00fe"},
		"búsqueda enorme":  {Kind: RecipeKindProduct, Status: RecipeStatusDone, Sort: RecipeSortAZ, Limit: 25, Query: string(make([]byte, 201))},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			if err := f.Validate(); !errors.Is(err, ErrValidation) {
				t.Fatalf("debe rechazarse: %v", err)
			}
		})
	}
}

// Confirmar de una vez acepta la página que se ve, no una lista arbitraria.
func TestValidConfirmIDs(t *testing.T) {
	if err := ValidConfirmIDs([]int64{1, 2}); err != nil {
		t.Fatalf("dos recetas: %v", err)
	}
	cases := map[string][]int64{
		"ninguna":      {},
		"más de 500":   make([]int64, MaxRecipeConfirm+1),
		"id no válido": {0},
	}
	for name, ids := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidConfirmIDs(ids); !errors.Is(err, ErrValidation) {
				t.Fatalf("debe rechazarse: %v", err)
			}
		})
	}
}

// Lo que un extra lleva hoy, en una línea, para ofrecer copiarle otra receta sin esconderlo.
func TestRecipeToday(t *testing.T) {
	cases := map[string]struct {
		status, linked string
		items          []string
		want           string
	}{
		"sin capturar":      {status: "", want: "Pendiente"},
		"no gasta insumos":  {status: "confirmed", want: "No gasta insumos"},
		"es otro producto":  {status: "confirmed", linked: "Coca-Cola", want: "Es: Coca-Cola"},
		"con insumos":       {status: "estimated", items: []string{"5 g Polvo"}, want: "5 g Polvo"},
		"corta en tres":     {status: "confirmed", items: []string{"a", "b", "c", "d", "e"}, want: "a · b · c · +2 más"},
		"estimada sin nada": {status: "estimated", want: "Pendiente"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := RecipeToday(c.status, c.linked, c.items); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}
