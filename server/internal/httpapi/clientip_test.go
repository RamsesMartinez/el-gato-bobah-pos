package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// LA IP DE LA BITÁCORA ES LA MISMA QUE LA DEL LIMITADOR: LA ÚLTIMA DEL X-Forwarded-For.
//
// Caddy AGREGA el peer real al final de la cabecera, así que con un solo proxy de confianza esa es
// la única entrada que el cliente no puede falsear — lo que el cliente mande queda a su izquierda.
// `rateKeyIP` ya lo hacía; `clientIP`, que es la que va a los eventos de seguridad, tomaba la
// PRIMERA: `curl -H 'X-Forwarded-For: 8.8.8.8'` dejaba el intento fallido registrado con la IP que
// el atacante quisiera.
//
// El throttle nunca se pudo evadir por ahí. Lo que se perdía era la bitácora, que en un subdominio
// público es lo único que queda de un intento.
func TestLaIPDelEventoNoLaEligeElCliente(t *testing.T) {
	casos := []struct {
		nombre string
		xff    string
		remote string
		quiere string
	}{
		{
			nombre: "el cliente miente a la izquierda y Caddy pone la verdad al final",
			xff:    "8.8.8.8, 203.0.113.7",
			remote: "10.0.0.1:5432",
			quiere: "203.0.113.7",
		},
		{
			nombre: "un solo salto: la cabecera entera es el peer",
			xff:    "203.0.113.7",
			remote: "10.0.0.1:5432",
			quiere: "203.0.113.7",
		},
		{
			nombre: "sin cabecera, el peer de la conexión",
			remote: "203.0.113.7:5432",
			quiere: "203.0.113.7:5432",
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
			req.RemoteAddr = c.remote
			if c.xff != "" {
				req.Header.Set("X-Forwarded-For", c.xff)
			}
			if got := clientIP(req); got != c.quiere {
				t.Fatalf("clientIP() = %q, quiere %q", got, c.quiere)
			}
		})
	}
}
