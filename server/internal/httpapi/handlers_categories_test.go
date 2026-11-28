package httpapi

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// El PATCH de una categoría (spec 099) distingue «no mandé el padre» de «quítale el padre»: con un
// *int64 a secas los dos serían nil y volver raíz sería imposible, o peor, mandar solo el nombre
// la volvería raíz sin pedirlo.
func TestDecodeCategoryPatch(t *testing.T) {
	cases := []struct {
		name, body string
		family     bool
		check      func(p categoryPatchBody) bool
		bad        bool
	}{
		{name: "solo la familia, como la pantalla", body: `{"foodFamily":"tea"}`, family: true,
			check: func(p categoryPatchBody) bool { return !p.Patch.SetParent && p.Patch.Name == nil }},
		{name: "solo el nombre no toca el padre", body: `{"name":"Tés"}`,
			check: func(p categoryPatchBody) bool { return !p.Patch.SetParent && *p.Patch.Name == "Tés" }},
		{name: "parentId null la vuelve raíz", body: `{"parentId":null}`,
			check: func(p categoryPatchBody) bool { return p.Patch.SetParent && p.Patch.ParentID == nil }},
		{name: "mover", body: `{"parentId":5,"isActive":true}`,
			check: func(p categoryPatchBody) bool {
				return p.Patch.SetParent && *p.Patch.ParentID == 5 && *p.Patch.Active
			}},
		{name: "nada que cambiar", body: `{}`, bad: true},
		{name: "familia junto con otra cosa", body: `{"foodFamily":"tea","name":"x"}`, bad: true},
		{name: "padre que no es número", body: `{"parentId":"5"}`, bad: true},
		{name: "activa que no es booleano", body: `{"isActive":"no"}`, bad: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := decodeCategoryPatch(httptest.NewRequest("PATCH", "/", strings.NewReader(c.body)))
			if c.bad {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("err = %v; quiere ErrValidation (400)", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.HasFamily != c.family || !c.check(p) {
				t.Fatalf("= %+v", p)
			}
		})
	}
}

func TestDecodeCategoryCreate(t *testing.T) {
	if _, _, err := decodeCategoryCreate(httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"Tés","parentId":"x"}`))); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("parentId de texto = %v", err)
	}
	name, parent, err := decodeCategoryCreate(httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"Tés","parentId":3}`)))
	if err != nil || name != "Tés" || parent == nil || *parent != 3 {
		t.Fatalf("= %q %v %v", name, parent, err)
	}
}
