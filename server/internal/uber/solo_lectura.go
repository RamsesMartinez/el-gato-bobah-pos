// Package uber lee el menú publicado de una tienda en Uber Eats. **Solo lee**: no existe en este
// paquete ninguna operación que cree, modifique, borre, publique o suspenda nada en la plataforma,
// y eso está garantizado por el transporte, no por una convención.
//
// Sigue el precedente de internal/hibp: el conocimiento de una API ajena vive en un paquete propio,
// sin base de datos ni lógica de negocio, para que no se mezcle con `app` (principio I).
package uber

import (
	"fmt"
	"net/http"
	"strings"
)

// NombreDeLaPlataforma aparece en los errores del guardia. Está aquí y no interpolado en cada
// mensaje porque el día que exista un paquete gemelo para DiDi, el error tiene que decir cuál de
// las dos se intentó tocar.
const NombreDeLaPlataforma = "Uber Eats"

// escriturasPermitidas es la lista blanca de rutas donde este paquete SÍ puede escribir.
//
// Son dos, y son las dos decisiones sobre un pedido que llegó: aceptarlo y rechazarlo. Sin ellas no
// hay integración —un pedido que no se puede aceptar no sirve— y con cualquier otra, sí hay riesgo:
// el `PUT` de menú reemplaza el menú publicado completo y el `DELETE` de `pos_data` desconecta la
// integración, y ninguno de los dos se deshace.
//
// Se comparan como SUFIJO de la ruta y no como subcadena: `strings.Contains` dejaría pasar
// `/menus?x=accept_pos_order` y cualquier otra cosa que lleve el texto adentro.
var escriturasPermitidas = []string{
	"/accept_pos_order",
	"/deny_pos_order",
}

// soloLectura envuelve un RoundTripper y **rechaza todo verbo distinto de GET antes de abrir el
// socket**, salvo las rutas de escriturasPermitidas.
//
// POR QUÉ EN EL TRANSPORTE Y NO EN UNA REVISIÓN. El `PUT` de menú de Uber es reemplazo total
// —«overwrites any existing menus»— así que el primer disparo contra la tienda viva sustituye el
// menú publicado por lo que vaya en el cuerpo, sin deshacer. Una regla que vive en la cabeza de
// quien revisa se rompe el día que alguien agrega un método «solo para probar»; ésta no se puede
// evadir sin borrarla a propósito, y borrarla rompe dos pruebas.
//
// Cubre también el escenario que más cuesta imaginar: aunque Uber conceda a la credencial permisos
// de escritura, el sistema no ejerce ninguno.
func soloLectura(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet && !escrituraPermitida(r) {
			return nil, fmt.Errorf(
				"%s: operación %s sobre %s bloqueada — este cliente solo escribe para aceptar o rechazar un pedido, y un %s de menú reemplaza el menú publicado completo",
				NombreDeLaPlataforma, r.Method, r.URL.Path, r.Method)
		}
		return base.RoundTrip(r)
	})
}

// soloElHostDeToken permite POST, pero **únicamente contra el host de autenticación**.
//
// El token necesita un POST y por eso viaja por otro cliente. Sin esta restricción, ese segundo
// cliente sería la puerta trasera del guardia de arriba: bastaría usarlo para mandar un PUT al host
// de menú. La comparación es por host exacto, no por prefijo.
func soloElHostDeToken(host string, base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != host {
			return nil, fmt.Errorf(
				"%s: el cliente de token solo habla con %s, se intentó %s contra %s",
				NombreDeLaPlataforma, host, r.Method, r.URL.Host)
		}
		return base.RoundTrip(r)
	})
}

// escrituraPermitida exige POST y que la ruta TERMINE en una de las dos permitidas. El método se
// verifica también: un PUT a `/accept_pos_order` no es nada que la plataforma acepte, y dejarlo
// pasar sería ensanchar la puerta sin ganar nada.
func escrituraPermitida(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	for _, ruta := range escriturasPermitidas {
		if strings.HasSuffix(r.URL.Path, ruta) {
			return true
		}
	}
	return false
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
