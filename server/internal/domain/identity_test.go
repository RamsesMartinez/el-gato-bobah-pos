package domain

import (
	"errors"
	"testing"
)

func TestIdentitySettingsValidate(t *testing.T) {
	ok := func(lock, horas int) IdentitySettings {
		return IdentitySettings{LockAfterSeconds: lock, SessionHours: horas}
	}
	casos := []struct {
		nombre string
		in     IdentitySettings
		quiere error
	}{
		{"los defaults", ok(180, 8), nil},
		// Cero es una elección válida: una caja en una oficina cerrada no necesita bloquearse.
		{"sin bloqueo", ok(0, 8), nil},
		{"una hora de bloqueo", ok(3600, 8), nil},
		{"treinta días de sesión", ok(180, 720), nil},

		{"bloqueo negativo", ok(-1, 8), ErrValidation},
		// Un cero de más deja la tableta bloqueada para siempre y nadie buscaría la causa aquí.
		{"bloqueo absurdo", ok(36000, 8), ErrValidation},
		// Cero horas dejaría la tableta pidiendo credenciales a cada instante.
		{"sesión en cero", ok(180, 0), ErrValidation},
		{"sesión negativa", ok(180, -8), ErrValidation},
		{"sesión absurda", ok(180, 100000), ErrValidation},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if err := c.in.Validate(); !errors.Is(err, c.quiere) {
				t.Fatalf("Validate(%+v) = %v, quiere %v", c.in, err, c.quiere)
			}
		})
	}
}
