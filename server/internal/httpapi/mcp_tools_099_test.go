package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// Los argumentos de las herramientas de la 099 se rechazan ANTES de despachar: un agente que manda
// mal la merma o la categoría recibe qué corregir, no un cambio a medias.

func TestExpectedWasteSetArgsBody(t *testing.T) {
	cases := []struct {
		name    string
		args    expectedWasteSetArgs
		wantPct string // JSON del pct que se manda a la ruta
		errHas  string
	}{
		{name: "porcentaje", args: expectedWasteSetArgs{IngredientID: 1, Pct: new("8"), Reason: "rota poco"}, wantPct: "8"},
		{name: "cero es capturado en cero, no sin captura", args: expectedWasteSetArgs{IngredientID: 1, Pct: new("0"), Reason: "x"}, wantPct: "0"},
		{name: "quitar la captura", args: expectedWasteSetArgs{IngredientID: 1, Clear: true, Reason: "x"}, wantPct: "null"},
		{name: "ni pct ni clear: ausente no significa quitarla", args: expectedWasteSetArgs{IngredientID: 1, Reason: "x"}, errHas: "pct o clear"},
		{name: "los dos", args: expectedWasteSetArgs{IngredientID: 1, Pct: new("8"), Clear: true, Reason: "x"}, errHas: "pct o clear"},
		{name: "sin motivo", args: expectedWasteSetArgs{IngredientID: 1, Pct: new("8")}, errHas: "reason"},
		{name: "motivo de puros espacios", args: expectedWasteSetArgs{IngredientID: 1, Pct: new("8"), Reason: "   "}, errHas: "reason"},
		{name: "100 no cabe", args: expectedWasteSetArgs{IngredientID: 1, Pct: new("100"), Reason: "x"}, errHas: "menos de 100"},
		{name: "tres decimales", args: expectedWasteSetArgs{IngredientID: 1, Pct: new("7.125"), Reason: "x"}, errHas: "menos de 100"},
		{name: "cero a la izquierda no es un número JSON y tronaba al aplicar", args: expectedWasteSetArgs{IngredientID: 1, Pct: new("08"), Reason: "x"}, errHas: "menos de 100"},
		{name: "00 tampoco", args: expectedWasteSetArgs{IngredientID: 1, Pct: new("00"), Reason: "x"}, errHas: "menos de 100"},
		{name: "0.5 sí", args: expectedWasteSetArgs{IngredientID: 1, Pct: new("0.5"), Reason: "x"}, wantPct: "0.5"},
		{name: "negativo", args: expectedWasteSetArgs{IngredientID: 1, Pct: new("-1"), Reason: "x"}, errHas: "menos de 100"},
		{name: "sin insumo", args: expectedWasteSetArgs{Pct: new("8"), Reason: "x"}, errHas: "ingredient_id"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body, err := c.args.body()
			if c.errHas != "" {
				if err == nil || !strings.Contains(err.Error(), c.errHas) {
					t.Fatalf("err = %v; quiere uno que diga %q", err, c.errHas)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal(body["pct"])
			if string(b) != c.wantPct {
				t.Fatalf("pct = %s; quiere %s (la ruta rechaza el %% como texto)", b, c.wantPct)
			}
		})
	}
}

func TestCategoryUpdateArgsBody(t *testing.T) {
	cases := []struct {
		name   string
		args   categoryUpdateArgs
		want   string
		errHas string
	}{
		{name: "apagar", args: categoryUpdateArgs{CategoryID: 3, Active: new(false)}, want: `{"isActive":false}`},
		{name: "renombrar y mover", args: categoryUpdateArgs{CategoryID: 3, Name: new("Tés"), ParentID: new(int64(1))}, want: `{"name":"Tés","parentId":1}`},
		{name: "volver raíz manda parentId null, no lo omite", args: categoryUpdateArgs{CategoryID: 3, MakeRoot: true}, want: `{"parentId":null}`},
		{name: "nada que cambiar", args: categoryUpdateArgs{CategoryID: 3}, errHas: "nada que cambiar"},
		{name: "mover y volver raíz a la vez", args: categoryUpdateArgs{CategoryID: 3, ParentID: new(int64(1)), MakeRoot: true}, errHas: "parent_id o make_root"},
		{name: "sin categoría", args: categoryUpdateArgs{Active: new(false)}, errHas: "category_id"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body, err := c.args.body()
			if c.errHas != "" {
				if err == nil || !strings.Contains(err.Error(), c.errHas) {
					t.Fatalf("err = %v; quiere uno que diga %q", err, c.errHas)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal(body)
			if string(b) != c.want {
				t.Fatalf("cuerpo = %s; quiere %s", b, c.want)
			}
		})
	}
}

func TestCategoryCreateArgsBody(t *testing.T) {
	if _, err := (categoryCreateArgs{Name: "  "}).body(); err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("nombre vacío = %v", err)
	}
	b, err := (categoryCreateArgs{Name: "Tés", ParentID: new(int64(1))}).body()
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := json.Marshal(b); string(got) != `{"name":"Tés","parentId":1}` {
		t.Fatalf("cuerpo = %s", got)
	}
	b, _ = (categoryCreateArgs{Name: "Tés"}).body()
	if got, _ := json.Marshal(b); string(got) != `{"name":"Tés","parentId":null}` {
		t.Fatalf("cuerpo de una raíz = %s", got)
	}
}

// Un cuerpo ilegible de la lista de categorías se dice como error: leído como «no hay categorías»,
// la vista previa afirmaba «no existe la categoría N» de una que sí existe.
func TestParseCategoryRows(t *testing.T) {
	if _, err := parseCategoryRows([]byte(`{"items": [{"id": "no es número"}]}`)); err == nil {
		t.Fatal("un cuerpo ilegible pasó como lista vacía")
	}
	rows, err := parseCategoryRows([]byte(`{"items": [{"id": 3, "name": "Frías", "parentId": 1, "isActive": true}]}`))
	if err != nil || len(rows) != 1 || rows[0].ID != 3 || rows[0].ParentID == nil || *rows[0].ParentID != 1 {
		t.Fatalf("= %+v, %v", rows, err)
	}
}
