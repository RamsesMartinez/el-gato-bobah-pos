package domain

import (
	"strings"
	"testing"
	"time"
)

// Las tres reglas que hacen utilizable la lista. Viven como test y no como comentario porque el
// día que alguien agregue "Gallo" junto a "Ganso", o "Camarón" porque suena bonito, el comentario
// no lo detiene y esto sí.
func TestLaListaDeAnimalesSeGritaSinConfusion(t *testing.T) {
	if len(animales) != 100 {
		t.Fatalf("la lista tiene %d nombres, quiere 100", len(animales))
	}

	visto := map[string]string{}
	for _, a := range animales {
		if otro, dup := visto[a]; dup {
			t.Errorf("%q está repetido (%q)", a, otro)
		}
		visto[a] = a
	}

	// Nueve letras es lo que cabe grande en un ticket de 58 mm y se dice de un tirón.
	for _, a := range animales {
		if n := len([]rune(a)); n > 9 {
			t.Errorf("%q tiene %d letras, el tope es 9", a, n)
		}
	}

	// Dos nombres que empiezan igual son el mismo sonido en una cocina ruidosa, y eso entrega el
	// pedido equivocado. La primera sílaba es lo único que se alcanza a oír.
	porSilaba := map[string]string{}
	for _, a := range animales {
		clave := sinAcentos(a)
		if len(clave) > 3 {
			clave = clave[:3]
		}
		if otro, choca := porSilaba[clave]; choca {
			t.Errorf("%q y %q empiezan igual (%q): se confunden al cantarlos", a, otro, clave)
		}
		porSilaba[clave] = a
	}
}

// Una vuelta completa usa los 100 nombres una sola vez: si el barajado perdiera uno, dos pedidos
// distintos del mismo día se llamarían igual y la cocina no podría distinguirlos.
func TestUnaVueltaUsaCadaNombreUnaSolaVez(t *testing.T) {
	dia := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	visto := map[string]int{}
	for n := 1; n <= len(animales); n++ {
		nombre := NombreDeFolio(7, dia, n)
		if antes, dup := visto[nombre]; dup {
			t.Fatalf("%q salió en el pedido %d y otra vez en el %d", nombre, antes, n)
		}
		visto[nombre] = n
	}
	if len(visto) != len(animales) {
		t.Fatalf("se usaron %d nombres distintos, quiere %d", len(visto), len(animales))
	}
}

// Al agotarse la lista el nombre se repite con número, que es lo que permite que un local con 200
// pedidos al día siga cantando nombres en vez de volver a los folios.
func TestAlAgotarseLaListaElNombreLlevaNumero(t *testing.T) {
	dia := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	primero := NombreDeFolio(7, dia, 1)

	if segundaVuelta := NombreDeFolio(7, dia, 1+len(animales)); segundaVuelta != primero+" 2" {
		t.Errorf("pedido %d = %q, quiere %q", 1+len(animales), segundaVuelta, primero+" 2")
	}
	if tercera := NombreDeFolio(7, dia, 1+2*len(animales)); tercera != primero+" 3" {
		t.Errorf("tercera vuelta = %q, quiere %q", tercera, primero+" 3")
	}
}

// El orden se revuelve por día: si fuera secuencial, el primer nombre sería siempre el mismo y
// acabaría significando "el temprano" en vez de identificar a un pedido.
func TestCadaDiaEmpiezaConOtroAnimal(t *testing.T) {
	var arranques []string
	for d := 1; d <= 10; d++ {
		arranques = append(arranques, NombreDeFolio(7, time.Date(2026, 8, d, 0, 0, 0, 0, time.UTC), 1))
	}
	distintos := map[string]bool{}
	for _, a := range arranques {
		distintos[a] = true
	}
	if len(distintos) < 7 {
		t.Errorf("en 10 días solo hubo %d arranques distintos (%v): el barajado no está revolviendo",
			len(distintos), arranques)
	}
}

// Dos empresas el mismo día no comparten el orden. No es un requisito de seguridad —el nombre no
// es secreto—, pero dos locales de la misma cadena cantando "Tigre" a la misma hora es una
// coincidencia que confunde a quien opera los dos.
func TestDosEmpresasElMismoDiaNoCompartenElOrden(t *testing.T) {
	dia := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	if NombreDeFolio(1, dia, 1) == NombreDeFolio(2, dia, 1) {
		t.Error("dos empresas arrancaron con el mismo animal el mismo día")
	}
}

// El mismo pedido pide su nombre dos veces (al imprimir el ticket y al reimprimirlo) y tiene que
// dar lo mismo. Es la razón de que el generador sea propio y no math/rand.
func TestElMismoPedidoSiempreDaElMismoNombre(t *testing.T) {
	dia := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 50; i++ {
		if a, b := NombreDeFolio(7, dia, 42), NombreDeFolio(7, dia, 42); a != b {
			t.Fatalf("dos llamadas dieron %q y %q", a, b)
		}
	}
}

// Un pedido sin número todavía no existe; devolver un animal sería inventarle identidad a algo que
// no la tiene, y ese nombre acabaría impreso.
func TestSinNumeroNoHayNombre(t *testing.T) {
	dia := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	for _, n := range []int{0, -1, -100} {
		if got := NombreDeFolio(7, dia, n); got != "" {
			t.Errorf("NombreDeFolio(…, %d) = %q, quiere vacío", n, got)
		}
	}
}

// sinAcentos deja "Búho" como "buho" para comparar sílabas iniciales. Solo cubre las vocales
// acentuadas, la diéresis y la eñe, que es todo lo que aparece en la lista.
func sinAcentos(s string) string {
	r := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	)
	return r.Replace(strings.ToLower(s))
}

func TestSanitizarFolio(t *testing.T) {
	casos := []struct {
		nombre, entra, quiere string
	}{
		{"un animal normal", "Tigre", "Tigre"},
		{"con acento", "Búho", "Búho"},
		{"con eñe", "Ñandú", "Ñandú"},
		{"espacios alrededor", "  Tigre  ", "Tigre"},
		{"el más largo que cabe", "Cocodrilooo", "Cocodrilooo"},

		{"vacío", "", ""},
		{"demasiado corto", "Ti", ""},
		{"no cabe en el papel", "Rinoceronteee", ""},
		// Este texto se imprime en el ticket del cliente y en la comanda. Sin el filtro, la
		// pantalla podría mandar cualquier cosa y saldría en papel con el nombre del negocio.
		{"con dígitos", "Tigre2", ""},
		{"con símbolos", "Tigre!", ""},
		{"con salto de línea", "Tigre\nMESA GRATIS", ""},
		{"con espacio en medio", "Tigre Blanco", ""},
		{"solo espacios", "     ", ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := SanitizarFolio(c.entra); got != c.quiere {
				t.Errorf("SanitizarFolio(%q) = %q, quiere %q", c.entra, got, c.quiere)
			}
		})
	}
}

func TestSiguienteFolioLibre(t *testing.T) {
	casos := []struct {
		nombre string
		base   string
		usados []string
		quiere string
	}{
		{"nadie lo ha usado", "Tigre", nil, "Tigre"},
		{"otros animales no estorban", "Tigre", []string{"Zorro", "Búho"}, "Tigre"},
		// Se conserva el animal en vez de saltar a otro: quien pidió ya oyó "Tigre", y cambiárselo
		// a media espera es peor que agregarle un número.
		{"ya se usó hoy", "Tigre", []string{"Tigre"}, "Tigre 2"},
		{"ya van dos vueltas", "Tigre", []string{"Tigre", "Tigre 2"}, "Tigre 3"},
		{"con un hueco en medio", "Tigre", []string{"Tigre", "Tigre 3"}, "Tigre 2"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := SiguienteFolioLibre(c.base, c.usados); got != c.quiere {
				t.Errorf("SiguienteFolioLibre(%q, %v) = %q, quiere %q", c.base, c.usados, got, c.quiere)
			}
		})
	}
}
