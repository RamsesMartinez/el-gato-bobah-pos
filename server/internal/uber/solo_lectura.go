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
)

// NombreDeLaPlataforma aparece en los errores del guardia. Está aquí y no interpolado en cada
// mensaje porque el día que exista un paquete gemelo para DiDi, el error tiene que decir cuál de
// las dos se intentó tocar.
const NombreDeLaPlataforma = "Uber Eats"

// soloLectura envuelve un RoundTripper y **rechaza todo verbo distinto de GET antes de abrir el
// socket**.
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
		if r.Method != http.MethodGet {
			return nil, fmt.Errorf(
				"%s: operación %s bloqueada — este cliente es de solo lectura y %s reemplaza el menú publicado completo",
				NombreDeLaPlataforma, r.Method, r.Method)
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
