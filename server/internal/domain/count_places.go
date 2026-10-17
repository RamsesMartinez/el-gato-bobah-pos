package domain

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// MaxCountPlaceName es el largo de un lugar para contar, en letras: cabe en el renglón del artículo.
const MaxCountPlaceName = 30

// CleanCountPlaceName es el nombre como se guarda: sin espacios en las orillas ni repetidos adentro.
func CleanCountPlaceName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

// ValidCountPlaceName pide de 1 a 30 letras, contadas después de limpiar los espacios.
func ValidCountPlaceName(name string) error {
	n := utf8.RuneCountInString(CleanCountPlaceName(name))
	if n == 0 || n > MaxCountPlaceName {
		return fmt.Errorf("%w: el lugar lleva de 1 a %d letras", ErrValidation, MaxCountPlaceName)
	}
	return nil
}

// CountPlaceExists dice que ya hay un lugar con ese nombre (sin distinguir mayúsculas, acentos ni
// espacios); lleva su id para que la pantalla lo elija en vez de crear otro. Es un conflicto (409).
type CountPlaceExists struct{ ID int64 }

func (e CountPlaceExists) Error() string { return "ese lugar ya existe" }
func (e CountPlaceExists) Unwrap() error { return ErrConflict }

// CountPlaceHasIngredients dice que el lugar que se quiere archivar tiene artículos y no vino a
// dónde pasarlos: la pantalla pregunta. Es un conflicto (409).
type CountPlaceHasIngredients struct{ Ingredients int32 }

func (e CountPlaceHasIngredients) Error() string {
	return fmt.Sprintf("el lugar tiene %d artículos: elige a qué lugar pasan", e.Ingredients)
}
func (e CountPlaceHasIngredients) Unwrap() error { return ErrConflict }

// MoveCountPlace devuelve el recorrido con el lugar id en la posición to (0 = primero), sin tocar
// el que recibe. to es el ordinal entre los lugares activos: fuera de rango es un error, no el
// extremo más cercano, porque una pantalla que pide una posición que no existe trae otra lista.
func MoveCountPlace(order []int64, id int64, to int) ([]int64, error) {
	from := slices.Index(order, id)
	if from < 0 {
		return nil, fmt.Errorf("%w: el lugar no está en el recorrido", ErrValidation)
	}
	if to < 0 || to >= len(order) {
		return nil, fmt.Errorf("%w: esa posición no existe en el recorrido", ErrValidation)
	}
	out := slices.Delete(slices.Clone(order), from, from+1)
	return slices.Insert(out, to, id), nil
}
