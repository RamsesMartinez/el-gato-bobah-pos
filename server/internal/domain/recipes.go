package domain

import (
	"fmt"
	"math"
	"strings"
)

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
	if f.Limit < 1 || f.Limit > MaxRecipePage || f.Offset < 0 || f.Offset > math.MaxInt32 || f.Category < 0 || len(f.Query) > 200 {
		return fmt.Errorf("%w: página inválida", ErrValidation)
	}
	if strings.IndexFunc(f.Query, esDeControl) >= 0 {
		return fmt.Errorf("%w: la búsqueda trae un carácter que no se puede buscar", ErrValidation)
	}
	return nil
}

// MaxRecipeConfirm es el tope de «Confirmar las que se ven»: holgado sobre la página más grande.
const MaxRecipeConfirm = 500

// ValidConfirmIDs rechaza una lista vacía, más larga que el tope o con un id imposible.
func ValidConfirmIDs(ids []int64) error {
	if len(ids) == 0 || len(ids) > MaxRecipeConfirm {
		return fmt.Errorf("%w: lista de recetas inválida", ErrValidation)
	}
	for _, id := range ids {
		if id <= 0 {
			return fmt.Errorf("%w: receta %d inválida", ErrValidation, id)
		}
	}
	return nil
}
