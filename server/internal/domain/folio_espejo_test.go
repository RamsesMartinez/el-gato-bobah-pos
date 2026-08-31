package domain

import (
	"os"
	"regexp"
	"testing"
)

// La lista de animales vive dos veces: aquí y en web/src/features/pos/folio.ts, porque la pantalla
// le pone nombre a la cuenta al abrirla y pedirlo al servidor sería un viaje de red por cada
// cuenta nueva. Este test es lo que hace sostenible esa copia.
//
// Si divergen no truena nada de inmediato —el servidor valida FORMA, no pertenencia— pero un
// animal que solo existe de un lado hace que ese nombre nunca se reparta, o que se reparta uno que
// la pantalla no sabe mostrar. El fallo diría "todo bien" hasta que alguien lo notara en el papel.
func TestLaListaDelFrontEsLaMisma(t *testing.T) {
	const ruta = "../../../web/src/features/pos/folio.ts"
	src, err := os.ReadFile(ruta)
	if err != nil {
		t.Skipf("sin %s a la mano (¿se corre fuera del repo?): %v", ruta, err)
	}

	bloque := regexp.MustCompile(`(?s)export const ANIMALES = \[(.*?)\n\] as const;`).FindSubmatch(src)
	if bloque == nil {
		t.Fatalf("no encontré el arreglo ANIMALES en %s; ¿cambió de forma?", ruta)
	}
	var front []string
	for _, m := range regexp.MustCompile(`'([^']+)'`).FindAllSubmatch(bloque[1], -1) {
		front = append(front, string(m[1]))
	}

	if len(front) != len(animales) {
		t.Fatalf("el front tiene %d animales y el servidor %d", len(front), len(animales))
	}
	enGo := map[string]bool{}
	for _, a := range animales {
		enGo[a] = true
	}
	enFront := map[string]bool{}
	for _, a := range front {
		enFront[a] = true
	}
	for _, a := range animales {
		if !enFront[a] {
			t.Errorf("%q está en el servidor y no en el front", a)
		}
	}
	for _, a := range front {
		if !enGo[a] {
			t.Errorf("%q está en el front y no en el servidor", a)
		}
	}
}
