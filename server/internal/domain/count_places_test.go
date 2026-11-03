package domain

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestValidCountPlaceName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"Refri", true},
		{"  Refri  ", true},
		{"", false},
		{"   ", false},
		{strings.Repeat("ñ", 30), true}, // 30 letras, 60 bytes: el tope cuenta letras
		{strings.Repeat("a", 31), false},
		{"Ñandú", true},
	}
	for _, c := range cases {
		err := ValidCountPlaceName(c.name)
		if (err == nil) != c.ok {
			t.Errorf("ValidCountPlaceName(%q) = %v; quiere ok=%v", c.name, err, c.ok)
		}
		if err != nil && !errors.Is(err, ErrValidation) {
			t.Errorf("ValidCountPlaceName(%q) = %v; quiere ErrValidation", c.name, err)
		}
	}
}

// CleanCountPlaceName es el nombre que se guarda: «refri » y «Refri» no pueden dejar dos lugares, y el
// que se guarda no lleva los espacios de más con que se capturó.
func TestCleanCountPlaceName(t *testing.T) {
	if got := CleanCountPlaceName("  Anaquel   seco "); got != "Anaquel seco" {
		t.Fatalf("= %q", got)
	}
}

func TestMoveCountPlace(t *testing.T) {
	order := []int64{10, 20, 30, 40}
	cases := []struct {
		id   int64
		to   int
		want []int64
		err  bool
	}{
		{30, 0, []int64{30, 10, 20, 40}, false},
		{10, 3, []int64{20, 30, 40, 10}, false},
		{20, 2, []int64{10, 30, 20, 40}, false},
		{20, 1, []int64{10, 20, 30, 40}, false},
		{20, -1, nil, true},
		{20, 4, nil, true},
		{99, 0, nil, true}, // un lugar que no está entre los activos
	}
	for _, c := range cases {
		got, err := MoveCountPlace(order, c.id, c.to)
		if c.err {
			if !errors.Is(err, ErrValidation) {
				t.Errorf("mover %d a %d = %v; quiere ErrValidation", c.id, c.to, err)
			}
			continue
		}
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("mover %d a %d = %v, %v; quiere %v", c.id, c.to, got, err, c.want)
		}
	}
	if !slices.Equal(order, []int64{10, 20, 30, 40}) {
		t.Fatal("MoveCountPlace cambió el orden que recibió")
	}
}
