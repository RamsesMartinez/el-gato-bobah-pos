package domain

import (
	"fmt"
	"strings"
	"time"
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

// FolioNames devuelve la lista de nombres, para que la pantalla proponga uno al abrir la cuenta.
//
// La sirve el servidor y no una copia en el front: son la MISMA lista, y tenerla dos veces hacía
// que agregar un animal de un lado dejara al otro sin poder mostrarlo o sin poder repartirlo. Se
// pide una vez por carga de la aplicación —es estática dentro de un despliegue—, no por cuenta.
//
// Devuelve una copia: quien la reciba no puede reordenar la del servidor, de la que cuelga el
// barajado por día.
func FolioNames() []string {
	return append([]string(nil), animales[:]...)
}

// NombreDeFolio devuelve el nombre con el que se canta el pedido dailyNumber de ese día.
//
// El orden se revuelve por día y por empresa: si fuera secuencial, "Águila" sería siempre el
// primero de la mañana y el nombre acabaría significando "el temprano" en vez de identificar a
// UN pedido. Al agotarse la lista el nombre se repite con número —"Tigre 2"—, que sigue siendo
// más fácil de gritar y de recordar que "#187".
//
// Quien lo llame debe GUARDAR lo que regresa, no volver a calcularlo: este nombre se imprime en el
// ticket y el cliente lo usa para pedir su factura. Si algún día crece la lista, recalcularlo
// renombraría pedidos viejos y un ticket impreso como "Tigre" se reimprimiría como "Zorro".
func NombreDeFolio(companyID int64, fecha time.Time, dailyNumber int) string {
	if dailyNumber < 1 {
		return ""
	}
	n := len(animales)
	orden := ordenDelDia(companyID, fecha)
	nombre := animales[orden[(dailyNumber-1)%n]]

	if vuelta := (dailyNumber-1)/n + 1; vuelta > 1 {
		return fmt.Sprintf("%s %d", nombre, vuelta)
	}
	return nombre
}

// ordenDelDia baraja la lista de forma determinista a partir de la empresa y la fecha.
//
// El generador es propio y no math/rand a propósito: rand no promete el mismo resultado entre
// versiones de Go, y aunque el nombre se guarda —así que un cambio solo afectaría pedidos
// futuros—, un identificador que sale en papel no debería depender de con qué compilador se armó
// el binario. Son diez líneas contra una dependencia de comportamiento invisible.
func ordenDelDia(companyID int64, fecha time.Time) [len(animales)]int {
	y, m, d := fecha.Date()
	semilla := uint64(companyID)*1_000_000 + uint64(y)*10_000 + uint64(m)*100 + uint64(d)

	var orden [len(animales)]int
	for i := range orden {
		orden[i] = i
	}
	// Fisher-Yates hacia atrás: cada posición se cambia con una anterior o consigo misma, que es
	// lo que garantiza que toda permutación sea igual de probable y que ningún nombre se pierda.
	for i := len(orden) - 1; i > 0; i-- {
		j := int(siguiente(&semilla) % uint64(i+1))
		orden[i], orden[j] = orden[j], orden[i]
	}
	return orden
}

// siguiente es splitmix64: un paso, sin estado más allá del propio contador.
func siguiente(estado *uint64) uint64 {
	*estado += 0x9E3779B97F4A7C15
	z := *estado
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// maxFolio es el largo máximo de un nombre de folio. Sale del papel: a 46 px en un ticket de 58 mm
// caben unas nueve letras, y el sufijo de vuelta ("Tigre 10") suma tres más.
const maxFolio = 12

// SanitizarFolio limpia el nombre que propone la pantalla, o devuelve "" si no sirve.
//
// La pantalla propone el nombre al abrir la cuenta —así el operador lo ve desde el primer producto
// y no solo al cobrar— pero lo que se guarda pasa por aquí: ese texto se imprime en el ticket del
// cliente y en la comanda, así que no puede llevar lo que a alguien se le ocurra mandar.
//
// Valida FORMA y no pertenencia a la lista de animales a propósito: la pantalla tiene su propia
// copia de la lista, y exigir que coincidan haría que agregar un animal de un lado renombrara en
// silencio los pedidos del otro. Con la forma, una lista que se adelanta sigue funcionando.
func SanitizarFolio(propuesto string) string {
	limpio := strings.TrimSpace(propuesto)
	if n := utf8.RuneCountInString(limpio); n < 3 || n > maxFolio {
		return ""
	}
	for _, r := range limpio {
		if !unicode.IsLetter(r) {
			return ""
		}
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
