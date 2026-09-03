package main

import "testing"

// EL PIN DE EJEMPLO DEL REPO PÚBLICO SE ACEPTABA.
//
// `ADMIN_PASSWORD` pasaba por `config.IsPlaceholder` y el PIN no, solo por `IsWeakPin` — que mira
// secuencias y repeticiones, no valores de ejemplo. "cambia-esto" tiene once caracteres y no es
// ninguna de las dos cosas, así que un despliegue copiado de `deploy/.env.example` con solo la
// contraseña cambiada dejaba al admin con un PIN publicado en GitHub.
//
// La feature 004 subió la exposición: `/auth/unlock-options` le publica a cualquier autenticado el
// id y el nombre del administrador, así que quien levante una tableta desatendida ya no tiene que
// adivinar contra quién probar.
//
// Y un PIN alfabético no se puede teclear en el teclado numérico de la pantalla de bloqueo: se
// aceptaba un valor que nadie podía usar para entrar, y el admin se quedaba sin desbloqueo sin que
// nada lo dijera.
func TestElPinDeEjemploNoSeAcepta(t *testing.T) {
	casos := []struct {
		nombre string
		pin    string
		quiere bool // true = tiene que rechazarse
	}{
		{"el de .env.example", "cambia-esto", true},
		{"otro placeholder", "your_pin_here", true},
		{"vacío es no configurar PIN", "", false},
		{"secuencia", "1234", true},
		{"todos iguales", "0000", true},
		{"con letras no se teclea en el keypad", "abc123", true},
		{"uno válido", "482715", false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := checkAdminSecrets("Contrasena-Larga-1!", c.pin)
			if (err != nil) != c.quiere {
				t.Errorf("checkAdminSecrets(%q) = %v; rechazar=%v", c.pin, err, c.quiere)
			}
		})
	}
}
