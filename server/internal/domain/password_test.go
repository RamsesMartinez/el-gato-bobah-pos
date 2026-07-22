package domain

import (
	"errors"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	cases := []struct {
		name string
		pw   string
		ok   bool
	}{
		{"too short (11)", "abcABC123!@", false}, // 11 runes
		{"exactly 12 ok", "correct-horse", true}, // 13 chars, no común
		{"long passphrase ok", "un caballo azul come pasto", true},
		{"common blocklist", "passwordpassword", false},
		{"common blocklist case-insensitive", "Administrador123", false},
		{"contains @ rejected", "correo@dominio-largo", false},
		{"other symbols allowed", "b!en$egura#2026", true},
		{"empty", "", false},
		{"unicode length counts runes", "áéíóúáéíóúá", false}, // 11 runes → corto
		{"unicode 12 runes ok", "áéíóúáéíóúáé", true},         // 12 runes, no común
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidatePassword(c.pw)
			if c.ok && err != nil {
				t.Errorf("ValidatePassword(%q) = %v, want nil", c.pw, err)
			}
			if !c.ok {
				if err == nil {
					t.Errorf("ValidatePassword(%q) = nil, want error", c.pw)
				} else if !errors.Is(err, ErrWeakPassword) {
					t.Errorf("ValidatePassword(%q) error = %v, want ErrWeakPassword", c.pw, err)
				}
			}
		})
	}
}
