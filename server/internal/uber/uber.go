package uber

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Hosts de cada ambiente. `sandbox` devuelve la tienda y el menú REALES del comercio —verificado el
// 2026-09-14— mientras la documentación dice que los datos de prueba se reinician periódicamente.
// Esas dos cosas se contradicen; para LEER da igual, y para escribir sería bloqueante. Este paquete
// no escribe.
const (
	authSandbox = "https://sandbox-login.uber.com"
	authProd    = "https://auth.uber.com"
	apiSandbox  = "https://test-api.uber.com"
	apiProd     = "https://api.uber.com"
)

// tiempoDeLectura acota cuánto puede tardar una lectura de menú. El menú real pesa 211 KB sin
// comprimir; el tope existe para que una plataforma que no responde no deje la lectura colgada.
const tiempoDeLectura = 60 * time.Second

// Errores del paquete. Se traducen a domain.ClaseDeFallo en el servicio: aquí no se conoce la base
// de datos ni el vocabulario del negocio (principio I).
var (
	ErrAuth      = errors.New("uber: autenticación rechazada")
	ErrRespuesta = errors.New("uber: respuesta inválida")
	ErrVacio     = errors.New("uber: el menú volvió sin productos")
	// ErrTruncado: la respuesta pasó el techo. Comparar un menú a medias reporta como «falta
	// arriba» lo que sí está publicado — el edge case que el spec nombra.
	ErrTruncado = errors.New("uber: el menú no cabe en una lectura")
)

// MaxBytesDeMenu acota el cuerpo de la respuesta del menú. Medido: el menú real de la tienda pesa
// 211 KB sin comprimir, así que 4 MB son un orden de magnitud de margen.
const MaxBytesDeMenu = 4 << 20

// Client lee el menú publicado de una tienda. **Solo lee**: su transporte rechaza todo verbo
// distinto de GET antes de abrir el socket.
type Client struct {
	clientID     string
	clientSecret string
	authBase     string
	apiBase      string
	// Dos clientes con dos guardias distintos, y es la parte que sostiene la garantía: el de menú
	// no deja pasar nada que no sea GET, y el de token acepta POST pero solo contra el host de
	// autenticación. Con uno solo, el POST del token sería la puerta trasera del otro.
	http  *http.Client
	auth  *http.Client
	tok   tokenCache
	ahora func() time.Time
}

// New arma el cliente para el ambiente pedido. `ambiente` ya viene validado por config: solo
// "sandbox" o "production", nunca un default adivinado.
func New(clientID, clientSecret, ambiente string) (*Client, error) {
	authBase, apiBase := authSandbox, apiSandbox
	switch ambiente {
	case "sandbox":
	case "production":
		authBase, apiBase = authProd, apiProd
	default:
		return nil, fmt.Errorf("uber: ambiente %q desconocido", ambiente)
	}
	hostAuth, err := url.Parse(authBase)
	if err != nil {
		return nil, err
	}
	return &Client{
		clientID: clientID, clientSecret: clientSecret,
		authBase: authBase, apiBase: apiBase,
		http:  &http.Client{Timeout: tiempoDeLectura, Transport: soloLectura(http.DefaultTransport)},
		auth:  &http.Client{Timeout: 30 * time.Second, Transport: soloElHostDeToken(hostAuth.Host, http.DefaultTransport)},
		ahora: time.Now,
	}, nil
}

// LeerMenu devuelve los platillos y opciones publicados de una tienda.
//
// `Accept-Encoding: gzip` lo recomienda la propia documentación y está medido: el menú de la tienda
// real pesa 211 KB sin comprimir y 23.8 KB comprimido. El `http.Client` lo descomprime solo.
func (c *Client) LeerMenu(ctx context.Context, storeID string) ([]domain.ItemDePlataforma, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	destino := fmt.Sprintf("%s/v2/eats/stores/%s/menus", c.apiBase, url.PathEscape(storeID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, destino, nil)
	if err != nil {
		return nil, fmt.Errorf("armar la lectura de menú: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := c.http.Do(req)
	if err != nil {
		// El error crudo trae la URL; aquí no importa porque no lleva secreto, pero el servicio
		// nunca lo guarda ni lo registra: solo la clase del fallo (FR-022).
		return nil, fmt.Errorf("%w: no se pudo leer el menú de %s", ErrRespuesta, NombreDeLaPlataforma)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%w: %s rechazó el token al leer el menú", ErrAuth, NombreDeLaPlataforma)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%w: %s respondió HTTP %d al menú", ErrRespuesta, NombreDeLaPlataforma, resp.StatusCode)
	}

	// TOPE AL CUERPO. Sin él, una respuesta que llegue a gigabytes —la plataforma degradada, un
	// portal cautivo, un proxy que devuelve otra cosa— se decodifica entera en memoria y mata el
	// proceso por OOM en una VM de 1 GB, tumbando el POS a media operación.
	//
	// El menú real pesa 211 KB; el techo es un orden de magnitud arriba. Que se alcance significa
	// que lo que llegó no es el menú, o que ya no cabe en una lectura: las dos cosas son
	// `menu_truncado`, que es el valor que el esquema tenía declarado y que hasta ahora nada emitía.
	acotado := io.LimitReader(resp.Body, MaxBytesDeMenu+1)
	crudo, err := io.ReadAll(acotado)
	if err != nil {
		return nil, fmt.Errorf("%w: no se pudo leer el menú de %s", ErrRespuesta, NombreDeLaPlataforma)
	}
	if int64(len(crudo)) > MaxBytesDeMenu {
		return nil, ErrTruncado
	}
	var m menuResponse
	if err := json.Unmarshal(crudo, &m); err != nil {
		return nil, fmt.Errorf("%w: el menú de %s no se pudo interpretar", ErrRespuesta, NombreDeLaPlataforma)
	}
	items := m.Aplanar(c.ahora())
	if len(items) == 0 {
		// No es «el menú está vacío»: es una lectura que no sirve. Compararla diría que sobra todo
		// el catálogo, que es el peor reporte posible y el que más invita a borrar algo.
		return nil, ErrVacio
	}
	return items, nil
}

// ClaseDeFalloDe traduce un error de este paquete al vocabulario del negocio. Vive aquí porque es
// quien conoce sus propios errores, y devuelve una CLASE y nunca el mensaje: el texto de una API
// ajena puede traer la dirección completa —y DiDi manda su secreto en el query string— así que lo
// que se guarda y se registra es siempre uno de seis valores conocidos.
func ClaseDeFalloDe(err error) domain.ClaseDeFallo {
	switch {
	case errors.Is(err, ErrAuth):
		return domain.FalloAuthRechazada
	case errors.Is(err, ErrVacio):
		return domain.FalloMenuVacio
	case errors.Is(err, ErrTruncado):
		return domain.FalloMenuTruncado
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return domain.FalloTiempoAgotado
	default:
		return domain.FalloRespuestaInvalida
	}
}
