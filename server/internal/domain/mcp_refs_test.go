package domain

import (
	"strings"
	"testing"
)

// #335: el rechazo de un id que no es de la empresa nombra qué es y cuál, con el MISMO texto para uno
// que existe en otra empresa y uno que no existe en ninguna (la capa no sabe la diferencia y no debe
// decirla), y dice con qué lectura se busca.
func TestMissingRefMessageNamesWhatAndWhich(t *testing.T) {
	cases := []struct {
		ref  Ref
		want []string
	}{
		{Ref{Kind: RefProduct, ID: 3}, []string{"El producto 3 no existe en esta empresa", "products_list"}},
		{Ref{Kind: RefIngredient, ID: 7}, []string{"El insumo 7 no existe en esta empresa", "articles_search"}},
		{Ref{Kind: RefWarehouse, ID: 9}, []string{"El almacén 9 no existe en esta empresa", "warehouses_list"}},
		{Ref{Kind: RefExpenseCategory, ID: 2}, []string{"La categoría de gasto 2 no existe en esta empresa", "expense_categories_list"}},
	}
	for _, c := range cases {
		got := MissingRefMessage(c.ref)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%v: %q no contiene %q", c.ref, got, w)
			}
		}
	}
}

// Toda clase de referencia tiene su sustantivo y su lectura: una que se agregue sin ellos daría «  5 no
// existe» y un agente sin forma de buscar el id bueno.
func TestEveryRefKindHasNounAndListTool(t *testing.T) {
	for k, info := range refKinds {
		if info.noun == "" || info.listTool == "" || info.table == "" {
			t.Errorf("%s sin sustantivo, lectura o tabla: %+v", k, info)
		}
		if _, ok := MCPToolByName(info.listTool); !ok {
			t.Errorf("%s apunta a la lectura %s, que no está en el catálogo", k, info.listTool)
		}
	}
}

// La FK compuesta que frena un id ajeno no dice el valor (con RLS, Postgres lo oculta), solo la tabla.
// De la tabla sale el sustantivo; una tabla que no se conoce da un texto genérico, nunca su nombre.
func TestReferenceNotFoundFromTable(t *testing.T) {
	if got := ReferenceNotFoundIn("products").Error(); !strings.Contains(got, "el producto") || !strings.Contains(got, "no existe en esta empresa") {
		t.Errorf("products: %q", got)
	}
	got := ReferenceNotFoundIn("platform_menu_reads").Error()
	if strings.Contains(got, "platform_menu_reads") || !strings.Contains(got, "no existe en esta empresa") {
		t.Errorf("una tabla interna se le nombraría a quien opera: %q", got)
	}
}

// #335: un 500 le dice a quien llamó un id con el que el administrador encuentra el correo, una causa
// por categoría (nunca el texto del error, que puede traer la consulta o datos) y si es seguro
// reintentar.
func TestInternalErrorHint(t *testing.T) {
	notApplied := InternalErrorHint("req-123", InternalCause("pg:XX000"), InternalNotApplied)
	for _, w := range []string{"req-123", "No se aplicó nada", "misma idempotency_key"} {
		if !strings.Contains(notApplied, w) {
			t.Errorf("sin aplicar: %q no contiene %q", notApplied, w)
		}
	}
	maybe := InternalErrorHint("req-9", InternalCause("*net.OpError"), InternalMayHaveApplied)
	for _, w := range []string{"req-9", "pudo haberse aplicado", "lectura"} {
		if !strings.Contains(maybe, w) {
			t.Errorf("tal vez aplicado: %q no contiene %q", maybe, w)
		}
	}
	for kind, want := range map[string]string{
		"pg:23514":                      "regla de los datos",
		"pg:40001":                      "otra operación",
		"pg:40P01":                      "otra operación",
		"pg:57014":                      "tardó demasiado",
		"context.deadlineExceededError": "tardó demasiado",
		"pg:08006":                      "base de datos no respondió",
		"*pgconn.ConnectError":          "base de datos no respondió",
		"*errors.errorString":           "no previsto",
		"pg:P0001":                      "base de datos rechazó",
	} {
		if got := InternalCause(kind); !strings.Contains(got, want) {
			t.Errorf("%s: %q no contiene %q", kind, got, want)
		}
	}
}
