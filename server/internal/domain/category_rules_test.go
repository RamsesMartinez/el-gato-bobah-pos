package domain

import (
	"errors"
	"strings"
	"testing"
)

func catID(v int64) *int64 { return new(v) }

func TestCheckCategoryChange(t *testing.T) {
	root := &CategoryNode{ID: 1, Name: "Bebidas", Active: true}
	inactiveRoot := &CategoryNode{ID: 2, Name: "Desayunos", Active: false}
	sub := &CategoryNode{ID: 3, Name: "Frías", ParentID: catID(1), Active: true}
	lonely := &CategoryNode{ID: 4, Name: "Postres", Active: true}

	cases := []struct {
		name  string
		facts CategoryFacts
		next  CategoryNext
		want  error
		// msg es lo que el mensaje debe decir para que el agente sepa qué corregir.
		msg string
	}{
		{name: "alta de una raíz", next: CategoryNext{Name: "Tés", Active: true}},
		{name: "alta bajo una raíz activa", facts: CategoryFacts{Parent: root},
			next: CategoryNext{Name: "Tés", ParentID: catID(1), Active: true}},
		{name: "nombre vacío no se guarda", next: CategoryNext{Name: "", Active: true}, want: ErrValidation},
		{name: "nombre de puros espacios cuenta como vacío", next: CategoryNext{Name: "   ", Active: true}, want: ErrValidation},
		{name: "nombre de 61 letras se rechaza", next: CategoryNext{Name: strings.Repeat("ñ", 61), Active: true}, want: ErrValidation},
		{name: "60 letras con acentos caben (cuenta letras, no bytes)", next: CategoryNext{Name: strings.Repeat("ñ", 60), Active: true}},
		{name: "un padre que no existe en la empresa", next: CategoryNext{Name: "Tés", ParentID: catID(99), Active: true},
			want: ErrNotFound},
		{name: "bajo una subcategoría haría un tercer nivel que la venta no pinta", facts: CategoryFacts{Parent: sub},
			next: CategoryNext{Name: "Tés", ParentID: catID(3), Active: true}, want: ErrCategoryParentInvalid, msg: "raíz"},
		{name: "alta activa bajo un padre inactivo quedaría oculta", facts: CategoryFacts{Parent: inactiveRoot},
			next: CategoryNext{Name: "Tés", ParentID: catID(2), Active: true}, want: ErrCategoryParentInvalid, msg: "inactiva"},
		{name: "su propio padre", facts: CategoryFacts{Target: lonely, Parent: lonely},
			next: CategoryNext{Name: "Postres", ParentID: catID(4), Active: true}, want: ErrCategoryParentInvalid, msg: "propio"},
		{name: "una raíz con subcategorías no se mueve bajo otra", facts: CategoryFacts{Target: lonely, Parent: root, HasChildren: true},
			next: CategoryNext{Name: "Postres", ParentID: catID(1), Active: true}, want: ErrCategoryParentInvalid, msg: "subcategorías"},
		{name: "mover bajo una raíz activa", facts: CategoryFacts{Target: lonely, Parent: root},
			next: CategoryNext{Name: "Postres", ParentID: catID(1), Active: true}},
		{name: "volver raíz", facts: CategoryFacts{Target: sub},
			next: CategoryNext{Name: "Frías", Active: true}},
		{name: "apagar sin productos ni subcategorías activas", facts: CategoryFacts{Target: lonely},
			next: CategoryNext{Name: "Postres", Active: false}},
		{name: "apagar con productos activos los dejaría fuera de la venta", facts: CategoryFacts{Target: lonely, ActiveProducts: 3},
			next: CategoryNext{Name: "Postres", Active: false}, want: ErrCategoryHasActiveProducts, msg: "3 productos activos"},
		{name: "apagar con subcategorías activas las dejaría huérfanas", facts: CategoryFacts{Target: root, HasChildren: true, ActiveChildren: []string{"Frías", "Calientes"}},
			next: CategoryNext{Name: "Bebidas", Active: false}, want: ErrCategoryHasActiveChildren, msg: "Frías, Calientes"},
		{name: "reactivar bajo un padre inactivo", facts: CategoryFacts{Target: &CategoryNode{ID: 5, Name: "Molletes", ParentID: catID(2)}, Parent: inactiveRoot},
			next: CategoryNext{Name: "Molletes", ParentID: catID(2), Active: true}, want: ErrCategoryParentInvalid, msg: "inactiva"},
		{name: "renombrar una subcategoría ya activa bajo un padre inactivo no se bloquea por el padre",
			facts: CategoryFacts{Target: &CategoryNode{ID: 6, Name: "Viejo", ParentID: catID(2), Active: true}, Parent: inactiveRoot},
			next:  CategoryNext{Name: "Nuevo", ParentID: catID(2), Active: true}},
		{name: "apagar una subcategoría inactiva bajo padre inactivo", facts: CategoryFacts{Target: &CategoryNode{ID: 5, Name: "Molletes", ParentID: catID(2)}, Parent: inactiveRoot},
			next: CategoryNext{Name: "Molletes", ParentID: catID(2), Active: false}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckCategoryChange(c.facts, c.next)
			if c.want == nil {
				if err != nil {
					t.Fatalf("se rechazó un cambio válido: %v", err)
				}
				return
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, quiere %v", err, c.want)
			}
			if c.msg != "" && !strings.Contains(err.Error(), c.msg) {
				t.Errorf("el mensaje %q no dice %q", err.Error(), c.msg)
			}
		})
	}
}

func TestNormalizeCategoryName(t *testing.T) {
	got, err := NormalizeCategoryName("  Tés  ")
	if err != nil || got != "Tés" {
		t.Fatalf("= %q, %v; quiere \"Tés\" sin espacios en los extremos", got, err)
	}
}
