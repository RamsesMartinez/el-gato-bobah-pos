package domain

import (
	"fmt"
	"time"
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
