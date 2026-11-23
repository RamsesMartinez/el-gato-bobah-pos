package domain

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestSlugifyMenuName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"El Gato Bobah", "el-gato-bobah"},
		{"Doña Lupe", "dona-lupe"},
		{"  Fonda   «La Güera»  ", "fonda-la-guera"},
		{"ÁÉÍÓÚ àèìòù âêîôû äëïöü ãõ ç Ñ", "aeiou-aeiou-aeiou-aeiou-ao-c-n"},
		{"Tacos #1 & Más", "tacos-1-mas"},
		{"--lupe--fonda--", "lupe-fonda"},
		{"🍕", ""},
		{"", ""},
		{"Ñ", "n"},
		// 41 caracteres: se trunca a 40 y no deja guion al final del corte.
		{strings.Repeat("a", 39) + " b", strings.Repeat("a", 39)},
		{strings.Repeat("a", 40), strings.Repeat("a", 40)},
		{strings.Repeat("a", 41), strings.Repeat("a", 40)},
	}
	for _, c := range cases {
		if got := SlugifyMenuName(c.in); got != c.want {
			t.Errorf("SlugifyMenuName(%q) = %q, quiero %q", c.in, got, c.want)
		}
	}
}

func TestValidMenuNameShape(t *testing.T) {
	ok := []string{"abc", "el-gato-bobah", "fonda-2", strings.Repeat("a", 40)}
	bad := []string{"", "ab", "-abc", "abc-", "a--b", "Abc", "dona lupe", "doña", strings.Repeat("a", 41), "abc/def", "abc.def"}
	for _, n := range ok {
		if !ValidMenuNameShape(n) {
			t.Errorf("%q debería ser válido", n)
		}
	}
	for _, n := range bad {
		if ValidMenuNameShape(n) {
			t.Errorf("%q no debería ser válido: la Function lo rechaza antes de la base", n)
		}
	}
}

func TestReservedMenuNames(t *testing.T) {
	for _, n := range []string{"q", "api", "admin", "www", "menu", "static", "assets", "login", "ayuda", "soporte"} {
		if !IsReservedMenuName(n) {
			t.Errorf("%q debe estar reservado: choca con una ruta del menú o confunde al cliente", n)
		}
	}
	if IsReservedMenuName("el-gato-bobah") {
		t.Error("un nombre de negocio normal no está reservado")
	}
}

func TestValidPalette(t *testing.T) {
	for _, p := range []string{"brand", "blue", "teal", "green", "cyan", "purple", "pink", "orange"} {
		if !ValidPalette(p) {
			t.Errorf("%q es una paleta de Interfaz", p)
		}
	}
	for _, p := range []string{"", "red", "Brand", "#ff0000"} {
		if ValidPalette(p) {
			t.Errorf("%q no es una paleta conocida", p)
		}
	}
}

func TestFeatureKeyIsStable(t *testing.T) {
	// Se guardará como dato en el #333: cambiarlo dejaría sin menú a quien ya lo tenía en su paquete.
	if FeaturePublicMenu != "public_menu" {
		t.Fatal("la clave de la función no puede cambiar")
	}
}

func TestPlanMenuRename(t *testing.T) {
	retired := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = "viejo-" + strings.Repeat("x", i+1)
		}
		return out
	}
	cases := []struct {
		name    string
		in      MenuRenameInput
		want    MenuRenameAction
		wantErr error
	}{
		{"igual al vigente", MenuRenameInput{Requested: "lupe", Current: "lupe"}, MenuRenameNone, nil},
		{"forma inválida", MenuRenameInput{Requested: "ab", Current: "lupe"}, 0, ErrValidation},
		{"reservado", MenuRenameInput{Requested: "admin", Current: "lupe"}, 0, ErrMenuNameUnavailable},
		{"de otra empresa", MenuRenameInput{Requested: "otra", Current: "lupe", TakenElsewhere: true}, 0, ErrMenuNameUnavailable},
		{"revivir uno propio", MenuRenameInput{Requested: "viejo", Current: "lupe", Retired: []string{"viejo"}}, MenuRenameRevive, nil},
		{"revivir con el tope lleno", MenuRenameInput{Requested: "viejo-x", Current: "lupe", Retired: retired(MaxRetiredMenuNames)}, MenuRenameRevive, nil},
		{"nuevo", MenuRenameInput{Requested: "nuevo", Current: "lupe"}, MenuRenameInsert, nil},
		{"nuevo con el tope lleno", MenuRenameInput{Requested: "nuevo", Current: "lupe", Retired: retired(MaxRetiredMenuNames)}, 0, ErrMenuNameLimit},
	}
	for _, c := range cases {
		got, err := PlanMenuRename(c.in)
		if c.wantErr != nil {
			if !errors.Is(err, c.wantErr) {
				t.Errorf("%s: err %v, quiero %v", c.name, err, c.wantErr)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: %v, %v; quiero %v", c.name, got, err, c.want)
		}
	}
}

// testdata/menu_slug_cases.json es la tabla que comparten Go, la base (public_menu_slugify) y la
// pantalla de Ajustes (web/src/features/admin/settings/menuPublico/slug.ts): si una de las tres
// normaliza distinto, la vista previa promete una dirección y se guarda otra.
func TestSlugifyMenuNameSharedCases(t *testing.T) {
	raw, err := os.ReadFile("testdata/menu_slug_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases [][2]string
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if got := SlugifyMenuName(c[0]); got != c[1] {
			t.Errorf("SlugifyMenuName(%q) = %q, la tabla compartida dice %q", c[0], got, c[1])
		}
	}
}
