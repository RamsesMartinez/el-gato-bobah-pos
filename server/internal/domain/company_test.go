package domain

import "testing"

func TestValidSlug(t *testing.T) {
	good := []string{"gatobobah", "acme", "el-gato", "a1", "cafe-2024", "x0"}
	bad := []string{
		"a",            // muy corto
		"-abc", "abc-", // guion al borde
		"Acme",        // mayúscula
		"gato_bobah",  // guion bajo
		"café",        // no-ascii
		"con espacio", // espacio
		"",            // vacío
		"esto-es-un-slug-demasiado-largo-para-cuarenta", // >40
	}
	for _, s := range good {
		if !ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = false, want true", s)
		}
	}
	for _, s := range bad {
		if ValidSlug(s) {
			t.Errorf("ValidSlug(%q) = true, want false", s)
		}
	}
}
