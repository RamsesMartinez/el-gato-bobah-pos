package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// LA MEDICIÓN DE USO (spec 017). Un endpoint de una sola dirección: entra y no contesta nada.
//
// Todo lo de aquí está escrito para que el operador no se entere de que existe. Si algo falla
// —cuerpo roto, base caída, nombres que no están en la lista— la respuesta es la misma: 204.

// RegistrarUso: POST /api/v1/usage
func (h *Handlers) RegistrarUso(w http.ResponseWriter, r *http.Request) {
	// Se responde 204 SIEMPRE y desde el primer renglón del camino de error. El cliente manda esto
	// sin esperar la respuesta: un 400 no lo ayudaría a nada y solo serviría para que alguien lo vea
	// en la pestaña de red y crea que el POS está roto.
	sinContenido := func() { w.WriteHeader(http.StatusNoContent) }

	u, ok := userFrom(r.Context())
	if !ok {
		sinContenido()
		return
	}

	// El tope por usuario va ANTES de leer el cuerpo: un bucle en el front no puede costar ni una
	// deserialización. Y muerde en silencio, como todo lo demás aquí.
	//
	// `recordAndOver` y no `blocked`+`record`: los dos pasos dejan un hueco por donde 300 peticiones
	// simultáneas con el mismo token pasan todas —leen el contador antes de que aterrice el primer
	// incremento— y el tope promete un número que no cumple. Además falla CERRADO si Redis está
	// caído: perder mediciones no le cuesta nada a nadie, quedarse sin tope sí.
	if h.usoIngesta.recordAndOver(r.Context(), "uso:"+itoa64(u.ID), usoMax) {
		sinContenido()
		return
	}

	var body struct {
		Eventos []domain.EventoDeUso `json:"eventos"`
		// Los toques por zona (spec 019). Viajan en el MISMO request que las aperturas: es un solo
		// vaciado de cola cada diez segundos, no dos.
		Toques []domain.Toque `json:"toques"`
	}
	// Sin `Decode` del paquete: ése rechaza campos desconocidos y devuelve error, y aquí un cuerpo
	// raro no es un error sino algo que se tira. Lo que NO se decodifica es tan importante como lo
	// que sí: un `x` y un `y` con precisión de píxel no tienen campo en `domain.Toque` ni columna
	// en la tabla, así que un cliente —viejo, modificado o un `curl`— no puede hacerlos existir en
	// el servidor. La promesa no es que nadie los mande; es que no hay dónde ponerlos.
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		sinContenido()
		return
	}
	if len(body.Eventos) == 0 && len(body.Toques) == 0 {
		sinContenido()
		return
	}

	// EL ROL SALE DEL TOKEN, no del cuerpo. Dejar que el cliente diga de qué rol es convertiría el
	// corte por rol en algo que cualquiera puede inventar, y con él la única dimensión que esta
	// feature promete medir bien.
	descartados := 0
	if len(body.Eventos) > 0 {
		n, err := h.usage.Registrar(r.Context(), u.Role, body.Eventos)
		if err != nil {
			// El operador no se entera; quien opera el sistema sí, en el log.
			slog.Warn("uso no registrado", "error", err, "eventos", len(body.Eventos))
			sinContenido()
			return
		}
		descartados = n
	}

	// Los toques van en su propia escritura y NO comparten el camino de error con las aperturas: si
	// una falla, la otra ya se guardó. Las dos se pueden perder sin consecuencia —es una medición—
	// pero perder las dos porque falló una sería tirar el doble por nada.
	if len(body.Toques) > 0 {
		n, err := h.usage.RegistrarToques(r.Context(), u.Role, body.Toques)
		if err != nil {
			slog.Warn("toques no registrados", "error", err, "toques", len(body.Toques))
			sinContenido()
			return
		}
		if n > 0 {
			// El mismo testigo que abajo, con su propio nombre: una versión del front que quedó
			// midiendo una pantalla que el servidor ya no instrumenta deja la rejilla vacía, y una
			// rejilla vacía se lee como «aquí nadie toca».
			slog.Warn("toques_descartados", "descartados", n, "del_lote", len(body.Toques))
		}
	}

	if descartados > 0 {
		// EL ÚNICO TESTIGO de que una versión del front dejó de medir. Sin esta línea, el mapa
		// simplemente muestra menos — que es indistinguible de «se usó menos».
		slog.Warn("usage_descartado",
			"descartados", descartados,
			"del_lote", len(body.Eventos),
			"primer_desconocido", primerDesconocido(body.Eventos))
	}
	sinContenido()
}

// primerDesconocido nombra el primer evento que no pasó la lista blanca, que es lo que dice qué hay
// que arreglar. Solo uno: el resto del lote suele ser el mismo nombre repetido.
func primerDesconocido(eventos []domain.EventoDeUso) string {
	for _, e := range eventos {
		if !domain.EventoDeUsoValido(e.Pantalla, e.Accion) {
			nombre := e.Pantalla
			if e.Accion != "" {
				nombre += "/" + e.Accion
			}
			return recortar(nombre, 80)
		}
	}
	return ""
}

// recortar acota lo que va al log: el nombre viene del cuerpo del request, y un log con un
// megabyte adentro se lleva por delante la rotación y con ella la bitácora de todo lo demás.
func recortar(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// itoa64 evita traer strconv a este archivo para un solo uso; el id siempre es positivo.
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
