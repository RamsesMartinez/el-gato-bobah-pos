package main

import (
	"os"
	"regexp"
	"testing"
)

// Los tres servicios que reciben el pepper tienen que recibir EL MISMO, y tiene que venir de la
// configuración.
//
// Este test existe por un defecto real: un script que parcheaba los tests alcanzó también main.go y
// dejó `NewSettingsService(st, "pepper-de-prueba")` cableado al binario de producción, en un repo
// público. No fue solo un secreto falso — la compuerta que impide encender el modo de solo-PIN
// pregunta `if pinPepper == ""`, así que con el literal NUNCA se cumplía y el fail-closed quedaba
// muerto en el binario real. `config.Validate` no puede verlo: un literal no pasa por la
// configuración.
func TestElPepperNuncaVaCableadoEnElBinario(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("leer main.go: %v", err)
	}

	// Cualquier constructor que reciba el pepper, con una cadena literal en vez de cfg.
	conLiteral := regexp.MustCompile(`New(SettingsService|UsersService|AuthServiceConPepper)\([^)]*"[^"]*"[^)]*\)`)
	if m := conLiteral.Find(src); m != nil {
		t.Fatalf("hay un secreto cableado en main.go: %s", m)
	}

	// Y los tres tienen que estar recibiéndolo: si alguno dejara de hacerlo, el modo de solo-PIN
	// quedaría a medias —unos servicios calculando huella y otros no— que es peor que apagado.
	for _, ctor := range []string{"NewSettingsService", "NewUsersService", "NewAuthServiceConPepper"} {
		conCfg := regexp.MustCompile(ctor + `\([^)]*cfg\.PinPepper[^)]*\)`)
		if !conCfg.Match(src) {
			t.Errorf("%s no recibe cfg.PinPepper: el modo de solo-PIN quedaría a medias", ctor)
		}
	}
}
