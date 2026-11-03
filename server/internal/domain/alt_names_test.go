package domain

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestCatalogNameKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Café Frío", "cafe frio"},
		{"  café   frío ", "cafe frio"},
		{"Cafe\tfrio", "cafe frio"},
		{"Café\t", "cafe"},
		{"\nCafé\n", "cafe"},
		{"café frío", "cafe frio"},
		{" café ", "cafe"},
		{"PIÑA Ü", "pina u"},
	}
	for _, c := range cases {
		if got := CatalogNameKey(c.in); got != c.want {
			t.Errorf("CatalogNameKey(%q) = %q, quiero %q", c.in, got, c.want)
		}
	}
}

func TestCleanAltNames(t *testing.T) {
	long := strings.Repeat("a", MaxAltName+1)
	eleven := make([]string, MaxAltNames+1)
	for i := range eleven {
		eleven[i] = string(rune('a'+i)) + " nombre"
	}
	cases := []struct {
		name    string
		own     string
		in      []string
		want    []string
		wantErr bool
	}{
		{name: "nil da lista vacía, nunca nil", own: "Ice Coffee", in: nil, want: []string{}},
		{name: "recorta y junta espacios, conserva el orden", own: "Ice Coffee",
			in: []string{"  Café   helado ", "Café frío"}, want: []string{"Café helado", "Café frío"}},
		{name: "descarta vacíos y de puros espacios", own: "Ice Coffee", in: []string{"", "   ", "\t", "Frío"}, want: []string{"Frío"}},
		{name: "diez caben", own: "X", in: eleven[:10], want: eleven[:10]},
		{name: "once no", own: "X", in: eleven, wantErr: true},
		{name: "once con un vacío sí: el vacío no cuenta", own: "X", in: append(append([]string{}, eleven[:10]...), " "), want: eleven[:10]},
		{name: "más de 60 letras no", own: "X", in: []string{long}, wantErr: true},
		{name: "60 letras con acentos sí", own: "X", in: []string{strings.Repeat("é", MaxAltName)}, want: []string{strings.Repeat("é", MaxAltName)}},
		{name: "repetido en la lista, sin acento ni mayúsculas", own: "X", in: []string{"Café frío", "cafe  FRIO"}, wantErr: true},
		{name: "igual al nombre propio", own: "Ice Coffee", in: []string{"ice coffee"}, wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := CleanAltNames(c.own, c.in)
			if c.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("quería ErrValidation, tuve %v (%v)", err, got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got == nil || !slices.Equal(got, c.want) {
				t.Fatalf("got %#v, quiero %#v", got, c.want)
			}
		})
	}
}

func TestCleanAltNamesSaysWhichIsRepeated(t *testing.T) {
	_, err := CleanAltNames("Latte", []string{"Café con leche", "cafe con leche"})
	if err == nil || !strings.Contains(err.Error(), "cafe con leche") {
		t.Fatalf("el error no nombra el repetido: %v", err)
	}
	_, err = CleanAltNames("Latte", []string{"latte"})
	if err == nil || !strings.Contains(err.Error(), "latte") {
		t.Fatalf("el error no nombra el igual al nombre: %v", err)
	}
}

func TestAltNameTakenMessage(t *testing.T) {
	cases := []struct {
		err  AltNameTaken
		want string
	}{
		{AltNameTaken{Name: "café frío", OwnerName: "Latte", OwnerAlt: true}, "«café frío» ya es otro nombre de «Latte»"},
		{AltNameTaken{Name: "frappe de cafe", OwnerName: "Frappé de café"}, "«frappe de cafe» ya es el nombre de «Frappé de café»"},
		{AltNameTaken{Name: "x", OwnerName: "Y", OwnerArchived: true, Article: true}, "«x» ya es el nombre de «Y» (archivado)"},
		{AltNameTaken{Name: "x", OwnerName: "Y", OwnerArchived: true}, "«x» ya es el nombre de «Y» (inactivo)"},
	}
	for _, c := range cases {
		if got := c.err.Error(); got != c.want {
			t.Errorf("got %q, quiero %q", got, c.want)
		}
		if !errors.Is(c.err, ErrAltNameTaken) || !errors.Is(c.err, ErrConflict) {
			t.Errorf("%v no envuelve ErrAltNameTaken/ErrConflict", c.err)
		}
	}
}

func TestSimilarAltNames(t *testing.T) {
	cands := []AltCandidate{
		{ID: 1, Name: "Café frío de olla"},
		{ID: 2, Name: "Frappé de café", Alt: "Café frío batido"},
		{ID: 2, Name: "Frappé de café", Alt: "Frappé"},
		{ID: 3, Name: "Ice Coffee"}, // el propio
		{ID: 4, Name: "café  FRIO"}, // idéntico: ése es el bloqueo, no un aviso
		{ID: 5, Name: "Té verde"},
	}
	got := SimilarAltNames("Café frío", 3, cands, 5)
	ids := []int64{}
	for _, g := range got {
		ids = append(ids, g.ID)
	}
	if slices.Contains(ids, 3) || slices.Contains(ids, 4) || slices.Contains(ids, 5) {
		t.Fatalf("trae el propio, el idéntico o uno que no se parece: %v", got)
	}
	if !slices.Contains(ids, 1) || !slices.Contains(ids, 2) {
		t.Fatalf("no trae «Café frío de olla» ni «Frappé de café» por su «Café frío batido»: %v", got)
	}
	for _, g := range got {
		if g.ID == 2 && g.ViaAlt != "Café frío batido" {
			t.Fatalf("«Frappé de café» no dice por cuál alternativo: %+v", g)
		}
	}
	seen := map[int64]bool{}
	for _, g := range got {
		if seen[g.ID] {
			t.Fatalf("repite el id %d: %v", g.ID, got)
		}
		seen[g.ID] = true
	}
	if got := SimilarAltNames("zzz", 0, nil, 5); got == nil {
		t.Fatal("sin candidatos devuelve nil; quiero []")
	}
	many := []AltCandidate{}
	for i := range 10 {
		many = append(many, AltCandidate{ID: int64(100 + i), Name: "Café frío de olla " + string(rune('a'+i))})
	}
	if got := SimilarAltNames("Café frío de olla", 0, many, 5); len(got) > 5 {
		t.Fatalf("más de 5: %d", len(got))
	}
}
