package domain

import "sort"

// DÓNDE CAE EL DEDO (spec 019). Lo puro: qué se puede contar y qué forma tiene una celda.
//
// La rejilla vive AQUÍ y no en la base a propósito: cambiar la resolución tiene que verse en un
// diff. Y ojo con la dirección —hacia una rejilla más gruesa se recalcula fusionando celdas; hacia
// una más fina NO se puede, porque el toque fino no se guarda en ningún lado. Esa es la decisión
// central de esta feature, no un efecto secundario.

// ColumnasDeLaRejilla y FilasDeLaRejilla, en horizontal. En vertical se invierten.
//
// En una tableta de 1024×600 son zonas de unos 85×86 px: el tamaño de un botón del POS. Es la
// resolución en la que la pregunta «¿esta parte se usa?» tiene respuesta y la pregunta «¿quién
// tocó?» no.
const (
	ColumnasDeLaRejilla = 12
	FilasDeLaRejilla    = 7
	CeldasDeLaRejilla   = ColumnasDeLaRejilla * FilasDeLaRejilla
)

// Orientación de la pantalla. Las dos NO se mezclan: la misma celda es otro lugar en cada forma.
const (
	OrientacionHorizontal = "horizontal"
	OrientacionVertical   = "vertical"
)

// Toque es lo que llega de la tableta: una celda, ya redondeada allá.
//
// **No hay coordenada.** Si viajara `(x, y)` con precisión de píxel, el dato fino existiría en el
// cuerpo del request, en el log de un proxy y en la memoria del servidor aunque después se
// redondeara. Redondear en el origen es lo único que hace que el punto exacto no exista.
type Toque struct {
	Pantalla    string
	Celda       int
	Orientacion string
}

// pantallasConToque es la lista CORTA de pantallas instrumentadas (FR-015).
//
// Se empieza por el POS, que es donde el dedo está todo el día. Medir doce pantallas para mirar dos
// es volumen y ruido a cambio de nada, y la 017 es la que va a decir cuál agregar.
//
// **Tiene que ser subconjunto de `pantallasMedibles`**: el toque entra por el mismo endpoint y se
// valida contra aquella lista, así que una pantalla que no esté allá tendría todos sus toques
// descartados en silencio y su rejilla saldría vacía sin un solo error. Lo vigila
// TestLasPantallasConToqueSonSubconjunto.
var pantallasConToque = map[string]struct{}{
	"pos": {},
}

// PantallasConToque devuelve la lista instrumentada, para el test de subconjunto y para el front.
func PantallasConToque() []string {
	nombres := make([]string, 0, len(pantallasConToque))
	for n := range pantallasConToque {
		nombres = append(nombres, n)
	}
	sort.Strings(nombres)
	return nombres
}

// ToqueValido dice si ese toque se puede contar.
//
// Valida las tres cosas por separado a propósito: una pantalla no instrumentada, una celda fuera de
// la rejilla y una orientación inventada son tres errores distintos, y los tres terminan igual —se
// descarta— pero por razones que conviene poder distinguir en el log.
func ToqueValido(t Toque) bool {
	if _, ok := pantallasConToque[t.Pantalla]; !ok {
		return false
	}
	if t.Celda < 0 || t.Celda >= CeldasDeLaRejilla {
		return false
	}
	return t.Orientacion == OrientacionHorizontal || t.Orientacion == OrientacionVertical
}

// RejillaDe devuelve cuántas columnas y filas tiene la rejilla en esa orientación.
func RejillaDe(orientacion string) (columnas, filas int) {
	if orientacion == OrientacionVertical {
		return FilasDeLaRejilla, ColumnasDeLaRejilla
	}
	return ColumnasDeLaRejilla, FilasDeLaRejilla
}

// MaxToquesPorLote acota lo que una tableta puede mandar de un golpe.
//
// Es el mismo número que el de las aperturas y no por simetría: el tope real lo pone la COLA del
// cliente, que guarda 50 mientras hay un envío en vuelo y se vacía a los 20 o a los diez segundos.
// Un lote más grande que eso no lo produce el POS —solo un bucle o un `curl`—, y lo que este tope
// protege es el camino: la llave del limitador y la línea de log se arman con lo que llega. La base
// ya está acotada por la lista blanca y por los `check` de las columnas.
const MaxToquesPorLote = MaxEventosPorLote

// ToqueAgregado es una zona del día con cuántas veces se tocó.
type ToqueAgregado struct {
	Pantalla    string
	Orientacion string
	Celda       int
	Veces       int
}

// PreAgregarToques agrupa el lote por (pantalla, orientación, celda) y cuenta.
//
// Es lo que convierte mil toques en un renglón, y no es una optimización cosmética: sin esto cada
// toque sería un `update` que deja muerta la versión vieja de la fila, y estas filas son pocas y
// calientes. Medido en la 017 sobre el esquema real: 135 filas con 2,000 incrementos de a uno pasan
// de 64 kB a 232 kB antes de que autovacuum llegue.
//
// Descarta lo que no está instrumentado Y lo que ese rol no podría haber abierto.
func PreAgregarToques(lote []Toque, rol Role) []ToqueAgregado {
	if len(lote) == 0 {
		return nil
	}
	type llave struct {
		pantalla, orientacion string
		celda                 int
	}
	// El orden de aparición, no el de un map: sin él dos lotes iguales producen los `update` en
	// orden distinto y dos transacciones concurrentes pueden interbloquearse.
	orden := make([]llave, 0, len(lote))
	veces := make(map[llave]int, len(lote))
	for _, t := range lote {
		if !ToqueValido(t) || !PantallaPermitidaParaRol(t.Pantalla, rol) {
			continue
		}
		k := llave{t.Pantalla, t.Orientacion, t.Celda}
		if _, visto := veces[k]; !visto {
			orden = append(orden, k)
		}
		veces[k]++
	}
	agregado := make([]ToqueAgregado, 0, len(orden))
	for _, k := range orden {
		agregado = append(agregado, ToqueAgregado{
			Pantalla: k.pantalla, Orientacion: k.orientacion, Celda: k.celda, Veces: veces[k],
		})
	}
	return agregado
}

// RecortarLoteDeToques deja el lote en el tope. Lo que sobra se pierde, que es lo que esta feature
// tiene permitido hacer con una medición.
func RecortarLoteDeToques(lote []Toque) []Toque {
	if len(lote) <= MaxToquesPorLote {
		return lote
	}
	return lote[:MaxToquesPorLote]
}
