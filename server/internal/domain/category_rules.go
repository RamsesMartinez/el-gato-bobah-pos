package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Reglas del árbol de categorías del menú (spec 099). La pantalla de venta pinta dos niveles —las
// raíces y las subcategorías de la raíz elegida—, así que una categoría de tercer nivel o una activa
// colgada de un padre apagado desaparece de la venta sin un solo error. Con «el padre es raíz» y
// «quien tiene subcategorías no recibe padre» tampoco hay ciclos posibles.

var (
	// ErrCategoryNameTaken: ya hay una categoría con ese nombre bajo el mismo padre.
	ErrCategoryNameTaken = fmt.Errorf("%w: ya hay una categoría con ese nombre", ErrConflict)
	// ErrCategoryHasActiveProducts: apagarla dejaría productos activos fuera de la venta.
	ErrCategoryHasActiveProducts = fmt.Errorf("%w: la categoría tiene productos activos", ErrConflict)
	// ErrCategoryHasActiveChildren: apagarla dejaría subcategorías activas colgando de nada.
	ErrCategoryHasActiveChildren = fmt.Errorf("%w: la categoría tiene subcategorías activas", ErrConflict)
	// ErrCategoryParentInvalid: el padre pedido no es una raíz activa, o la categoría no puede tener padre.
	ErrCategoryParentInvalid = errors.New("padre de categoría inválido")
)

// MaxCategoryNameLen es el tope en letras del nombre de una categoría: el riel de la venta lo
// recorta mucho antes, pero sin tope el agente puede guardar un párrafo.
const MaxCategoryNameLen = 60

// CategoryNode es una categoría tal como está guardada.
type CategoryNode struct {
	ID       int64
	Name     string
	ParentID *int64
	Active   bool
}

// CategoryFacts es lo que el servicio leyó (con candado) para decidir: la categoría que se edita
// (nil en un alta), el padre que tendría después (nil si queda raíz), y lo que cuelga de ella.
type CategoryFacts struct {
	Target         *CategoryNode
	Parent         *CategoryNode
	HasChildren    bool
	ActiveChildren []string
	ActiveProducts int
}

// CategoryNext es cómo quedaría la categoría.
type CategoryNext struct {
	Name     string
	ParentID *int64
	Active   bool
}

// NormalizeCategoryName quita los espacios de los extremos y valida el largo en letras.
func NormalizeCategoryName(name string) (string, error) {
	n := strings.TrimSpace(name)
	if n == "" {
		return "", fmt.Errorf("%w: la categoría necesita nombre", ErrValidation)
	}
	if utf8.RuneCountInString(n) > MaxCategoryNameLen {
		return "", fmt.Errorf("%w: el nombre de la categoría lleva a lo más %d letras", ErrValidation, MaxCategoryNameLen)
	}
	return n, nil
}

// CheckCategoryChange decide si una categoría puede quedar como next. El nombre debe venir ya
// normalizado o se normaliza aquí; la unicidad la revisa el servicio contra la base.
func CheckCategoryChange(f CategoryFacts, next CategoryNext) error {
	if _, err := NormalizeCategoryName(next.Name); err != nil {
		return err
	}
	if next.ParentID != nil {
		if f.Parent == nil || f.Parent.ID != *next.ParentID {
			return fmt.Errorf("%w: no existe la categoría %d", ErrNotFound, *next.ParentID)
		}
		// Las reglas del padre aplican cuando el padre cambia (o en un alta): un árbol heredado de antes
		// de ellas —un tercer nivel, una subcategoría con hijas— no bloquea renombrarla ni apagarla.
		parentChanges := f.Target == nil || !sameParent(f.Target.ParentID, next.ParentID)
		if parentChanges {
			if f.Target != nil && f.Target.ID == *next.ParentID {
				return fmt.Errorf("%w: una categoría no puede ser su propio padre", ErrCategoryParentInvalid)
			}
			if f.Parent.ParentID != nil {
				return fmt.Errorf("%w: «%s» ya es subcategoría; el padre debe ser una categoría raíz",
					ErrCategoryParentInvalid, f.Parent.Name)
			}
			if f.Target != nil && f.HasChildren {
				return fmt.Errorf("%w: «%s» tiene subcategorías y no puede quedar dentro de otra",
					ErrCategoryParentInvalid, f.Target.Name)
			}
		}
		// Igual con el padre apagado: solo cuando la categoría pasa a estar activa bajo él (alta,
		// reactivar o mover).
		becomesVisible := parentChanges || !f.Target.Active
		if next.Active && !f.Parent.Active && becomesVisible {
			return fmt.Errorf("%w: «%s» está inactiva; actívala primero", ErrCategoryParentInvalid, f.Parent.Name)
		}
	}
	if f.Target != nil && f.Target.Active && !next.Active {
		if f.ActiveProducts > 0 {
			return fmt.Errorf("%w: «%s» tiene %d productos activos", ErrCategoryHasActiveProducts, f.Target.Name, f.ActiveProducts)
		}
		if len(f.ActiveChildren) > 0 {
			return fmt.Errorf("%w: «%s» tiene subcategorías activas: %s", ErrCategoryHasActiveChildren,
				f.Target.Name, strings.Join(f.ActiveChildren, ", "))
		}
	}
	return nil
}

func sameParent(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
