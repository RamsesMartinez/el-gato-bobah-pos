package domain

import (
	"fmt"
	"strings"
	"testing"
)

// Las tres reglas que hacen utilizable la lista. Viven como test y no como comentario porque el
// día que alguien agregue "Gallo" junto a "Ganso", o "Camarón" porque suena bonito, el comentario
// no lo detiene y esto sí.
func TestLasListasSeGritanSinConfusion(t *testing.T) {
	listas := []struct {
		nombre    string
		items     []string
		cuantos   int
		topeLargo int
	}{
		{"animales", animales[:], 100, 9},
		// Las razas no llevan tope de largo: "Colorpoint Shorthair" es el nombre real y recortarlo
		// la convertiría en otra raza. Lo que se ajusta es la comanda, no la lista. El tope de
		// SanitizarFolio (20) sigue aplicando y se verifica aparte.
		{"razas", razas[:], 88, 0},
	}
	for _, l := range listas {
		t.Run(l.nombre, func(t *testing.T) {
			if len(l.items) != l.cuantos {
				t.Fatalf("la lista tiene %d nombres, quiere %d", len(l.items), l.cuantos)
			}

			visto := map[string]bool{}
			for _, a := range l.items {
				if visto[a] {
					t.Errorf("%q está repetido", a)
				}
				visto[a] = true
			}

			if l.topeLargo > 0 {
				for _, a := range l.items {
					if n := len([]rune(a)); n > l.topeLargo {
						t.Errorf("%q tiene %d letras, el tope es %d", a, n, l.topeLargo)
					}
				}
			}
			// Todo nombre tiene que pasar por la puerta que valida lo que se imprime: uno que la
			// propia lista no puede proponer es un nombre muerto.
			for _, a := range l.items {
				if SanitizarFolio(a) != a {
					t.Errorf("%q no sobrevive a SanitizarFolio: la pantalla no podría proponerlo", a)
				}
			}

			// Dos nombres que empiezan igual son el mismo sonido en una cocina ruidosa, y eso
			// entrega el pedido equivocado. La primera sílaba es lo único que se alcanza a oír.
			porSilaba := map[string]string{}
			for _, a := range l.items {
				clave := sinAcentos(a)
				if len(clave) > 3 {
					clave = clave[:3]
				}
				if otro, choca := porSilaba[clave]; choca {
					t.Errorf("%q y %q empiezan igual (%q): se confunden al cantarlos", a, otro, clave)
				}
				porSilaba[clave] = a
			}
		})
	}
}

// El default es razas de gato, y lo es también para el negocio que no tiene fila de ajustes. Con
// dos fuentes de "cuál es el default", la pantalla mostraría un esquema y el ticket saldría con el
// otro.
func TestElEsquemaPorDefectoEsRazas(t *testing.T) {
	if EsquemaPorDefecto != EsquemaRazas {
		t.Errorf("el default es %q, quiere razas", EsquemaPorDefecto)
	}
	if !EsquemaValido("animales") || !EsquemaValido("razas") {
		t.Error("los dos esquemas del producto tienen que ser válidos")
	}
	// Un esquema que no se entiende NO cae al default en silencio: devolvería nombres de una lista
	// que nadie pidió, con cara de correcta.
	for _, malo := range []string{"", "Animales", "gatos", "perros", "razas "} {
		if EsquemaValido(malo) {
			t.Errorf("EsquemaValido(%q) = true", malo)
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
		{"el más largo que cabe", "Colorpoint Shorthair", "Colorpoint Shorthair"},
		// Las razas llevan espacio y guion en su nombre real. Sin esto, la mitad de la lista no
		// podría proponerse desde la pantalla.
		{"raza con espacio", "Maine Coon", "Maine Coon"},
		{"raza con guion", "Pixie-bob", "Pixie-bob"},
		{"raza de tres palabras", "Sagrado de Birmania", "Sagrado de Birmania"},

		{"vacío", "", ""},
		{"demasiado corto", "Ti", ""},
		{"no cabe en el papel", "Colorpoint Shorthairs", ""},
		// El separador en la orilla o duplicado ya no es un nombre, es relleno — y esto se imprime.
		{"guion al principio", "-Persa", ""},
		{"guion al final", "Persa-", ""},
		{"dos espacios seguidos", "Maine  Coon", ""},
		{"espacio y guion pegados", "Maine -Coon", ""},
		// Este texto se imprime en el ticket del cliente y en la comanda. Sin el filtro, la
		// pantalla podría mandar cualquier cosa y saldría en papel con el nombre del negocio.
		{"con dígitos", "Tigre2", ""},
		{"con símbolos", "Tigre!", ""},
		{"con salto de línea", "Tigre\nMESA GRATIS", ""},
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

// bolsa reproduce lo que hace el servidor con cada pedido: sortea, vacía si la vuelta se agotó, y
// anota lo que salió. Vive en el test porque es la secuencia que se quiere probar, no código de
// producción disfrazado.
type bolsa struct {
	lista      []string
	consumidos []string
	vaciadas   int
}

func (b *bolsa) tomar(usadosHoy []string, azar func(int) int) string {
	n, vaciar := SiguienteDeLaBolsa(b.lista, b.consumidos, usadosHoy, azar)
	if vaciar {
		b.consumidos = nil
		b.vaciadas++
	}
	b.consumidos = append(b.consumidos, n)
	return n
}

func listaDe(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("N%03d", i)
	}
	return out
}

// primero es el sorteo más aburrido posible: siempre el primer disponible. Hace la secuencia
// predecible, que es lo que permite afirmar sobre ella en vez de sobre la suerte.
func primero(int) int { return 0 }

// LA BOLSA SE AGOTA ANTES DE VOLVER A EMPEZAR — los tres días del reporte.
//
// Sin esto, el sorteo con reemplazo repite nombres a los pocos días mientras media lista nunca sale:
// con 300 nombres y 100 pedidos diarios, la probabilidad de que un nombre salga dos veces EL MISMO
// día ya es alta, y "Alce 2" con el primer Alce todavía en la plancha es cómo se entrega el pedido
// equivocado.
func TestLaBolsaSeAgotaAntesDeVolverAEmpezar(t *testing.T) {
	b := &bolsa{lista: listaDe(300)}

	dia := func(cuantos int) []string {
		t.Helper()
		var hoy []string
		for i := 0; i < cuantos; i++ {
			hoy = append(hoy, b.tomar(hoy, primero))
		}
		return hoy
	}

	dia1 := dia(100)
	if len(b.consumidos) != 100 {
		t.Fatalf("tras el día 1 la bolsa lleva %d consumidos, quiere 100", len(b.consumidos))
	}
	if b.vaciadas != 0 {
		t.Fatalf("la bolsa se vació %d veces el día 1: quedaban 200 nombres sin usar", b.vaciadas)
	}

	// Día 2: 190 de los 200 que quedaban. Ninguno puede repetir uno del día 1 — esa es la promesa.
	dia2 := dia(190)
	if b.vaciadas != 0 {
		t.Fatalf("la bolsa se vació el día 2: todavía quedaban 10 nombres sin usar")
	}
	vistos := map[string]int{}
	for _, n := range dia1 {
		vistos[n] = 1
	}
	for _, n := range dia2 {
		if vistos[n] != 0 {
			t.Fatalf("%q salió el día 1 y otra vez el día 2, con la bolsa a medias", n)
		}
		vistos[n] = 2
	}
	if len(vistos) != 290 {
		t.Fatalf("en dos días salieron %d nombres distintos, quiere 290", len(vistos))
	}

	// Día 3: se piden 100. Los 10 que quedaban salen primero y ahí la bolsa se vacía UNA vez.
	dia3 := dia(100)
	if b.vaciadas != 1 {
		t.Fatalf("la bolsa se vació %d veces el día 3, quiere exactamente 1", b.vaciadas)
	}
	for _, n := range dia3[:10] {
		if vistos[n] != 0 {
			t.Errorf("%q ya había salido antes de que la bolsa se vaciara", n)
		}
	}
	if len(vistos)+10 != 300 {
		t.Fatalf("antes de vaciar habían salido %d nombres, quiere 300", len(vistos)+10)
	}

	// Y NINGUNO de los 100 del día 3 se repite entre sí: la vuelta nueva descuenta lo ya cantado hoy.
	// Sin eso, el pedido 11 podría llamarse igual que el 1, con los dos todavía en la plancha.
	delDia := map[string]bool{}
	for i, n := range dia3 {
		if delDia[n] {
			t.Fatalf("%q se repitió el mismo día (pedido %d): habría que cantarlo como \"%s 2\"", n, i+1, n)
		}
		delDia[n] = true
	}
}

// La bolsa NO se vacía mientras quede un solo nombre. Es la diferencia entre "se agotan los 300" y
// "se agotan casi todos": el que sobra nunca saldría.
func TestConUnNombreSinUsarLaBolsaNoSeVacia(t *testing.T) {
	lista := listaDe(5)
	_, vaciar := SiguienteDeLaBolsa(lista, lista[:4], nil, primero)
	if vaciar {
		t.Error("se vació con un nombre todavía sin usar")
	}
	n, vaciar := SiguienteDeLaBolsa(lista, lista, nil, primero)
	if !vaciar {
		t.Error("con todos consumidos tiene que vaciarse y empezar otra vuelta")
	}
	if n == "" {
		t.Error("un pedido sin nombre no es una opción: es con lo que cocina lo canta")
	}
}

// EL CASO QUE EL DUEÑO PIDIÓ VALIDAR: al empezar la vuelta nueva, no tomar uno que ya salió hoy.
//
// Es poco probable pero pasa, y el resultado es "Alce 2" con el primer Alce todavía esperando. Con
// tres nombres y dos ya cantados hoy, solo hay una respuesta correcta.
func TestAlEmpezarLaVueltaNoSeRepiteLoQueYaSalioHoy(t *testing.T) {
	lista := []string{"Alce", "Bisonte", "Castor"}
	// Bolsa agotada; hoy ya se cantaron Alce y Bisonte.
	n, vaciar := SiguienteDeLaBolsa(lista, lista, []string{"Alce", "Bisonte"}, primero)
	if !vaciar {
		t.Error("la bolsa estaba agotada: tenía que vaciarse")
	}
	if n != "Castor" {
		t.Errorf("salió %q, quiere Castor: los otros dos ya se cantaron hoy y sonarían como \"Alce 2\"", n)
	}
}

// Cuando el día pasó del largo de la lista ya no hay nada fresco que dar, y un pedido sin nombre no
// es una opción. Devuelve uno repetido a propósito: quien llama le pone el número.
func TestCuandoElDiaSuperaALaListaSeDevuelveUnoRepetido(t *testing.T) {
	lista := []string{"Alce", "Bisonte"}
	n, _ := SiguienteDeLaBolsa(lista, lista, lista, primero)
	if n == "" {
		t.Fatal("devolvió vacío: el pedido se quedaría sin nombre con el que cantarlo")
	}
	if libre := SiguienteFolioLibre(n, lista); libre != n+" 2" {
		t.Errorf("el nombre repetido se numera como %q, quiere %q", libre, n+" 2")
	}
}

// Un sorteo mal escrito no puede tumbar la venta. `azar` lo inyecta quien llama, y un índice fuera
// de rango aquí sería un panic en la ruta que crea el pedido.
func TestUnSorteoFueraDeRangoNoTumbaLaVenta(t *testing.T) {
	lista := listaDe(3)
	for _, malo := range []func(int) int{
		func(int) int { return -1 },
		func(n int) int { return n },
		func(n int) int { return n * 1000 },
	} {
		if n, _ := SiguienteDeLaBolsa(lista, nil, nil, malo); n == "" {
			t.Error("un sorteo fuera de rango dejó el pedido sin nombre")
		}
	}
}
