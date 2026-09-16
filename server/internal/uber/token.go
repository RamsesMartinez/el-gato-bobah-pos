package uber

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Scope que pide el token. Los tres van juntos porque con `eats.store` solo, la lectura de pedidos
// responde 401 — verificado contra el ambiente de pruebas el 2026-09-15.
const scopeDeLectura = "eats.store eats.order eats.report"

// margenDeRenovacion: cuánto antes de vencer se pide uno nuevo. El token dura 30 días, así que un
// día de colchón sobra y evita la carrera de renovarlo justo cuando expira a media lectura.
const margenDeRenovacion = 24 * time.Hour

// margen devuelve cuánto antes de vencer se renueva, ACOTADO a la mitad de la vida del token.
//
// Con un margen fijo de 24 h y un `expires_in` menor a eso, la condición de reuso nunca se cumple y
// se pide un token POR LECTURA — exactamente lo que este caché existe para evitar, y el camino al
// «401 intermitente imposible de reproducir». Hoy Uber devuelve 30 días y no muerde; el día que
// acorte, esto lo absorbe en vez de romperse.
func (c *Client) margen() time.Duration {
	vida := c.tok.vence.Sub(c.tok.emitido)
	if vida > 0 && vida/2 < margenDeRenovacion {
		return vida / 2
	}
	return margenDeRenovacion
}

// tokenCache guarda el token en memoria del proceso.
//
// NO ES UNA OPTIMIZACIÓN: es un requisito de la plataforma. Uber permite **100 tokens por hora y el
// número 101 invalida el más antiguo**. Pedir uno por lectura no desperdicia una llamada — invalida
// el token que otro proceso está usando, y el síntoma es un 401 intermitente imposible de
// reproducir.
//
// ponytail: el techo es UN TOKEN POR PROCESO. Con N réplicas de la API son N tokens vivos, que con
// el límite de 100/hora aguanta de sobra hoy (una réplica). El camino de upgrade, si algún día hay
// varias, es mover el token a Redis —que ya se usa como caché compartida— y no cambiar nada más.
type tokenCache struct {
	mu      sync.Mutex
	valor   string
	vence   time.Time
	emitido time.Time
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	Scope       string `json:"scope"`
}

// token devuelve uno válido, pidiéndolo solo si hace falta.
func (c *Client) token(ctx context.Context) (string, error) {
	c.tok.mu.Lock()
	defer c.tok.mu.Unlock()

	if c.tok.valor != "" && c.ahora().Add(c.margen()).Before(c.tok.vence) {
		return c.tok.valor, nil
	}

	cuerpo := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"grant_type":    {"client_credentials"},
		"scope":         {scopeDeLectura},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.authBase+"/oauth/v2/token", strings.NewReader(cuerpo.Encode()))
	if err != nil {
		return "", fmt.Errorf("armar la petición de token de %s: %w", NombreDeLaPlataforma, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.auth.Do(req)
	if err != nil {
		// SIN ENVOLVER EL ERROR DE RED TAL CUAL: un error de http.Client incluye la URL completa, y
		// aunque aquí el secreto va en el cuerpo, otras plataformas lo mandan en el query string.
		// Que la costumbre sea no filtrar la dirección, y no acordarse de cuál sí y cuál no.
		return "", fmt.Errorf("%w: no se pudo pedir el token a %s", ErrAuth, NombreDeLaPlataforma)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: %s rechazó las credenciales (HTTP %d)", ErrAuth, NombreDeLaPlataforma, resp.StatusCode)
	}
	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil || tr.AccessToken == "" {
		return "", fmt.Errorf("%w: la respuesta de token de %s no trae un token", ErrRespuesta, NombreDeLaPlataforma)
	}
	c.tok.valor = tr.AccessToken
	c.tok.emitido = c.ahora()
	c.tok.vence = c.tok.emitido.Add(time.Duration(tr.ExpiresIn) * time.Second)
	return c.tok.valor, nil
}
