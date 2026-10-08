package domain

import "fmt"

// Lista de recetas (pantalla Catálogo › Recetas): productos, extras y preparados con su estado.

// RecipeKind es qué lista se pide.
type RecipeKind string

const (
	RecipeKindProduct RecipeKind = "product"
	RecipeKindExtra   RecipeKind = "extra"
	RecipeKindPrep    RecipeKind = "prep"
)

// RecipeStatus es el estado de una receta tal como lo ve quien la revisa.
type RecipeStatus string

const (
	RecipeStatusPending RecipeStatus = "pending"
	RecipeStatusReview  RecipeStatus = "review"
	RecipeStatusDone    RecipeStatus = "done"
)

// RecipeSort es el orden de la lista: lo más vendido primero, que es por donde conviene empezar, o
// alfabético.
type RecipeSort string

const (
	RecipeSortSales RecipeSort = "sales"
	RecipeSortAZ    RecipeSort = "az"
)

// MaxRecipePage acota la página: la pantalla pide de 25 en 25.
const MaxRecipePage = 100

// RecipeFilter es lo que la pantalla pide.
type RecipeFilter struct {
	Kind     RecipeKind
	Status   RecipeStatus
	Sort     RecipeSort
	Query    string
	Category int64 // 0 = todas; en extras es el grupo
	Limit    int
	Offset   int
}

// Validate rechaza todo valor desconocido en lugar de caer a uno por omisión.
func (f RecipeFilter) Validate() error {
	switch f.Kind {
	case RecipeKindProduct, RecipeKindExtra, RecipeKindPrep:
	default:
		return fmt.Errorf("%w: lista de recetas desconocida", ErrValidation)
	}
	switch f.Status {
	case RecipeStatusPending, RecipeStatusReview, RecipeStatusDone:
	default:
		return fmt.Errorf("%w: estado de receta desconocido", ErrValidation)
	}
	switch f.Sort {
	case RecipeSortSales, RecipeSortAZ:
	default:
		return fmt.Errorf("%w: orden desconocido", ErrValidation)
	}
	if f.Limit < 1 || f.Limit > MaxRecipePage || f.Offset < 0 || f.Category < 0 || len(f.Query) > 200 {
		return fmt.Errorf("%w: página inválida", ErrValidation)
	}
	return nil
}
