package config

import (
	"strings"
	"testing"
)

// PIN_PEPPER es la llave que vuelve inútil la huella del PIN para quien se lleve la base. Un valor
// débil la convierte en un diccionario de un millón de entradas invertible en segundos, así que
// vale tan poco como no tenerla — pero da la falsa sensación de que hay protección.
//
// Es OPCIONAL: sin él el sistema funciona igual y solo el modo de solo-PIN queda bloqueado. Lo que
// no se acepta es que esté PUESTO y sea basura.
func TestPinPepperSeValidaSoloSiEstaPuesto(t *testing.T) {
	base := func() Config {
		return Config{
			// Entorno de desarrollo: este test es sobre el pepper, y producción exige media docena
			// de cosas más que solo estorbarían el caso que se quiere aislar.
			Env: "development", JWTSecret: strings.Repeat("k", 48),
			DatabaseURL: "postgres://x",
		}
	}
	casos := []struct {
		nombre string
		pepper string
		valido bool
	}{
		{"ausente: el sistema arranca igual", "", true},
		{"largo y aleatorio", strings.Repeat("p", 48), true},

		// Un placeholder olvidado en el .env es el modo más común de dejar un secreto falso en
		// producción, y es exactamente lo que pasó con "pepper-de-prueba" en main.go.
		{"placeholder", "cambia-esto", false},
		{"de prueba", "pepper-de-prueba", false},
		// Corto = invertible. La huella cubre un espacio de 10^6; con una llave adivinable, quien
		// tenga la base recorre ese millón en segundos.
		{"demasiado corto", "1234", false},
		{"casi suficiente", strings.Repeat("p", 31), false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			cfg := base()
			cfg.PinPepper = c.pepper
			err := Validate(cfg)
			if c.valido && err != nil {
				t.Fatalf("Validate() = %v, quiere nil", err)
			}
			if !c.valido && err == nil {
				t.Fatal("Validate() aceptó un pepper que no protege nada")
			}
		})
	}
}
