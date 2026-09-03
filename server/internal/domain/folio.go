package domain

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// animales son los nombres con los que se canta un pedido en cocina, en lugar de su número.
//
// La lista se eligió por CÓMO SUENAN, no por cuántos son. Tres reglas la gobiernan, y las tres
// las verifica folio_test.go para que nadie agregue un nombre que las rompa:
//
//   - Ningún par comparte las primeras tres letras. "Gato" y "Gallo" gritados en una cocina con
//     freidora encendida son el mismo sonido, y el pedido se entrega mal.
//   - Ninguno es comida ni aparece en una carta. "¡Pavo!" o "¡Pulpo!" se confunden con lo que se
//     está preparando; por eso también quedaron fuera las frutas cuando se pensó la lista.
//   - Ninguno pasa de nueve letras, para que quepa grande en el ticket y se diga de un tirón.
//
// "Gato" quedó fuera aparte: el negocio se llama El Gato Bobah y cantarlo sería un choque diario.
var animales = [...]string{
	"Águila", "Alce", "Ardilla", "Avestruz", "Ballena", "Bisonte", "Búfalo", "Búho", "Burro",
	"Caimán", "Camello", "Canguro", "Castor", "Cebra", "Chita", "Cisne", "Colibrí", "Coyote",
	"Cuervo", "Delfín", "Dingo", "Dragón", "Erizo", "Foca", "Gacela", "Ganso", "Gaviota",
	"Gorila", "Halcón", "Hiena", "Iguana", "Jabalí", "Jaguar", "Jirafa", "Koala", "Lagarto",
	"León", "Libélula", "Lince", "Llama", "Lobo", "Loro", "Lechuza", "Mamut", "Mandril",
	"Mapache", "Mono", "Morsa", "Mula", "Marmota", "Nutria", "Ocelote", "Orca", "Oso",
	"Panda", "Pelícano", "Pingüino", "Puma", "Quetzal", "Reno", "Sapo", "Suricata", "Tapir",
	"Tejón", "Tiburón", "Tigre", "Topo", "Tortuga", "Tucán", "Urraca", "Vicuña", "Zorro",
	"Alpaca", "Antílope", "Armadillo", "Cachalote", "Comadreja", "Elefante", "Flamenco",
	"Garza", "Guacamaya", "Jilguero", "Lémur", "Paloma", "Petirrojo", "Capibara", "Cocodrilo",
	"Perico", "Ajolote", "Coatí", "Grulla", "Hurón", "Mirlo", "Cigüeña", "Gecko", "Impala",
	"Emú", "Yegua", "Wallaby", "Orangután",
}

// razas son los nombres del otro esquema: razas de gato reconocidas, con su nombre real.
//
// Es el DEFAULT de todo negocio, nuevo o existente. Se rigen por las mismas dos reglas de sonido
// que los animales —ningún par comparte las primeras tres letras, ninguno es comida— y las verifica
// folio_test.go. La tercera regla, el largo, aquí es distinta: son nombres propios de raza y
// acortarlos los convertiría en otra cosa ("Colorpoint" no es "Colorpoint Shorthair"), así que la
// comanda baja el tamaño de letra en vez de que la lista invente nombres.
//
// De 104 razas reconocidas por TICA/CFA/FIFe/WCF quedaron fuera 16: catorce por chocar de prefijo
// con otra ya listada (Burmilla con Burmes, Ragdoll con Ragamuffin, American Curl con American
// Bobtail) y dos por no ser razas distintas (Nibelung es Nebelung; Chinchilla es un color del
// Persa).
var razas = [...]string{
	"Abisinio", "Aegean", "American Bobtail", "Anatolio", "Angora Turco", "Aphrodite", "Asiático",
	"Australian Mist", "Azul Ruso", "Balinés", "Bambino", "Bengalí", "Birmano", "Bobtail Japonés",
	"Bombay", "Bosque de Noruega", "Brasileño", "British Shorthair", "Burmés", "Californiano",
	"Ceylon", "Chartreux", "Cheetoh", "Colorpoint Shorthair", "Cornish Rex", "Cymric", "Cyprus",
	"Devon Rex", "Don Sphynx", "Dragon Li", "Europeo", "Exótico", "Foldex", "Genetta", "German Rex",
	"Habana", "Highlander", "Himalayo", "Javanés", "Kanaani", "Khao Manee", "Kinkalow", "Korat",
	"Kurilian Bobtail", "LaPerm", "Lambkin", "Levkoy Ucraniano", "Lykoi", "Maine Coon", "Manx",
	"Mau Egipcio", "Mekong Bobtail", "Minskin", "Munchkin", "Napoleón", "Nebelung",
	"Neva Masquerade", "Ocicat", "Ojos Azules", "Oregon Rex", "Oriental", "Persa", "Peterbald",
	"Pixie-bob", "Ragamuffin", "Sagrado de Birmania", "Sam Sawet", "Savannah", "Scottish Fold",
	"Selkirk Rex", "Serengeti", "Siamés", "Siberiano", "Singapura", "Skookum", "Snowshoe", "Sokoke",
	"Somalí", "Sphynx", "Suphalak", "Tennessee Rex", "Thai", "Tonkinés", "Toyger", "Ural Rex",
	"Ussuri", "Van Turco", "York Chocolate",
}

// EsquemaDeFolio dice con qué se nombran los pedidos de un negocio.
type EsquemaDeFolio string

const (
	EsquemaAnimales EsquemaDeFolio = "animales"
	EsquemaRazas    EsquemaDeFolio = "razas"

	// EsquemaPorDefecto es con lo que nace todo negocio. Vive aquí y no solo en el default de la
	// columna: un negocio sin fila de ajustes tiene que caer al MISMO valor que uno que sí la
	// tiene, o la pantalla mostraría un esquema y el ticket saldría con el otro.
	EsquemaPorDefecto = EsquemaRazas
)

// EsquemaValido rechaza cualquier otro valor. Un esquema que no se entiende NO cae al default en
// silencio: devolvería nombres de una lista que nadie pidió y nada lo delataría.
func EsquemaValido(e string) bool {
	return EsquemaDeFolio(e) == EsquemaAnimales || EsquemaDeFolio(e) == EsquemaRazas
}

// NombresDelEsquema devuelve la lista completa del esquema, en su orden de declaración.
//
// Devuelve una copia: quien la reciba no puede reordenar la del servidor, de la que cuelga el
// sorteo de la bolsa.
func NombresDelEsquema(e EsquemaDeFolio) []string {
	if e == EsquemaAnimales {
		return append([]string(nil), animales[:]...)
	}
	return append([]string(nil), razas[:]...)
}

// SiguienteDeLaBolsa elige el nombre del próximo pedido y dice si hay que vaciar la bolsa antes.
//
// LA BOLSA SE AGOTA ANTES DE REPETIR. `consumidos` son los nombres que ya salieron en la vuelta en
// curso; mientras quede uno sin salir, el sorteo es entre esos. Solo cuando no queda ninguno se
// vacía y empieza otra vuelta — es lo que impide que el mismo nombre salga dos veces en semanas
// mientras la mitad de la lista nunca se usa.
//
// `usadosHoy` es lo que ya se cantó HOY, y se descuenta aparte. Importa justo en la vuelta nueva:
// al vaciar la bolsa vuelven a estar disponibles nombres que ya salieron hoy, y tomar uno de esos
// obligaría a cantar "Alce 2" con el primer Alce todavía en la plancha.
//
// Si todo lo disponible ya salió hoy —el día pasó del largo de la lista— devuelve uno igual: quien
// llama le pone el número con SiguienteFolioLibre. Un pedido SIN nombre no es una opción, es con lo
// que cocina lo canta.
//
// `azar(n)` devuelve un índice en [0,n). Se inyecta para que el test pueda fijar qué sale: con
// math/rand adentro, "no repite en 300" sería una prueba de suerte y no del código.
func SiguienteDeLaBolsa(lista, consumidos, usadosHoy []string, azar func(n int) int) (string, bool) {
	opciones, vaciar := DisponiblesDeLaBolsa(lista, consumidos, usadosHoy)
	if len(opciones) == 0 {
		return "", false
	}
	return opciones[acotado(azar, len(opciones))], vaciar
}

// DisponiblesDeLaBolsa devuelve los nombres entre los que se sortea el próximo pedido, y si para
// llegar a ellos hay que vaciar la bolsa antes.
//
// Es UN solo predicado y lo usan los dos lados: de aquí sortea el servidor y de aquí propone la
// pantalla. Con dos, la pantalla ofrecería nombres que el servidor no acepta y el operador vería
// cambiar el que ya le dijo al cliente. Consultarla no vacía nada: eso lo hace quien crea el
// pedido, con el `bool` que devuelve.
func DisponiblesDeLaBolsa(lista, consumidos, usadosHoy []string) ([]string, bool) {
	if len(lista) == 0 {
		return nil, false
	}
	fuera := aConjunto(consumidos)
	hoy := aConjunto(usadosHoy)

	disponibles := make([]string, 0, len(lista))
	for _, n := range lista {
		if !fuera[n] {
			disponibles = append(disponibles, n)
		}
	}
	vaciar := false
	if len(disponibles) == 0 {
		vaciar = true
		disponibles = append(disponibles, lista...)
	}

	frescos := make([]string, 0, len(disponibles))
	for _, n := range disponibles {
		if !hoy[n] {
			frescos = append(frescos, n)
		}
	}
	if len(frescos) > 0 {
		return frescos, vaciar
	}
	// Todo lo disponible ya se cantó hoy: el día pasó del largo de la lista. Se devuelven igual y
	// quien llama les pone el número — un pedido sin nombre no es una opción.
	return disponibles, vaciar
}

func aConjunto(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// acotado protege del `azar` que devuelve fuera de rango. No es paranoia de librería: lo inyecta
// quien llama, y un índice negativo aquí es un panic en la ruta que crea el pedido — el POS
// dejaría de poder vender por un generador mal escrito.
func acotado(azar func(n int) int, n int) int {
	i := azar(n)
	if i < 0 || i >= n {
		return 0
	}
	return i
}

// maxFolio es el largo máximo de un nombre de folio.
//
// Sube de 12 a 20 por las razas: "Colorpoint Shorthair" es el nombre real de la raza y recortarlo
// la convertiría en otra. Lo que se ajusta es el papel — la comanda baja el tamaño de letra según
// la palabra más larga— y no la lista. El sufijo de vuelta ("Persa 2") se agrega DESPUÉS de este
// tope, así que no cuenta aquí.
const maxFolio = 20

// SanitizarFolio limpia el nombre que propone la pantalla, o devuelve "" si no sirve.
//
// La pantalla propone el nombre al abrir la cuenta —así el operador lo ve desde el primer producto
// y no solo al cobrar— pero lo que se guarda pasa por aquí: ese texto se imprime en el ticket del
// cliente y en la comanda, así que no puede llevar lo que a alguien se le ocurra mandar.
//
// Valida FORMA y no pertenencia a la lista a propósito: la pantalla tiene su propia copia, y exigir
// que coincidan haría que agregar un nombre de un lado renombrara en silencio los pedidos del otro.
// Con la forma, una lista que se adelanta sigue funcionando.
//
// Acepta espacio y guion INTERIORES porque los nombres de raza los llevan: "Maine Coon",
// "Pixie-bob", "Sagrado de Birmania". No al principio ni al final, ni dos seguidos: eso ya no es un
// nombre, es un texto con relleno, y este texto se imprime en el ticket del cliente.
func SanitizarFolio(propuesto string) string {
	limpio := strings.TrimSpace(propuesto)
	if n := utf8.RuneCountInString(limpio); n < 3 || n > maxFolio {
		return ""
	}
	var previo rune
	for i, r := range limpio {
		separador := r == ' ' || r == '-'
		if !unicode.IsLetter(r) && !separador {
			return ""
		}
		// El TrimSpace ya quitó los espacios de las orillas; el guion no, y "-Persa" no es un nombre.
		if separador && (i == 0 || previo == ' ' || previo == '-') {
			return ""
		}
		previo = r
	}
	if previo == ' ' || previo == '-' {
		return ""
	}
	return limpio
}

// SiguienteFolioLibre devuelve el nombre propuesto, o con su número de vuelta si ya se usó hoy.
//
// Es la misma regla que cuando el servidor reparte los nombres: al repetirse, el nombre lleva
// número ("Tigre 2"). Se conserva el animal en vez de saltar a otro porque quien pidió ya oyó
// "Tigre", y cambiárselo por "Zorro" a media espera es peor que agregarle un número.
func SiguienteFolioLibre(base string, usadosHoy []string) string {
	usados := make(map[string]bool, len(usadosHoy))
	for _, u := range usadosHoy {
		usados[u] = true
	}
	if !usados[base] {
		return base
	}
	// El tope no es defensivo: con 100 animales, llegar a la vuelta 99 del MISMO animal exigiría
	// ~9,900 pedidos en un día. Si pasa, gana el nombre con número alto antes que un ciclo infinito.
	for vuelta := 2; vuelta < 100; vuelta++ {
		cand := fmt.Sprintf("%s %d", base, vuelta)
		if !usados[cand] {
			return cand
		}
	}
	return ""
}
